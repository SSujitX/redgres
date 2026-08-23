package postgresadmin

import (
	"testing"

	"github.com/SSujitX/redgres/internal/config"
)

func TestPolicyDeniesHardCodedAndConfigured(t *testing.T) {
	p := NewPolicy(config.Config{
		PostgresDatabase:           "postgres",
		PostgresUser:               "redgres_console",
		PostgresProtectedDatabases: []string{"ops_extra"},
		PostgresProtectedRoles:     []string{"ops_role"},
	})
	deniedDB := []string{"postgres", "template0", "template1", "database_console_vault", "ops_extra"}
	for _, name := range deniedDB {
		if !p.DatabaseDenied(name) {
			t.Fatalf("expected database %q denied", name)
		}
	}
	if p.DatabaseDenied("Postgres") {
		t.Fatal("deny set is case-sensitive")
	}
	deniedOwner := []string{"postgres", "database_console", "onelife_pg_admin", "redgres_console", "ops_role", "pg_signal_backend"}
	for _, owner := range deniedOwner {
		if !p.OwnerDenied(owner) {
			t.Fatalf("expected owner %q denied", owner)
		}
	}
	if p.Manageable("project_a", "project_a_role", true, false) != true {
		t.Fatal("expected project_a manageable")
	}
	if p.Manageable("project_a", "project_a_role", false, false) {
		t.Fatal("datallowconn=false must not be manageable")
	}
	if p.Manageable("project_a", "project_a_role", true, true) {
		t.Fatal("template must not be manageable")
	}
}
