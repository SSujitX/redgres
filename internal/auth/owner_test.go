package auth

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCreateOwnerRefusesWeakPassword(t *testing.T) {
	db := testDB(t)
	_, err := CreateOrReplaceOwner(db, "admin", "short", false)
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("got %v, want ErrWeakPassword", err)
	}
	_, err = CreateOrReplaceOwner(db, "admin", "               ", false)
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("whitespace: %v", err)
	}
	_, err = CreateOrReplaceOwner(db, "admin", "admin", false)
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("username-equal: %v", err)
	}
	_, err = CreateOrReplaceOwner(db, "Administrator1", "Administrator1", false)
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("username-equal case: %v", err)
	}
	if err := ValidatePassword("Administrator1", "administrator1"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("folded username-equal: %v", err)
	}
}

func TestCreateOwnerRefusesOverwriteWithoutReplace(t *testing.T) {
	db := testDB(t)
	first, err := CreateOrReplaceOwner(db, "admin", testPassword, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CreateOrReplaceOwner(db, "other", testPassword+"x", false)
	if !errors.Is(err, ErrOwnerExists) {
		t.Fatalf("got %v, want ErrOwnerExists", err)
	}
	got, err := GetOwner(db)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "admin" || got.PasswordHash != first.PasswordHash {
		t.Fatal("owner mutated without --replace")
	}
}

func TestCreateOwnerReplaceUpdatesHashAndDeletesSessions(t *testing.T) {
	db := testDB(t)
	owner := mustOwner(t, db)
	if _, err := CreateSession(db, owner.ID, hour, 2*hour, nowUTC()); err != nil {
		t.Fatal(err)
	}
	replaced, err := CreateOrReplaceOwner(db, "operator", testPassword+"2", true)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Username != "operator" {
		t.Fatalf("username = %q", replaced.Username)
	}
	if replaced.PasswordHash == owner.PasswordHash {
		t.Fatal("hash unchanged after replace")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("sessions remaining = %d", n)
	}
}

func TestPasswordTooLongRejected(t *testing.T) {
	if err := ValidatePassword(strings.Repeat("a", MaxPasswordBytes+1), "admin"); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("got %v", err)
	}
}

func TestUsernameValidation(t *testing.T) {
	if err := ValidateUsername(""); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("empty: %v", err)
	}
	if err := ValidateUsername("ad\x00min"); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("control: %v", err)
	}
	long := strings.Repeat("a", MaxUsernameRunes+1)
	if utf8.RuneCountInString(long) <= MaxUsernameRunes {
		t.Fatal("fixture")
	}
	if err := ValidateUsername(long); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("long: %v", err)
	}
}
