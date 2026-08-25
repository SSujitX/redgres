package integration

import (
	"net"
	"strings"
	"testing"
)

func allowedLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func TestAllowListRejectsNonLoopback(t *testing.T) {
	t.Parallel()
	denied := []string{
		"host.docker.internal",
		"db.example.com",
		"10.0.0.1",
		"172.17.0.1",
		"192.168.1.1",
		"postgres",
		"redis",
	}
	for _, host := range denied {
		if allowedLoopbackHost(host) {
			t.Fatalf("allowed %q", host)
		}
	}
}

func TestAllowListAcceptsLoopback(t *testing.T) {
	t.Parallel()
	ok := []string{"127.0.0.1", "localhost", "::1", "[::1]"}
	for _, host := range ok {
		if !allowedLoopbackHost(host) {
			t.Fatalf("denied %q", host)
		}
	}
}
