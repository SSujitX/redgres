package auth

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReauthenticateSuccess(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	if err := Reauthenticate(db, "admin", testPassword, "127.0.0.1", nowUTC()); err != nil {
		t.Fatalf("success: %v", err)
	}
}

func TestReauthenticateMismatchIsDistinct(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	err := Reauthenticate(db, "admin", "wrong-password-xx", "127.0.0.1", nowUTC())
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

	err := Reauthenticate(db, "nobody", "missing-owner-pw", "127.0.0.1", nowUTC())
	if !called {
		t.Fatal("VerifyUnknown not called")
	}
	if gotPassword != "missing-owner-pw" {
		t.Fatalf("VerifyUnknown password = %q", gotPassword)
	}
	if !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("got %v, want ErrReauthRequired", err)
	}
}

func TestReauthenticateLookupFailure(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	_ = db.Close()
	err := Reauthenticate(db, "admin", testPassword, "127.0.0.1", nowUTC())
	if err == nil {
		t.Fatal("expected lookup error")
	}
	if errors.Is(err, ErrReauthRequired) || errors.Is(err, ErrRateLimited) {
		t.Fatalf("lookup failure mapped to %v", err)
	}
}

func TestReauthenticateHasSeparatePersistentRateLimit(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	now := nowUTC()
	for i := 0; i < lockoutThreshold; i++ {
		err := Reauthenticate(db, "admin", "wrong-password-xx", "127.0.0.1", now.Add(time.Duration(i)*time.Second))
		if !errors.Is(err, ErrReauthRequired) {
			t.Fatalf("failure %d = %v", i, err)
		}
	}
	err := Reauthenticate(db, "admin", testPassword, "127.0.0.1", now.Add(time.Duration(lockoutThreshold-1)*time.Second+200*time.Millisecond))
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}

	if err := Reauthenticate(db, "admin", testPassword, "127.0.0.2", now.Add(time.Duration(lockoutThreshold-1)*time.Second+200*time.Millisecond)); err != nil {
		t.Fatalf("login throttling leaked into other reauth client: %v", err)
	}
}

func TestReauthenticateFailsClosedWhenAttemptPersistenceFails(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	if _, err := db.Exec(`DROP TABLE login_attempts`); err != nil {
		t.Fatal(err)
	}
	err := Reauthenticate(db, "admin", "wrong-password-xx", "127.0.0.1", nowUTC())
	if err == nil || errors.Is(err, ErrReauthRequired) || errors.Is(err, ErrRateLimited) {
		t.Fatalf("persistence failure mapped to %v", err)
	}
}

func TestConcurrentReauthenticateReservesFailureBeforeHash(t *testing.T) {
	db := testDB(t)
	mustOwner(t, db)
	const extra = 3
	n := lockoutThreshold + extra
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	now := nowUTC()
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			errs[i] = Reauthenticate(db, "admin", "wrong-password-xx", "10.0.0.8", now)
		}(i)
	}
	wg.Wait()
	mismatch, limited := 0, 0
	for _, err := range errs {
		switch {
		case errors.Is(err, ErrReauthRequired):
			mismatch++
		case errors.Is(err, ErrRateLimited):
			limited++
		default:
			t.Fatalf("err %v", err)
		}
	}
	if mismatch != lockoutThreshold || limited != extra {
		t.Fatalf("mismatch=%d limited=%d", mismatch, limited)
	}
	var stored int
	if err := db.QueryRow(`SELECT COUNT(*) FROM login_attempts WHERE succeeded = 0`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != lockoutThreshold {
		t.Fatalf("stored failures = %d, want %d", stored, lockoutThreshold)
	}
}
