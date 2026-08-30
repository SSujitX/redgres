package toolgate

import (
	"net"
	"net/url"
	"strings"
	"sync"
)

// Origin holds the live console URL used when a tool-gate deny redirects.
// Confirm-reachable and domain apply update it so a process that started
// with bootstrap :8989 does not keep sending browsers to the raw IP.
type Origin struct {
	mu  sync.RWMutex
	url string
}

func NewOrigin(url string) *Origin {
	o := &Origin{}
	o.Set(url)
	return o
}

func (o *Origin) Set(raw string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.url = strings.TrimRight(strings.TrimSpace(raw), "/")
	o.mu.Unlock()
}

func (o *Origin) Get() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.url
}

// PreferredConsoleURL never returns a bootstrap :8989 origin or a raw
// public-IP HTTP URL. Persisted https://{console} wins when present.
func PreferredConsoleURL(baseURL, persistedHTTPS string) string {
	persisted := strings.TrimRight(strings.TrimSpace(persistedHTTPS), "/")
	if usableConsoleOrigin(persisted) && !bootstrapOrigin(persisted) {
		return persisted
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if usableConsoleOrigin(base) && !bootstrapOrigin(base) {
		return base
	}
	return ""
}

func usableConsoleOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Path != "" && u.Path != "/" {
		return false
	}
	return true
}

func bootstrapOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return true
	}
	if u.Port() == "8989" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && !ip.IsLoopback() && u.Scheme == "http"
}
