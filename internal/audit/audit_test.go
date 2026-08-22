package audit

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/migrations"
)

func TestAuditLoginOmitsSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}

	store := Store{DB: db}
	if err := store.Record("admin", "owner.login", "admin", "success", "aabbccddeeff00112233445566778899", "127.0.0.1", map[string]any{
		"username":      "admin",
		"password":      "canary-password-value",
		"csrf_token":    "canary-csrf",
		"cookie":        "redgres_session=canary",
		"session":       "should-stay-unless-key-matches",
		"admin_url":     "postgresql://owner:canary@127.0.0.1/db",
		"authorization": "Bearer canary",
	}); err != nil {
		t.Fatal(err)
	}

	var raw string
	if err := db.QueryRow(`SELECT metadata FROM audit_events`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if ContainsSecret(raw) {
		t.Fatalf("secret material stored: %s", raw)
	}
	if strings.Contains(raw, "canary") {
		t.Fatalf("canary leaked: %s", raw)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["username"] != "admin" {
		t.Fatalf("username = %#v", meta["username"])
	}
	if _, ok := meta["password"]; ok {
		t.Fatal("password key present")
	}
}
