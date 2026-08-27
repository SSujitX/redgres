// Package bootstrap provides a temporary, self-closing HTTP listener for the
// first-run Redgres console bootstrap (PRD OPS-008, ADR-012). It binds a public
// address for a bounded window and is always closed either explicitly via Close
// (the "bootstrap complete" signal) or by its hard-cap timer, so the port can
// never be left open.
package bootstrap

import (
	"errors"
	"net"
	"net/http"
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
	// Arm the timer only now that the port is open: the exposure window is the
	// port-open window. The timer is stored under mu so the callback observes a
	// synchronized value.
	l.timer = time.AfterFunc(l.ttl, func() { _ = l.Close() })
	go func() { _ = l.server.Serve(ln) }()
	return nil
}

// Addr returns the bound address after Start. A ":0" address resolves to the
// actual ephemeral port.
func (l *Listener) Addr() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.addr
}

// Close force-closes the bootstrap listener and stops the timer. It is
// idempotent and safe to call concurrently (including from the timer). It never
// leaves the port open: it stops the timer, captures the listener, closes the
// server (dropping active connections), and force-closes the listener directly
// even if Serve has not yet registered it.
func (l *Listener) Close() error {
	l.closeOnce.Do(func() {
		l.mu.Lock()
		if l.timer != nil {
			l.timer.Stop()
			l.timer = nil
		}
		ln := l.listener
		l.listener = nil
		l.closed = true
		l.mu.Unlock()

		if err := l.server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.closeErr = err
		}
		if ln != nil {
			_ = ln.Close()
		}
	})
	return l.closeErr
}
