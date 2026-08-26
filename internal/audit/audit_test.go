package audit

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/migrations"
)

func TestAuditRejectsNestedSecrets(t *testing.T) {
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
	err = store.Record("admin", "owner.login", "admin", "success", "aabbccddeeff00112233445566778899", "127.0.0.1", map[string]any{
		"username": "admin",
		"context": map[string]any{
			"password": "canary-password-value",
		},
	})
	if !errors.Is(err, ErrUnsafeMetadata) {
		t.Fatalf("got %v, want ErrUnsafeMetadata", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unsafe audit event inserted: %d", count)
	}
}

func TestAuditRejectsNestedSecretUnderAllowedKey(t *testing.T) {
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
	for name, metadata := range map[string]map[string]any{
		"nested_map": {"username": map[string]any{"password": "canary-password-value"}},
		"nested_array": {"username": []any{
			map[string]any{"secret": "canary-secret-value"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			err := store.Record("admin", "owner.login", "admin", "success", "aabbccddeeff00112233445566778899", "127.0.0.1", metadata)
			if !errors.Is(err, ErrUnsafeMetadata) {
				t.Fatalf("got %v, want ErrUnsafeMetadata", err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("unsafe audit event inserted: %d", count)
			}
		})
	}
}

func TestAuditRejectsActionSpecificUnknownKeyAndURLValue(t *testing.T) {
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
	for name, metadata := range map[string]map[string]any{
		"unknown_key": {"username": "admin", "session": "safe-looking"},
		"url_value":   {"username": "postgresql://owner:canary@127.0.0.1/db"},
	} {
		t.Run(name, func(t *testing.T) {
			err := store.Record("admin", "owner.login", "admin", "success", "aabbccddeeff00112233445566778899", "127.0.0.1", metadata)
			if !errors.Is(err, ErrUnsafeMetadata) {
				t.Fatalf("got %v, want ErrUnsafeMetadata", err)
			}
		})
	}
}

func TestAuditStoresOnlyAllowedMetadata(t *testing.T) {
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
		"username": "admin",
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
	if len(meta) != 1 {
		t.Fatalf("metadata = %#v", meta)
	}
}

func TestDuplicateAuditAllowsOperationID(t *testing.T) {
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
	err = store.Record("admin", "postgres.database.duplicate", "project_a_copy", "success", "aabbccddeeff00112233445566778899", "127.0.0.1", map[string]any{
		"database":     "project_a_copy",
		"owner":        "app_project_a_copy",
		"source":       "project_a",
		"operation_id": "0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = store.Record("admin", "postgres.database.duplicate", "project_a_copy", "success", "aabbccddeeff00112233445566778899", "127.0.0.1", map[string]any{
		"database": "project_a_copy",
		"owner":    "app_project_a_copy",
		"source":   "project_a",
		"password": "canary-secret",
	})
	if !errors.Is(err, ErrUnsafeMetadata) {
		t.Fatalf("got %v, want ErrUnsafeMetadata", err)
	}
}
