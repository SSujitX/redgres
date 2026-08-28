package auth

import (
	"errors"
	"testing"
	"time"
)

func TestChangeOwnerPasswordSucceedsAndInvalidatesSessions(t *testing.T) {
	db := testDB(t)
	owner := mustOwner(t, db)

	if _, err := CreateSession(db, owner.ID, time.Hour, 24*time.Hour, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	const newPassword = "new-owner-secret-17"
	if err := ChangeOwnerPassword(db, "admin", testPassword, newPassword, "127.0.0.1", "req-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	updated, err := GetOwner(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(updated.PasswordHash, testPassword); !errors.Is(err, ErrMismatchedHash) {
		t.Fatalf("old password still verifies: %v", err)
	}
	if err := Verify(updated.PasswordHash, newPassword); err != nil {
		t.Fatalf("new password does not verify: %v", err)
	}

	var sessions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("sessions remaining = %d, want 0", sessions)
	}

	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action = ? AND outcome = ?`, "owner.password_change", "success").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("success audit rows = %d, want 1", audits)
	}
}

func TestChangeOwnerPasswordRejectsWrongCurrent(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	err := ChangeOwnerPassword(db, "admin", "wrong-current-pass", "new-owner-secret-17", "127.0.0.1", "req-1", time.Now().UTC())
	if !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("err = %v, want ErrReauthRequired", err)
	}
}

func TestChangeOwnerPasswordRejectsSamePassword(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	err := ChangeOwnerPassword(db, "admin", testPassword, testPassword, "127.0.0.1", "req-1", time.Now().UTC())
	if !errors.Is(err, ErrSamePassword) {
		t.Fatalf("err = %v, want ErrSamePassword", err)
	}
}

func TestChangeOwnerPasswordRejectsWeakNew(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	err := ChangeOwnerPassword(db, "admin", testPassword, "short", "127.0.0.1", "req-1", time.Now().UTC())
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
}

func TestChangeOwnerPasswordRateLimits(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	now := time.Now().UTC()
	for i := 0; i < lockoutThreshold; i++ {
		err := ChangeOwnerPassword(db, "admin", "wrong-current-pass", "new-owner-secret-17", "127.0.0.1", "req-1", now)
		if !errors.Is(err, ErrReauthRequired) {
			t.Fatalf("attempt %d: err = %v, want ErrReauthRequired", i+1, err)
		}
	}
	err := ChangeOwnerPassword(db, "admin", "wrong-current-pass", "new-owner-secret-17", "127.0.0.1", "req-1", now)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}
func TestChangeOwnerPasswordAuditFailureRollsBack(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)

	// Drop the audit table so the audit INSERT inside the transaction fails,
	// proving the hash update and session deletion roll back with it.
	if _, err := db.Exec(`DROP TABLE audit_events`); err != nil {
		t.Fatal(err)
	}

	err := ChangeOwnerPassword(db, "admin", testPassword, "new-owner-secret-17", "127.0.0.1", "req-1", time.Now().UTC())
	if err == nil {
		t.Fatal("expected audit failure")
	}

	updated, err := GetOwner(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(updated.PasswordHash, testPassword); err != nil {
		t.Fatalf("old password should still verify after rollback: %v", err)
	}
}
