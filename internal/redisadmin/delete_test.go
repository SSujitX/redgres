package redisadmin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const deleteACLLine = "user project_a on #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 ~project_a:* -@all +echo +get +ping"

func TestDeleteUserRemovesExactUsername(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{
		deleteACLLine,
		"user other_app on ~other:* -@all +ping",
	}}
	svc := NewService(mem)
	if err := svc.DeleteUser(context.Background(), "project_a"); err != nil {
		t.Fatal(err)
	}
	if len(mem.ACLDelUserCalls) != 1 || mem.ACLDelUserCalls[0] != "project_a" {
		t.Fatalf("DELUSER calls = %#v", mem.ACLDelUserCalls)
	}
	if _, err := svc.GetUser(context.Background(), "project_a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted user still present: %v", err)
	}
	if _, err := svc.GetUser(context.Background(), "other_app"); err != nil {
		t.Fatalf("sibling user removed: %v", err)
	}
}

func TestDeleteUserRejectsProtectedWithoutDelUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{
		"user default on nopass ~* &* +@all",
		"user admin on ~* -@all +ping",
		"user redact_admin on ~* -@all +ping",
		"user ops_admin on ~* -@all +ping",
		"user project_a on ~project_a:* -@all +ping",
	}}
	svc := NewServiceAdmin(mem, "ops_admin")
	for _, name := range []string{"default", "admin", "redact_admin", "ops_admin", "OPS_ADMIN"} {
		if err := svc.DeleteUser(context.Background(), name); !errors.Is(err, ErrProtectedUser) {
			t.Fatalf("%s err = %v", name, err)
		}
	}
	if mem.ACLListCalls != 0 {
		t.Fatalf("ACL LIST on protected: %d", mem.ACLListCalls)
	}
	if len(mem.ACLDelUserCalls) != 0 {
		t.Fatalf("DELUSER on protected: %#v", mem.ACLDelUserCalls)
	}
}

func TestDeleteUserMissingUserDoesNotDelUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{deleteACLLine}}
	svc := NewService(mem)
	if err := svc.DeleteUser(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(mem.ACLDelUserCalls) != 0 {
		t.Fatalf("DELUSER on missing: %#v", mem.ACLDelUserCalls)
	}
}

func TestDeleteUserZeroCountIsNotFound(t *testing.T) {
	n := int64(0)
	mem := &MemoryClient{ACLLines: []string{deleteACLLine}, ACLDelUserN: &n}
	svc := NewService(mem)
	if err := svc.DeleteUser(context.Background(), "project_a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(mem.ACLDelUserCalls) != 1 || mem.ACLDelUserCalls[0] != "project_a" {
		t.Fatalf("DELUSER calls = %#v", mem.ACLDelUserCalls)
	}
}

func TestDeleteUserMapsDelUserErrorWithoutCanary(t *testing.T) {
	mem := &MemoryClient{
		ACLLines:      []string{deleteACLLine},
		ACLDelUserErr: errors.New("ERR ACL DELUSER canary-secret: unknown user"),
	}
	svc := NewService(mem)
	err := svc.DeleteUser(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), "DELUSER") {
		t.Fatalf("leaked Redis error: %v", err)
	}
}

func TestDeleteUserNilClientIsNotConfigured(t *testing.T) {
	svc := NewService(nil)
	if err := svc.DeleteUser(context.Background(), "project_a"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}
