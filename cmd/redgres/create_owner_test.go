package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/migrations"
)

func TestCreateOwnerRefusesOverwriteWithoutReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	readPasswordPair = func() (string, string, error) {
		return "correct-horse-ok", "correct-horse-ok", nil
	}
	t.Cleanup(func() { readPasswordPair = defaultReadPasswordPair })

	if err := createOwner([]string{"-username", "admin", "-sqlite-path", path}); err != nil {
		t.Fatal(err)
	}
	first := mustOwnerHash(t, path)
	if err := createOwner([]string{"-username", "other", "-sqlite-path", path}); err == nil {
		t.Fatal("expected overwrite refusal")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v", err)
	}
	if mustOwnerHash(t, path) != first {
		t.Fatal("hash changed without --replace")
	}
}

func TestCreateOwnerReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	readPasswordPair = func() (string, string, error) {
		return "correct-horse-ok", "correct-horse-ok", nil
	}
	t.Cleanup(func() { readPasswordPair = defaultReadPasswordPair })
	if err := createOwner([]string{"-username", "admin", "-sqlite-path", path}); err != nil {
		t.Fatal(err)
	}
	readPasswordPair = func() (string, string, error) {
		return "correct-horse-ok2", "correct-horse-ok2", nil
	}
	if err := createOwner([]string{"-username", "operator", "-sqlite-path", path, "-replace"}); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	owner, err := auth.GetOwner(db)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Username != "operator" {
		t.Fatalf("username = %q", owner.Username)
	}
}

func TestCreateOwnerHasNoPasswordFlag(t *testing.T) {
	err := createOwner([]string{"-username", "admin", "-password", "must-not-be-accepted"})
	if err == nil {
		t.Fatal("expected unknown -password flag")
	}
	if !strings.Contains(err.Error(), "password") {
		t.Fatalf("error = %v", err)
	}
}

func mustOwnerHash(t *testing.T, path string) string {
	t.Helper()
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	owner, err := auth.GetOwner(db)
	if err != nil {
		t.Fatal(err)
	}
	return owner.PasswordHash
}
