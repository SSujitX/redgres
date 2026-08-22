package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	encoded, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("unexpected encoded form: %s", encoded)
	}
	if err := Verify(encoded, "correct horse battery staple"); err != nil {
		t.Fatalf("verify correct password: %v", err)
	}
}

func TestVerifyRejectsWrongPassword(t *testing.T) {
	encoded, err := Hash("secret-password-ok")
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(encoded, "other-password-ok"); err != ErrMismatchedHash {
		t.Fatalf("got %v, want ErrMismatchedHash", err)
	}
}

func TestUniqueSaltPerHash(t *testing.T) {
	a, err := Hash("same-password-15")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Hash("same-password-15")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("expected unique salts")
	}
}

func TestHashWithCustomParamsRoundTrip(t *testing.T) {
	p := Params{Memory: 32 * 1024, Time: 2, Parallelism: 2, SaltLen: 16, KeyLen: 32}
	encoded, err := HashWithParams("pw", p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded, "m=32768,t=2,p=2") {
		t.Fatalf("params not encoded: %s", encoded)
	}
	if err := Verify(encoded, "pw"); err != nil {
		t.Fatal(err)
	}
}

func TestDummyHashVerifiesWithoutErrorPath(t *testing.T) {
	if DummyHash() == "" {
		t.Fatal("dummy hash missing")
	}
	VerifyUnknown("anything")
}
