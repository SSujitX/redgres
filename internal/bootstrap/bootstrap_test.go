package bootstrap

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func dial(t *testing.T, addr string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	conn.Close()
}

// assertRefused waits briefly until addr stops accepting connections.
func assertRefused(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return
		}
		conn.Close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("address %s still accepts connections after close", addr)
}

func TestStartServesAndImmediateClose(t *testing.T) {
	l := New(okHandler(), "127.0.0.1:0", time.Minute)
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := l.Addr()
	if addr == "" || addr == "127.0.0.1:0" {
		t.Fatalf("Addr = %q after Start", addr)
	}
	dial(t, addr)

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertRefused(t, addr)

	// Close is idempotent.
	if err := l.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestTimerCloses(t *testing.T) {
	l := New(okHandler(), "127.0.0.1:0", 300*time.Millisecond)
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := l.Addr()
	dial(t, addr)
	// The timer must close the listener without an explicit Close.
	assertRefused(t, addr)
}

func TestCloseDropsActiveConnection(t *testing.T) {
	l := New(okHandler(), "127.0.0.1:0", time.Minute)
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := l.Addr()
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	start := time.Now()
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Close blocked %v with an active connection", elapsed)
	}
	conn.Close()
	assertRefused(t, addr)
}

func TestShutdownFinishesInFlightHandler(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	l := New(h, "127.0.0.1:0", time.Minute)
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := l.Addr()

	errCh := make(chan error, 1)
	bodyCh := make(chan []byte, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			errCh <- err
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			errCh <- errors.New("unexpected status")
			return
		}
		bodyCh <- b
		errCh <- nil
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		done <- l.Shutdown(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	close(release)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("client: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not finish")
	}
	select {
	case body := <-bodyCh:
		if string(body) != `{"ok":true}` {
			t.Fatalf("body = %q", body)
		}
	default:
		t.Fatal("missing body")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not finish")
	}
	assertRefused(t, addr)
}

func TestStartFailsClosedOnBadAddress(t *testing.T) {
	l := New(okHandler(), "not-a-valid-addr", time.Minute)
	if err := l.Start(); err == nil {
		t.Fatal("Start should fail on an invalid address")
	}
	// Close is safe and idempotent even after a failed Start.
	if err := l.Close(); err != nil {
		t.Fatalf("Close after failed Start: %v", err)
	}
}

func TestTTLDefault(t *testing.T) {
	l := New(okHandler(), "127.0.0.1:0", 0)
	if l.ttl != DefaultTTL {
		t.Fatalf("ttl = %s, want default %s", l.ttl, DefaultTTL)
	}
	_ = l.Close()
}

func TestTimerNotArmedBeforeStart(t *testing.T) {
	l := New(okHandler(), "127.0.0.1:0", 300*time.Millisecond)
	// Sleep past the TTL before Start: the timer must not fire (or close the
	// listener) until Start has bound the port.
	time.Sleep(400 * time.Millisecond)
	if err := l.Start(); err != nil {
		t.Fatalf("Start after ttl elapsed (timer must arm on Start, not New): %v", err)
	}
	addr := l.Addr()
	dial(t, addr)
	_ = l.Close()
}
