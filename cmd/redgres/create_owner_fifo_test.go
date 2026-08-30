//go:build unix

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/database"
	"golang.org/x/sys/unix"
)

func TestCreateOwnerGeneratePasswordFifo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	fifo := filepath.Join(t.TempDir(), "owner.fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
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

	gotCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(fifo, os.O_RDONLY, 0)
		if err != nil {
			errCh <- err
			return
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			errCh <- err
			return
		}
		gotCh <- string(b)
	}()

	if err := createOwner([]string{"-username", "admin", "-sqlite-path", path, "-generate", "-password-fifo", fifo}); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errCh:
		t.Fatal(err)
	case got := <-gotCh:
		if got != "generated-owner-password-1\n" {
			t.Fatalf("fifo = %q, want generated password line", got)
		}
	}

	printed, err := os.ReadFile(ttyPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(printed), "generated-owner-password-1") {
		t.Fatal("password must not be printed to TTY when --password-fifo is set")
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

func TestCreateOwnerGeneratePasswordFifoWithoutTTY(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	fifo := filepath.Join(t.TempDir(), "owner.fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	openOwnerTTY = func() (*os.File, error) { return nil, os.ErrNotExist }
	generateOwnerPassword = func() (string, error) { return "generated-owner-password-2", nil }
	t.Cleanup(func() {
		openOwnerTTY = func() (*os.File, error) { return os.OpenFile("/dev/tty", os.O_WRONLY, 0) }
		generateOwnerPassword = auth.GeneratePassword
	})
	gotCh := make(chan string, 1)
	go func() {
		f, err := os.OpenFile(fifo, os.O_RDONLY, 0)
		if err != nil {
			t.Error(err)
			return
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			t.Error(err)
			return
		}
		gotCh <- string(b)
	}()
	if err := createOwner([]string{"-username", "admin", "-sqlite-path", path, "-generate", "-password-fifo", fifo}); err != nil {
		t.Fatal(err)
	}
	if got := <-gotCh; got != "generated-owner-password-2\n" {
		t.Fatalf("fifo = %q", got)
	}
}
