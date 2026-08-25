package postgresadmin

import (
	"strings"
	"testing"
)

func TestMaskedProjectConnectionURLBothPorts(t *testing.T) {
	direct, err := MaskedProjectConnectionURL("db.example.com", "5432", "project_a_role", "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if direct != "postgresql://project_a_role:********@db.example.com:5432/project_a?sslmode=require" {
		t.Fatalf("direct = %q", direct)
	}
	pooled, err := MaskedProjectConnectionURL("db.example.com", "6432", "project_a_role", "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if pooled != "postgresql://project_a_role:********@db.example.com:6432/project_a?sslmode=require" {
		t.Fatalf("pooled = %q", pooled)
	}
	if !strings.Contains(direct, "********") || !strings.Contains(pooled, "********") {
		t.Fatal("password slot must be eight asterisks")
	}
	if strings.Contains(direct, "sslmode=prefer") || strings.Contains(pooled, "sslmode=prefer") {
		t.Fatal("must hardcode sslmode=require, not prefer")
	}
}

func TestMaskedProjectConnectionURLEncodesOwner(t *testing.T) {
	got, err := MaskedProjectConnectionURL("db.example.com", "5432", "app/user:name", "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "app%2Fuser%3Aname") {
		t.Fatalf("owner encoding = %q", got)
	}
	if strings.Contains(got, "app/user:name") {
		t.Fatalf("owner must be percent-encoded: %q", got)
	}
	spaced, err := MaskedProjectConnectionURL("db.example.com", "5432", "app user", "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(spaced, "app%20user") {
		t.Fatalf("space encoding = %q", spaced)
	}
	if strings.Contains(spaced, "app+user") {
		t.Fatalf("space must be %%20 not +: %q", spaced)
	}
}

func TestMaskedProjectConnectionURLNeverCopiesAdminHost(t *testing.T) {
	got, err := MaskedProjectConnectionURL("db.example.com", "5432", "project_a_role", "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "10.0.0.1") {
		t.Fatalf("must not copy admin host: %q", got)
	}
}

func TestMaskedProjectConnectionURLRequiresHostAndPort(t *testing.T) {
	if _, err := MaskedProjectConnectionURL("", "5432", "project_a_role", "project_a"); err == nil {
		t.Fatal("expected missing host to fail")
	}
	if _, err := MaskedProjectConnectionURL("db.example.com", "", "project_a_role", "project_a"); err == nil {
		t.Fatal("expected missing port to fail")
	}
	if _, err := MaskedProjectConnectionURL("  ", "5432", "project_a_role", "project_a"); err == nil {
		t.Fatal("expected whitespace host to fail")
	}
	if _, err := MaskedProjectConnectionURL("db.example.com", "  ", "project_a_role", "project_a"); err == nil {
		t.Fatal("expected whitespace port to fail")
	}
}
