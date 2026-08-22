package auth

import (
	"testing"
	"time"
)

func TestLoginRateLimit(t *testing.T) {
	db := testDB(t)
	store := AttemptStore{DB: db}
	now := nowUTC()
	for i := 0; i < 5; i++ {
		if err := store.Record("admin", "127.0.0.1", false, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	remaining, err := store.LockoutRemaining("Admin", "127.0.0.1", now.Add(4*time.Second+200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if remaining <= 0 {
		t.Fatal("expected lockout after 5 failures")
	}
	if remaining > 2*time.Second {
		t.Fatalf("first lockout remaining %s", remaining)
	}
	if err := store.ClearFailures("admin", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Record("admin", "127.0.0.1", true, now.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	remaining, err = store.LockoutRemaining("admin", "127.0.0.1", now.Add(6*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("cleared lockout remaining %s", remaining)
	}
}
