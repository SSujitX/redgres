package redisadmin

import (
	"strings"
	"testing"
)

func TestGeneratePasswordEntropy(t *testing.T) {
	pw, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) != 32 {
		t.Fatalf("password length = %d", len(pw))
	}
	pw2, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	if pw == pw2 {
		t.Fatal("passwords must be unique")
	}
}

func TestProjectConnectionURLNeverCopiesAdmin(t *testing.T) {
	got, err := ProjectConnectionURL("redis.example.com", "6380", "project_a", "p@ss:word/x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "rediss://project_a:") {
		t.Fatalf("got %s", got)
	}
	if !strings.HasSuffix(got, "@redis.example.com:6380/0") {
		t.Fatalf("must use public host:port and db 0, got %s", got)
	}
	if strings.Contains(got, "p@ss:word/x") {
		t.Fatal("password must be percent-encoded")
	}
	if strings.Contains(got, "admin") || strings.Contains(got, "canary-secret") || strings.Contains(got, "10.0.0.1") {
		t.Fatal("must not leak administrator credential or host")
	}
}

func TestProjectConnectionURLRequiresHostAndPort(t *testing.T) {
	if _, err := ProjectConnectionURL("", "6380", "project_a", "secret"); err == nil {
		t.Fatal("expected missing host to fail")
	}
	if _, err := ProjectConnectionURL("redis.example.com", "", "project_a", "secret"); err == nil {
		t.Fatal("expected missing port to fail")
	}
}
