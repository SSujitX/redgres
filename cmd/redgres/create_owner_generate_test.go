package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/database"
)

func TestCreateOwnerGenerate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	ttyPath := filepath.Join(t.TempDir(), "tty")
	tty, err := os.Create(ttyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()

	openOwnerTTY = func() (*os.File, error) { return tty, nil }
	generateOwnerPassword = func() (string, error) { return "generated-owner-password-1", nil }
	t.Cleanup(func() {
		openOwnerTTY = func() (*os.File, error) { return os.OpenFile("/dev/tty", os.O_WRONLY, 0) }
		generateOwnerPassword = auth.GeneratePassword
	})

	if err := createOwner([]string{"-username", "admin", "-sqlite-path", path, "-generate"}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(ttyPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Generated owner password: generated-owner-password-1\n"; string(got) != want {
		t.Fatalf("printed = %q, want %q", string(got), want)
	}

	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner, err := auth.GetOwner(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Verify(owner.PasswordHash, "generated-owner-password-1"); err != nil {
		t.Fatalf("generated password does not verify: %v", err)
	}
}

func TestCreateOwnerGenerateRequiresTTY(t *testing.T) {
	openOwnerTTY = func() (*os.File, error) { return nil, os.ErrNotExist }
	t.Cleanup(func() {
		openOwnerTTY = func() (*os.File, error) { return os.OpenFile("/dev/tty", os.O_WRONLY, 0) }
	})

	err := createOwner([]string{"-username", "admin", "-sqlite-path", filepath.Join(t.TempDir(), "x.db"), "-generate"})
	if err == nil || !strings.Contains(err.Error(), "controlling terminal") {
		t.Fatalf("err = %v, want controlling terminal error", err)
	}
}
func TestCreateOwnerGenerateDisplayFailureMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	tty, err := os.Create(filepath.Join(t.TempDir(), "tty"))
	if err != nil {
		t.Fatal(err)
	}
	tty.Close()

	openOwnerTTY = func() (*os.File, error) { return tty, nil }
	generateOwnerPassword = func() (string, error) { return "generated-owner-password-1", nil }
	t.Cleanup(func() {
		openOwnerTTY = func() (*os.File, error) { return os.OpenFile("/dev/tty", os.O_WRONLY, 0) }
		generateOwnerPassword = auth.GeneratePassword
	})

	err = createOwner([]string{"-username", "admin", "-sqlite-path", path, "-generate"})
	if err == nil || !strings.Contains(err.Error(), "rerun create-owner --generate --replace") {
		t.Fatalf("err = %v, want recovery message", err)
	}
}
