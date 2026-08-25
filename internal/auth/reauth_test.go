package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestReauthenticateSuccess(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	if err := Reauthenticate(db, "admin", testPassword); err != nil {
		t.Fatalf("success: %v", err)
	}
}

func TestReauthenticateMismatchIsDistinct(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	err := Reauthenticate(db, "admin", "wrong-password-xx")
	if !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("got %v, want ErrReauthRequired", err)
	}
	if errors.Is(err, ErrMismatchedHash) {
		t.Fatal("mismatch reused login ErrMismatchedHash")
	}
	if strings.Contains(err.Error(), "wrong-password-xx") {
		t.Fatalf("password leaked in error: %v", err)
	}
}

func TestReauthenticateMissingOwnerUsesVerifyUnknown(t *testing.T) {
	db := testDB(t)
	called := false
	gotPassword := ""
	orig := verifyUnknown
	verifyUnknown = func(password string) {
		called = true
		gotPassword = password
		orig(password)
	}
	t.Cleanup(func() { verifyUnknown = orig })

	err := Reauthenticate(db, "nobody", "missing-owner-pw")
	if !called {
		t.Fatal("VerifyUnknown not called")
	}
	if gotPassword != "missing-owner-pw" {
		t.Fatalf("VerifyUnknown password = %q", gotPassword)
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
	if errors.Is(err, ErrReauthRequired) {
		t.Fatal("missing owner mapped to reauth mismatch")
	}
}

func TestReauthenticateLookupFailure(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	_ = db.Close()
	err := Reauthenticate(db, "admin", testPassword)
	if err == nil {
		t.Fatal("expected lookup error")
	}
	if errors.Is(err, ErrReauthRequired) || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("lookup failure mapped to %v", err)
	}
}
