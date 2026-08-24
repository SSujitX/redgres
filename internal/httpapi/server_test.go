package httpapi

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/migrations"
)

const (
	wantCSP         = "default-src 'self'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
	wantPermissions = "accelerometer=(), autoplay=(), camera=(), display-capture=(), encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), midi=(), payment=(), picture-in-picture=(), screen-wake-lock=(), usb=(), xr-spatial-tracking=()"
)

func testServer(t *testing.T, assets fstest.MapFS) (*Server, string) {
	t.Helper()
	if assets == nil {
		assets = fstest.MapFS{}
	}
	return testServerFS(t, assets)
}

func testServerFS(t *testing.T, assets fs.FS) (*Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "redgres.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return New(config.Config{
		Address:            "127.0.0.1:8790",
		BaseURL:            "http://127.0.0.1:8790",
		SessionTTL:         12 * time.Hour,
		AbsoluteSessionTTL: 24 * time.Hour,
	}, db, assets, nil, nil), path
}

func TestHealthzOK(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var body healthzBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q", body.Status)
	}
	if !requestIDOK(body.RequestID) {
		t.Fatalf("request_id = %q", body.RequestID)
	}
	if rec.Header().Get("X-Request-ID") != body.RequestID {
		t.Fatalf("header/body request id mismatch")
	}
}

func TestHealthzUnavailableDoesNotLeak(t *testing.T) {
	srv, path := testServer(t, nil)
	_ = srv.db.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable {
		t.Fatalf("code = %q", body.Error.Code)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, path) || strings.Contains(strings.ToLower(raw), "sqlite") || strings.Contains(raw, "modernc") {
		t.Fatalf("error leaked internals: %s", raw)
	}
	if !requestIDOK(body.RequestID) {
		t.Fatalf("request_id = %q", body.RequestID)
	}
}

func TestSecurityHeadersOnAPIAndStatic(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html>ok</html>")}}
	srv, _ := testServer(t, assets)
	for _, path := range []string{"/api/v1/healthz", "/"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Handler().ServeHTTP(rec, req)
		if rec.Header().Get("Content-Security-Policy") != wantCSP {
			t.Fatalf("%s CSP = %q", path, rec.Header().Get("Content-Security-Policy"))
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s X-Content-Type-Options missing", path)
		}
		if rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("%s Referrer-Policy missing", path)
		}
		if rec.Header().Get("Cross-Origin-Opener-Policy") != "same-origin" {
			t.Fatalf("%s COOP missing", path)
		}
		if rec.Header().Get("Permissions-Policy") != wantPermissions {
			t.Fatalf("%s Permissions-Policy = %q", path, rec.Header().Get("Permissions-Policy"))
		}
		if rec.Header().Get("Strict-Transport-Security") != "" {
			t.Fatalf("%s unexpectedly set HSTS", path)
		}
	}
}

func TestRequestIDIsServerGenerated(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	req.Header.Set("X-Request-ID", "<script>alert(1)</script>")
	srv.Handler().ServeHTTP(rec, req)
	got := rec.Header().Get("X-Request-ID")
	if got == "<script>alert(1)</script>" {
		t.Fatal("inbound request id was echoed")
	}
	if !requestIDOK(got) {
		t.Fatalf("generated request id = %q", got)
	}
}

func TestAPINotFoundAndMethodNotAllowed(t *testing.T) {
	srv, _ := testServer(t, nil)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing = %d", rec.Code)
	}
	var missing errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &missing); err != nil {
		t.Fatal(err)
	}
	if missing.Error.Code != CodeNotFound {
		t.Fatalf("code = %q", missing.Error.Code)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/healthz", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST healthz = %d", rec.Code)
	}
	var method errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &method); err != nil {
		t.Fatal(err)
	}
	if method.Error.Code != CodeMethodNotAllowed {
		t.Fatalf("code = %q", method.Error.Code)
	}
}

func TestStaticAssetsPresentAndAbsent(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":         {Data: []byte("<html>boot</html>")},
		"assets/app-hash.js": {Data: []byte("console.log(1)")},
	}
	srv, _ := testServer(t, assets)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "boot") {
		t.Fatalf("GET / = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("index cache = %q", rec.Header().Get("Cache-Control"))
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app-hash.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset cache = %q", rec.Header().Get("Cache-Control"))
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/postgres/unknown", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "boot") {
		t.Fatalf("SPA fallback = %d %s", rec.Code, rec.Body.String())
	}

	empty, _ := testServer(t, fstest.MapFS{})
	rec = httptest.NewRecorder()
	empty.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing assets = %d", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("503 cache = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestStaticAllowsOnlyIndexAndAssets(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":         {Data: []byte("<html>boot</html>")},
		"secret.env":         {Data: []byte("REDGRES_SESSION=leak")},
		"assets/app-hash.js": {Data: []byte("ok")},
	}
	srv, _ := testServer(t, assets)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/secret.env", nil))
	if strings.Contains(rec.Body.String(), "leak") {
		t.Fatal("non-allow-listed embed file was served")
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/../secret.env", nil))
	if strings.Contains(rec.Body.String(), "leak") {
		t.Fatal("parent-path request served a secret")
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "//api/v1/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cleaned healthz = %d %s", rec.Code, rec.Body.String())
	}
}

// TestStaticFromDirectoryKeepsAllowList proves the development filesystem asset
// source is bounded by the same allow-list as the embedded source. os.DirFS has
// different traversal and separator behavior than fstest.MapFS.
func TestStaticFromDirectoryKeepsAllowList(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, "outside.txt"), []byte("outside-canary"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "app")
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>boot</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app-hash.js"), []byte("console.log(1)"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.env"), []byte("REDGRES_SESSION=inside-canary"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, _ := testServerFS(t, os.DirFS(root))

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "boot") {
		t.Fatalf("GET / = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("index cache = %q", rec.Header().Get("Cache-Control"))
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app-hash.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset cache = %q", rec.Header().Get("Cache-Control"))
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/secret.env", nil))
	if strings.Contains(rec.Body.String(), "inside-canary") {
		t.Fatalf("non-allow-listed directory file was served: %s", rec.Body.String())
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "boot") {
		t.Fatalf("secret.env should fall back to index: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset = %d", rec.Code)
	}

	// Backslash, colon, and NUL are legal inside a single fs path element, so
	// these pass allowedStaticName and reach the filesystem layer. They must
	// still never resolve outside the asset root or open a device.
	for _, target := range []string{
		"/assets/../../outside.txt",
		"/assets/%2e%2e/%2e%2e/outside.txt",
		"/assets/..%2f..%2foutside.txt",
		`/assets/..\..\outside.txt`,
		"/assets/%5c..%5c..%5coutside.txt",
		"/assets/app-hash.js:secret",
		"/assets/x%00.js",
		"/assets/NUL.js",
		"/assets/CON",
	} {
		rec = httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if strings.Contains(rec.Body.String(), "outside-canary") {
			t.Fatalf("%s escaped the asset directory: %s", target, rec.Body.String())
		}
		if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
			t.Fatalf("%s = %d %s", target, rec.Code, rec.Body.String())
		}
		if rec.Code == http.StatusOK && !strings.Contains(rec.Body.String(), "boot") {
			t.Fatalf("%s served non-index content: %s", target, rec.Body.String())
		}
	}

	// A trailing slash is not an allow-listed asset name, so it falls back to
	// index.html like any unknown route. It must never list the directory.
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/", nil))
	if strings.Contains(rec.Body.String(), "app-hash.js") {
		t.Fatalf("directory listing was served: %s", rec.Body.String())
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "boot") {
		t.Fatalf("directory request = %d %s", rec.Code, rec.Body.String())
	}
}

func TestStaticFromEmptyDirectoryIsUnavailable(t *testing.T) {
	srv, _ := testServerFS(t, os.DirFS(t.TempDir()))

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("503 cache = %q", rec.Header().Get("Cache-Control"))
	}
}

func requestIDOK(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func TestLimitBody(t *testing.T) {
	srv, _ := testServer(t, nil)
	var sawMaxBytes bool
	handler := srv.limitBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errorsAsMaxBytes(err) {
				sawMaxBytes = true
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			_ = maxErr
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	body := strings.Repeat("a", maxBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	handler.ServeHTTP(rec, req)
	if !sawMaxBytes {
		t.Fatal("expected MaxBytesError")
	}
}

func TestRecovererHidesPanic(t *testing.T) {
	srv, _ := testServer(t, nil)
	handler := srv.withRequestID(srv.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret-panic-value")
	})))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret-panic-value") {
		t.Fatalf("panic leaked: %s", rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeInternal {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func errorsAsMaxBytes(err error) bool {
	return err != nil && strings.Contains(err.Error(), "http: request body too large")
}
