package toolgate

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Gate reverse-proxies one expert tool after a valid launch cookie.
type Gate struct {
	Tool       string
	Upstream   *url.URL
	Store      *Memory
	Secure     bool
	ConsoleURL string
}

func NewGate(tool, upstream string, store *Memory, secure bool, consoleURL string) (*Gate, error) {
	if !ValidTool(tool) {
		return nil, ErrInvalidTool
	}
	parsed, err := url.Parse(upstream)
	if err != nil || parsed.Host == "" {
		return nil, ErrNotConfigured
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	return &Gate{Tool: tool, Upstream: parsed, Store: store, Secure: secure, ConsoleURL: strings.TrimRight(consoleURL, "/")}, nil
}

func (g *Gate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == LaunchPath {
		g.consumeLaunch(w, r)
		return
	}
	cookie, err := r.Cookie(CookieName)
	if err != nil || !g.Store.ValidSession(cookie.Value, g.Tool) {
		g.deny(w, r)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(g.Upstream)
	proxy.ServeHTTP(w, r)
}

func (g *Gate) consumeLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	token, err := g.Store.Consume(ticket, g.Tool)
	if err != nil {
		g.deny(w, r)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   g.Secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().UTC().Add(sessionTTL),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (g *Gate) deny(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if g.ConsoleURL != "" {
		http.Redirect(w, r, g.ConsoleURL+"/system", http.StatusSeeOther)
		return
	}
	http.Error(w, "Open this tool from the Redgres console.", http.StatusForbidden)
}

// ListenAndServe binds a loopback-only gate.
func ListenAndServe(addr string, handler http.Handler) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return ErrNotConfigured
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}
