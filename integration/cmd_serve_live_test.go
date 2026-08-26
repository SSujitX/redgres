package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/internal/secrets"
	"github.com/SSujitX/redgres/migrations"
)

const (
	serveOwnerPassword = "owner-secret-15"
	serveAddr          = "127.0.0.1:8791"
	serveBase          = "http://127.0.0.1:8791"
	serveDB            = "serve_live"
	serveOwner         = "app_serve_live"
	serveCopyDB        = "serve_live_copy"
	serveCopyOwner     = "app_serve_live_copy"
)

func serveEnv(t *testing.T, pgHost, pgPort, pgPassFile, redisURLFile, sqlitePath, secretFile string) []string {
	t.Helper()
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "REDGRES_") {
			continue
		}
		env = append(env, e)
	}
	env = append(env,
		"REDGRES_ENVIRONMENT=development",
		"REDGRES_ADDRESS="+serveAddr,
		"REDGRES_BASE_URL="+serveBase,
		"REDGRES_SQLITE_PATH="+sqlitePath,
		"REDGRES_LOG_LEVEL=error",
		"REDGRES_POSTGRES_HOST="+pgHost,
		"REDGRES_POSTGRES_PORT="+pgPort,
		"REDGRES_POSTGRES_DATABASE=postgres",
		"REDGRES_POSTGRES_USER=postgres",
		"REDGRES_POSTGRES_PASSWORD_FILE="+pgPassFile,
		"REDGRES_POSTGRES_SSLMODE=prefer",
		"REDGRES_POSTGRES_EXPECTED_MAJOR="+strconv.Itoa(livePostgresExpectedMajor(t)),
		"REDGRES_LEGACY_VAULT_SECRET_FILE="+secretFile,
		"REDGRES_REDIS_ADMIN_URL_FILE="+redisURLFile,
		"REDGRES_REDIS_EXPECTED_SERIES="+liveRedisExpectedSeries(t),
		"REDGRES_FEATURE_POSTGRES_ROW_DELETE=true",
		"REDGRES_FEATURE_POSTGRES_TRUNCATE=true",
		"REDGRES_FEATURE_POSTGRES_DROP=true",
	)
	return env
}

func serveLogin(t *testing.T, client *http.Client) (cookie, csrf string) {
	t.Helper()
	body := `{"username":"admin","password":"` + serveOwnerPassword + `"}`
	req, err := http.NewRequest(http.MethodPost, serveBase+"/api/v1/auth/login", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", serveBase)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status %d: %s", resp.StatusCode, raw)
	}
	var bodyMap map[string]any
	if err := json.Unmarshal(raw, &bodyMap); err != nil {
		t.Fatal(err)
	}
	csrf, _ = bodyMap["csrf_token"].(string)
	for _, c := range resp.Cookies() {
		if c.Name == "redgres_session" {
			cookie = c.Value
		}
	}
	if cookie == "" || csrf == "" {
		t.Fatal("missing session cookie or csrf")
	}
	return cookie, csrf
}

func serveAuthed(t *testing.T, client *http.Client, method, path, cookie, csrf, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, serveBase+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "redgres_session", Value: cookie})
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	req.Header.Set("Origin", serveBase)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

// TestLiveCmdServePoller runs the real cmd/redgres binary against real
// PostgreSQL, Redis, and a seeded SQLite control plane: the binary's
// config.Load wiring, startup Reconcile (live probe + compensator), the 1s
// queued-duplicate poller, and the real HTTP listener all run for real. The
// owner is seeded on the SQLite file before the process starts because the
// create-owner CLI is deliberately TTY-only. The 202 duplicate posted over
// HTTP is completed by the binary's own poller.
func TestLiveCmdServePoller(t *testing.T) {
	clearInheritedRedgresEnv(t)
	pgHost, pgPort, pgPassFile, pgOK := livePostgresEnv(t)
	redisURLFile, redisOK := liveRedisEnv(t)
	if !pgOK || !redisOK {
		t.Skip(skipLiveEnv)
	}
	provisionVault(t, pgHost, pgPort, pgPassFile)
	seedClean(t, pgHost, pgPort, pgPassFile)

	// Seed the owner on a fresh SQLite (CLI is TTY-only by design).
	sqlitePath := filepath.Join(t.TempDir(), "serve.db")
	db, err := database.Open(sqlitePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateOrReplaceOwner(db, "admin", serveOwnerPassword, false); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(t.TempDir(), "vault-secret")
	if err := os.WriteFile(secretFile, []byte(testVaultSecret), 0o600); err != nil {
		t.Fatal(err)
	}

	// Build the real binary.
	bin := filepath.Join(t.TempDir(), "redgres-serve.exe")
	buildOut, err := exec.Command("go", "build", "-o", bin, "github.com/SSujitX/redgres/cmd/redgres").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v: %s", err, buildOut)
	}

	// Start serve.
	cmd := exec.Command(bin, "serve")
	cmd.Env = serveEnv(t, pgHost, pgPort, pgPassFile, redisURLFile, sqlitePath, secretFile)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	// Wait for healthz (also proves startup Reconcile did not fail).
	client := &http.Client{Timeout: 10 * time.Second}
	deadline := time.Now().Add(60 * time.Second)
	healthy := false
	for time.Now().Before(deadline) {
		resp, err := http.Get(serveBase + "/api/v1/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				healthy = true
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !healthy {
		t.Fatal("serve did not become healthy")
	}

	cookie, csrf := serveLogin(t, client)

	// Create the source database over HTTP.
	code, raw := serveAuthed(t, client, http.MethodPost, "/api/v1/postgres/databases", cookie, csrf,
		`{"database":"`+serveDB+`","owner":"`+serveOwner+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("create status %d: %s", code, raw)
	}
	var created struct {
		Credential struct {
			Password string `json:"password"`
		} `json:"credential"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	srcConn := livePGConn(t, pgHost, pgPort, serveDB, serveOwner, created.Credential.Password)
	if _, err := srcConn.Exec(context.Background(), "CREATE TABLE public.items (id integer PRIMARY KEY, name text NOT NULL)"); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if _, err := srcConn.Exec(context.Background(), "INSERT INTO public.items (id, name) VALUES (1,'a'),(2,'b')"); err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	// POST duplicate -> 202 queued; the binary's own poller completes it.
	code, raw = serveAuthed(t, client, http.MethodPost, "/api/v1/postgres/databases/"+serveDB+"/duplicate", cookie, csrf,
		`{"database":"`+serveCopyDB+`","owner":"`+serveCopyOwner+`"}`)
	if code != http.StatusAccepted {
		t.Fatalf("duplicate status %d: %s", code, raw)
	}
	var accepted struct {
		Operation struct {
			ID string `json:"id"`
		} `json:"operation"`
	}
	if err := json.Unmarshal(raw, &accepted); err != nil {
		t.Fatal(err)
	}

	// Poll the operation until the binary's poller transitions it to succeeded.
	deadline = time.Now().Add(90 * time.Second)
	succeeded := false
	for time.Now().Before(deadline) {
		code, raw = serveAuthed(t, client, http.MethodGet, "/api/v1/operations/"+accepted.Operation.ID, cookie, csrf, "")
		if code == http.StatusOK && strings.Contains(string(raw), `"status":"succeeded"`) {
			succeeded = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !succeeded {
		t.Fatalf("duplicate operation did not succeed via the binary poller; last body %s", raw)
	}

	// Copy is listed; vault row decrypts to a working password; rows cloned.
	code, raw = serveAuthed(t, client, http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, "")
	if code != http.StatusOK || !strings.Contains(string(raw), serveCopyDB) {
		t.Fatalf("copy missing after poller: %d %s", code, raw)
	}
	password := livePGPassword(t, os.Getenv("REDGRES_TEST_POSTGRES_PASSWORD_FILE"))
	vconn := livePGConn(t, pgHost, pgPort, "database_console_vault", "postgres", password)
	var token string
	if err := vconn.QueryRow(context.Background(), "SELECT encrypted_password FROM public.project_credentials WHERE role_name = '"+serveCopyOwner+"'").Scan(&token); err != nil {
		t.Fatalf("vault row: %v", err)
	}
	plain, err := secrets.Decrypt(secrets.DeriveVaultKey(testVaultSecret), token)
	if err != nil {
		t.Fatalf("decrypt copy password: %v", err)
	}
	copyConn := livePGConn(t, pgHost, pgPort, serveCopyDB, serveCopyOwner, string(plain))
	var items int
	if err := copyConn.QueryRow(context.Background(), "SELECT count(*)::int FROM public.items").Scan(&items); err != nil {
		t.Fatalf("copy items: %v", err)
	}
	if items != 2 {
		t.Fatalf("copy items = %d want 2", items)
	}

	// The binary audited the duplicate success.
	code, raw = serveAuthed(t, client, http.MethodGet, "/api/v1/audit?limit=50", cookie, csrf, "")
	if code != http.StatusOK || !strings.Contains(string(raw), `"action":"postgres.database.duplicate"`) || !strings.Contains(string(raw), `"outcome":"success"`) {
		t.Fatalf("audit missing duplicate success: %d %s", code, raw)
	}
}
