package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
)

func testService(rows []CatalogRow) *Service {
	return NewService(MemoryCatalog{Rows: rows}, NewPolicy(config.Config{
		PostgresDatabase: "postgres",
		PostgresUser:     "redgres_console",
	}))
}

func projectRow(name, owner string) CatalogRow {
	return CatalogRow{Name: name, Owner: owner, AllowConn: true, SizePretty: "12 MB", SizeBytes: 12582912}
}

func TestServiceListFiltersProtected(t *testing.T) {
	svc := testService([]CatalogRow{
		projectRow("postgres", "postgres"),
		projectRow("database_console_vault", "postgres"),
		{Name: "template0", Owner: "postgres", AllowConn: false, IsTemplate: true},
		projectRow("owned_by_admin", "redgres_console"),
		{Name: "no_connect", Owner: "app_role", AllowConn: false},
		projectRow("project_a", "project_a_role"),
	})
	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Databases) != 1 || got.Databases[0].Name != "project_a" || got.Truncated {
		t.Fatalf("list = %#v", got)
	}
}

func TestServiceDetailsCollapsesProtectedAndMissing(t *testing.T) {
	svc := testService([]CatalogRow{projectRow("project_a", "project_a_role")})
	for _, name := range []string{"postgres", "template0", "template1", "database_console_vault", "missing_db"} {
		_, err := svc.Details(context.Background(), name)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	details, err := svc.Details(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if details.SavedCredential.Status != "not_available" || details.SavedCredential.Reason != "vault_not_implemented" {
		t.Fatalf("credential = %#v", details.SavedCredential)
	}
}

func TestServiceUnavailableWithoutCatalog(t *testing.T) {
	svc := NewService(nil, NewPolicy(config.Config{}))
	if _, err := svc.List(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("list: %v", err)
	}
	if _, err := svc.Details(context.Background(), "project_a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("details: %v", err)
	}
}

func TestServiceMapsCanaryErrors(t *testing.T) {
	svc := NewService(MemoryCatalog{Err: errors.New("postgresql://canary:secret@127.0.0.1/db")}, NewPolicy(config.Config{}))
	_, err := svc.List(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("leaked %v", err)
	}
}

func TestServiceRejectsInvalidDetailsName(t *testing.T) {
	svc := testService(nil)
	if _, err := svc.Details(context.Background(), "bad-name"); !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("err = %v", err)
	}
}
