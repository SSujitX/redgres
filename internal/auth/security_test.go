package auth

import (
	"errors"
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

func TestLoginRateLimitStopsIPWideUsernameSpray(t *testing.T) {
	db := testDB(t)
	store := AttemptStore{DB: db}
	now := nowUTC()
	for i := 0; i < sprayThreshold; i++ {
		username := string(rune('a' + i))
		if err := store.Record(username, "10.0.0.8", false, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	remaining, err := store.IPLockoutRemaining("10.0.0.8", now.Add(time.Duration(sprayThreshold-1)*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if remaining <= 0 {
		t.Fatal("expected IP-wide lockout after spray threshold")
	}

	otherIP, err := store.IPLockoutRemaining("10.0.0.9", now.Add(time.Duration(sprayThreshold-1)*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if otherIP != 0 {
		t.Fatalf("other IP lockout = %s", otherIP)
	}
}

func TestIPLockoutRemainingSkipsLoopback(t *testing.T) {
	db := testDB(t)
	store := AttemptStore{DB: db}
	now := nowUTC()
	for i := 0; i < sprayThreshold; i++ {
		username := string(rune('a' + i))
		if err := store.Record(username, "127.0.0.1", false, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	remaining, err := store.IPLockoutRemaining("127.0.0.1", now.Add(time.Duration(sprayThreshold-1)*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("loopback spray lockout = %s", remaining)
	}
}

func TestEffectiveClientIP(t *testing.T) {
	tests := []struct {
		name, remote, cf, want string
	}{
		{"remote only", "10.0.0.8:1234", "", "10.0.0.8"},
		{"spoof ignored", "10.0.0.8:1234", "203.0.113.10", "10.0.0.8"},
		{"loopback header", "127.0.0.1:12345", "203.0.113.10", "203.0.113.10"},
		{"loopback ipv6 header", "[::1]:12345", "198.51.100.20", "198.51.100.20"},
		{"loopback no header", "127.0.0.1:12345", "", "127.0.0.1"},
		{"loopback trimmed", "127.0.0.1:9", "  203.0.113.10  ", "203.0.113.10"},
		{"loopback comma", "127.0.0.1:9", "203.0.113.10,198.51.100.1", "127.0.0.1"},
		{"loopback port", "127.0.0.1:9", "203.0.113.10:443", "127.0.0.1"},
		{"loopback empty", "127.0.0.1:9", "   ", "127.0.0.1"},
		{"loopback garbage", "127.0.0.1:9", "not-an-ip", "127.0.0.1"},
		{"loopback ipv6 client", "127.0.0.1:9", "2001:db8::1", "2001:db8::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveClientIP(tt.remote, tt.cf)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestRecordSuccessClearsFailuresAtomically(t *testing.T) {
	db := testDB(t)
	store := AttemptStore{DB: db}
	now := nowUTC()
	for i := 0; i < 3; i++ {
		if err := store.Record("admin", "10.0.0.8", false, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RecordSuccess("admin", "10.0.0.8", now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	remaining, err := store.LockoutRemaining("admin", "10.0.0.8", now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("remaining after success = %s", remaining)
	}
	var failures, successes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM login_attempts WHERE succeeded = 0`).Scan(&failures); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM login_attempts WHERE succeeded = 1`).Scan(&successes); err != nil {
		t.Fatal(err)
	}
	if failures != 0 || successes != 1 {
		t.Fatalf("failures=%d successes=%d", failures, successes)
	}
}

func TestRecordSuccessFailsClosed(t *testing.T) {
	db := testDB(t)
	store := AttemptStore{DB: db}
	if _, err := db.Exec(`DROP TABLE login_attempts`); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordSuccess("admin", "10.0.0.8", nowUTC()); err == nil {
		t.Fatal("expected persistence error")
	}
}

func TestReserveFailureRejectsWhenThresholdExceeded(t *testing.T) {
	db := testDB(t)
	store := AttemptStore{DB: db}
	now := nowUTC()
	for i := 0; i < lockoutThreshold; i++ {
		if err := store.ReserveFailure("admin", "10.0.0.8", now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	err := store.ReserveFailure("admin", "10.0.0.8", now.Add(time.Duration(lockoutThreshold-1)*time.Second+200*time.Millisecond))
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM login_attempts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != lockoutThreshold {
		t.Fatalf("stored attempts = %d, want %d", n, lockoutThreshold)
	}
}

func TestAttemptStoreBoundsPersistedHistory(t *testing.T) {
	db := testDB(t)
	store := AttemptStore{DB: db}
	now := nowUTC()
	for i := 0; i < maxStoredAttempts+5; i++ {
		if err := store.Record("admin", "127.0.0.1", false, now.Add(time.Duration(i)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM login_attempts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != maxStoredAttempts {
		t.Fatalf("stored attempts = %d, want %d", count, maxStoredAttempts)
	}
}
