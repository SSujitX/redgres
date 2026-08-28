// Package bootstrap provides a temporary, self-closing HTTP listener for the
// first-run Redgres console bootstrap (PRD OPS-008, ADR-012). It binds a public
// address for a bounded window and is always closed either explicitly via Close
// (the "bootstrap complete" signal) or by its hard-cap timer, so the port can
// never be left open.
package bootstrap

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultTTL is the hard-cap auto-close used when the caller supplies a
// non-positive TTL.
const DefaultTTL = 30 * time.Minute

// Listener is a temporary HTTP listener for first-run bootstrap.
//
// Start binds the configured address, arms the hard-cap timer (so the exposure
// window is measured from bind, not construction), and serves handler. Close
// force-closes the listener immediately (dropping active connections) and is
// idempotent. The timer calls Close after ttl as a failsafe.
type Listener struct {
	server *http.Server
	ttl    time.Duration

	mu       sync.Mutex
	addr     string
	listener net.Listener
	closed   bool
	timer    *time.Timer

	closeOnce sync.Once
	closeErr  error

	sqlitePath string
}

// New prepares a bootstrap listener. A non-positive ttl falls back to
// DefaultTTL. The hard-cap timer is armed by Start, not here, so the exposure
// clock starts when the port opens.
func New(handler http.Handler, addr string, ttl time.Duration) *Listener {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Listener{
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    16 << 10,
		},
		addr: addr,
		ttl:  ttl,
	}
}

// Start binds addr, arms the hard-cap timer, and serves handler in a
// goroutine. It fails closed: on a bind error it returns without arming a timer
// or binding any fallback address.
func (l *Listener) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return errors.New("bootstrap listener already closed")
	}
	ln, err := net.Listen("tcp", l.addr)
	if err != nil {
		return err
	}
	l.listener = ln
	l.addr = ln.Addr().String()
	sqlitePath := l.sqlitePath
	// Arm the timer only now that the port is open: the exposure window is the
	// port-open window. The timer is stored under mu so the callback observes a
	// synchronized value. TTL persist writes the closed sentinel so a later
	// systemd restart cannot reopen :8989 (process SIGTERM Close does not).
	l.timer = time.AfterFunc(l.ttl, func() {
		_ = WriteClosedState(sqlitePath)
		_ = l.Close()
	})
	go func() { _ = l.server.Serve(ln) }()
	return nil
}

const (
	closedMarkerName = "bootstrap.closed"
	ufwRequestName   = "bootstrap-ufw-remove.requested"
)

// SetSQLitePath records the control-state path so the TTL callback can persist
// "already closed" next to SQLite. Process Close/SIGTERM does not persist.
func (l *Listener) SetSQLitePath(sqlitePath string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sqlitePath = sqlitePath
}

// ClosedMarkerPath is the sentinel next to the SQLite file.
func ClosedMarkerPath(sqlitePath string) string {
	if sqlitePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(sqlitePath), closedMarkerName)
}

// UFWRemoveRequestPath is the systemd path-unit trigger next to SQLite.
func UFWRemoveRequestPath(sqlitePath string) string {
	if sqlitePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(sqlitePath), ufwRequestName)
}

// MarkerPresent reports whether a closed sentinel exists. Any existing
// filesystem object counts so a restart cannot reopen the public port.
func MarkerPresent(sqlitePath string) bool {
	path := ClosedMarkerPath(sqlitePath)
	if path == "" {
		return false
	}
	_, err := os.Lstat(path)
	return err == nil
}

// WriteClosedState writes the closed sentinel and UFW-remove request. Empty
// sqlitePath is a no-op. Existing files are left in place (idempotent).
func WriteClosedState(sqlitePath string) error {
	if sqlitePath == "" {
		return nil
	}
	dir := filepath.Dir(sqlitePath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	for _, name := range []string{closedMarkerName, ufwRequestName} {
		path := filepath.Join(dir, name)
		if _, err := os.Lstat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte("1\n"), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Addr returns the bound address after Start. A ":0" address resolves to the
// actual ephemeral port.
func (l *Listener) Addr() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.addr
}

// Open reports whether the bootstrap port is still accepting connections.
func (l *Listener) Open() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.closed && l.listener != nil
}

// Close force-closes the bootstrap listener and stops the timer. It is
// idempotent and safe to call concurrently (including from the timer). It never
// leaves the port open: it stops the timer, captures the listener, closes the
// server (dropping active connections), and force-closes the listener directly
// even if Serve has not yet registered it.
func (l *Listener) Close() error {
	l.closeOnce.Do(func() {
		l.finishClose(false, context.Background())
	})
	return l.closeErr
}

// Shutdown stops accepting new connections and waits for in-flight handlers to
// finish (up to ctx), then closes the listener. Prefer this when closing from a
// request served on this listener so the success body can flush. Idempotent with
// Close (whichever runs first wins via closeOnce). The hard-cap timer still uses
// force Close.
func (l *Listener) Shutdown(ctx context.Context) error {
	l.closeOnce.Do(func() {
		l.finishClose(true, ctx)
	})
	return l.closeErr
}

func (l *Listener) finishClose(graceful bool, ctx context.Context) {
	l.mu.Lock()
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	ln := l.listener
	l.listener = nil
	l.closed = true
	l.mu.Unlock()

	var err error
	if graceful {
		err = l.server.Shutdown(ctx)
	} else {
		err = l.server.Close()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		l.closeErr = err
	}
	if ln != nil {
		_ = ln.Close()
	}
}
