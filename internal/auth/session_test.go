package auth

import (
	"bytes"
	"testing"
	"time"
)

const (
	hour = time.Hour
)

func nowUTC() time.Time {
	return time.Now().UTC()
}

func TestSessionStoresOnlyHashes(t *testing.T) {
	db := testDB(t)
	owner := mustOwner(t, db)
	issued, err := CreateSession(db, owner.ID, hour, 2*hour, nowUTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(issued.RawToken) != 64 || len(issued.RawCSRF) != 64 {
		t.Fatalf("token lengths %d %d", len(issued.RawToken), len(issued.RawCSRF))
	}
	stored, err := RawTokenStored(db, issued.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	if stored {
		t.Fatal("raw session token stored")
	}
	stored, err = RawTokenStored(db, issued.RawCSRF)
	if err != nil {
		t.Fatal(err)
	}
	if stored {
		t.Fatal("raw CSRF token stored")
	}
	if !bytes.Equal(issued.TokenHash, HashToken(issued.RawToken)) {
		t.Fatal("token hash mismatch")
	}
	if !bytes.Equal(issued.CSRFHash, HashToken(issued.RawCSRF)) {
		t.Fatal("csrf hash mismatch")
	}
}

func TestSessionIdleAndAbsoluteExpiry(t *testing.T) {
	db := testDB(t)
	owner := mustOwner(t, db)
	now := nowUTC()
	issued, err := CreateSession(db, owner.ID, time.Minute, 2*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LookupSession(db, issued.RawToken, now.Add(30*time.Second)); err != nil {
		t.Fatalf("active: %v", err)
	}
	if _, err := LookupSession(db, issued.RawToken, now.Add(61*time.Second)); err != ErrSessionExpired {
		t.Fatalf("idle: %v", err)
	}

	issued, err = CreateSession(db, owner.ID, time.Hour, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LookupSession(db, issued.RawToken, now.Add(61*time.Second)); err != ErrSessionExpired {
		t.Fatalf("absolute: %v", err)
	}
}

func TestCreateSessionReplacesExisting(t *testing.T) {
	db := testDB(t)
	owner := mustOwner(t, db)
	first, err := CreateSession(db, owner.ID, hour, 2*hour, nowUTC())
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateSession(db, owner.ID, hour, 2*hour, nowUTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LookupSession(db, first.RawToken, nowUTC()); err != ErrSessionNotFound {
		t.Fatalf("old session: %v", err)
	}
	if _, err := LookupSession(db, second.RawToken, nowUTC()); err != nil {
		t.Fatalf("new session: %v", err)
	}
}

func TestCSRFValid(t *testing.T) {
	raw, hash, err := randomToken(CSRFTokenBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !CSRFValid(hash, raw) {
		t.Fatal("expected valid")
	}
	if CSRFValid(hash, "") || CSRFValid(hash, "00") || CSRFValid(nil, raw) {
		t.Fatal("expected reject")
	}
}

func TestSameOrigin(t *testing.T) {
	base := "http://127.0.0.1:8790"
	if !SameOrigin(base, "", base) {
		t.Fatal("origin match")
	}
	if SameOrigin("https://evil.example", "", base) {
		t.Fatal("evil origin")
	}
	if !SameOrigin("", base+"/", base) {
		t.Fatal("referer-only")
	}
	if SameOrigin("", "", base) {
		t.Fatal("missing both")
	}
}

func TestSameOriginAnyAcceptsAdditionalBase(t *testing.T) {
	if !SameOriginAny("https://console.example.com", "", "http://203.0.113.10:8989", "https://console.example.com") {
		t.Fatal("console origin during bootstrap")
	}
	if SameOriginAny("https://evil.example", "", "http://203.0.113.10:8989", "https://console.example.com") {
		t.Fatal("evil origin")
	}
	if SameOriginAny("https://console.example.com", "", "http://203.0.113.10:8989") {
		t.Fatal("console origin without extra base")
	}
}
