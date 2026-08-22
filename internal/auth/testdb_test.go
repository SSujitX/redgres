package auth

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/migrations"
)

const testPassword = "correct-horse-ok"

func testDB(t *testing.T) *sql.DB {
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
	return db
}

func mustOwner(t *testing.T, db *sql.DB) Owner {
	t.Helper()
	owner, err := CreateOrReplaceOwner(db, "admin", testPassword, false)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
