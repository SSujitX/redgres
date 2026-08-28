package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCreateTunnelRetriesAfterNameConflict(t *testing.T) {
	t.Parallel()
	var creates atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cfd_tunnel"):
			if r.URL.Query().Get("name") == "" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": []map[string]any{
					{"id": "old-tun", "name": "redgres-console.example.com", "status": "inactive", "connections": []any{}},
				},
			})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "old-tun"):
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cfd_tunnel"):
			n := creates.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": false,
					"errors":  []map[string]any{{"code": 1013, "message": "A tunnel with this name already exists"}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": map[string]string{
					"id":    "new-tun",
					"name":  "redgres-console.example.com",
					"token": "connector-token",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := &HTTPClient{Token: "test-token", BaseURL: srv.URL, Client: srv.Client()}
	tun, err := client.CreateTunnel(context.Background(), "acct-1", "redgres-console.example.com")
	if err != nil {
		t.Fatalf("CreateTunnel: %v", err)
	}
	if tun.ID != "new-tun" || tun.Token != "connector-token" {
		t.Fatalf("tunnel = %#v", tun)
	}
	if creates.Load() != 2 {
		t.Fatalf("creates = %d, want 2", creates.Load())
	}
}

func TestCreateTunnelRefusesActiveNameConflict(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cfd_tunnel"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": []map[string]any{
					{"id": "live-tun", "name": "redgres-console.example.com", "status": "healthy", "connections": []any{map[string]any{}}},
				},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cfd_tunnel"):
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"errors":  []map[string]any{{"code": 1013, "message": "A tunnel with this name already exists"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := &HTTPClient{Token: "test-token", BaseURL: srv.URL, Client: srv.Client()}
	_, err := client.CreateTunnel(context.Background(), "acct-1", "redgres-console.example.com")
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestDiscoverZoneEncodesQuery(t *testing.T) {
	t.Parallel()
	var gotName string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotName = r.URL.Query().Get("name")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []map[string]any{
				{
					"id":   "zone-1",
					"name": "redgres.com",
					"account": map[string]string{
						"id": "acct-1",
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	client := &HTTPClient{Token: "test-token", BaseURL: srv.URL, Client: srv.Client()}
	z, err := client.DiscoverZone(context.Background(), "redgres.com")
	if err != nil {
		t.Fatalf("DiscoverZone: %v", err)
	}
	if gotName != "redgres.com" {
		t.Fatalf("query name = %q", gotName)
	}
	if z.ID != "zone-1" || z.AccountID != "acct-1" {
		t.Fatalf("zone = %#v", z)
	}
}
