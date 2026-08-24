package redisadmin

import "testing"

func TestValidateUsername(t *testing.T) {
	for _, u := range []string{"abc", "project_a", "worker-1", "n123", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if err := ValidateUsername(u); err != nil {
			t.Fatalf("%q should be valid: %v", u, err)
		}
	}
	invalid := []string{"ab", "A_user", "has space", "has.dot", "", "1", "_leading", "-no", "UPPER", "project.a"}
	for _, u := range invalid {
		if err := ValidateUsername(u); err == nil {
			t.Fatalf("%q should be invalid", u)
		}
	}
}

func TestNormalizePrefix(t *testing.T) {
	got, err := NormalizePrefix("project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got != "project_a:*" {
		t.Fatalf("got %q", got)
	}
	got, err = NormalizePrefix("project_a:*")
	if err != nil {
		t.Fatal(err)
	}
	if got != "project_a:*" {
		t.Fatalf("got %q", got)
	}
	got, err = NormalizePrefix("project_a:")
	if err != nil {
		t.Fatal(err)
	}
	if got != "project_a:*" {
		t.Fatalf("trailing colon got %q", got)
	}
	got, err = NormalizePrefix("ab")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ab:*" {
		t.Fatalf("two-char prefix got %q", got)
	}
	for _, bad := range []string{"", " ", "*", "a*", "project a", "x", string([]byte{0x07}) + "ab", "a", "ab?", "ab[", "ab]"} {
		if _, err := NormalizePrefix(bad); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

func TestProtectedUsernames(t *testing.T) {
	if !IsProtectedUsername("default", "") || !IsProtectedUsername("admin", "") || !IsProtectedUsername("redact_admin", "") {
		t.Fatal("reserved usernames must be protected")
	}
	if !IsProtectedUsername("ops_admin", "ops_admin") {
		t.Fatal("configured redis admin username must be protected")
	}
	if !IsProtectedUsername("OPS_ADMIN", "ops_admin") {
		t.Fatal("EqualFold admin username must be protected")
	}
	if IsProtectedUsername("project_a", "ops_admin") {
		t.Fatal("ordinary username should not be protected")
	}
}
