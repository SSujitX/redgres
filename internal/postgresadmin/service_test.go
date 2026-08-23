package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
)

func testService(rows []CatalogRow) *Service {
	return NewService(&MemoryCatalog{Rows: rows}, NewPolicy(config.Config{
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
	if _, err := svc.Tables(context.Background(), "project_a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("tables: %v", err)
	}
}

func TestServiceMapsCanaryErrors(t *testing.T) {
	svc := NewService(&MemoryCatalog{Err: errors.New("postgresql://canary:secret@127.0.0.1/db")}, NewPolicy(config.Config{}))
	_, err := svc.List(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("leaked %v", err)
	}
}

func TestServiceTablesRequiresManageableDatabase(t *testing.T) {
	cat := &MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
		Tables: map[string][]TableItem{
			"project_a": {{Schema: "public", Name: "items"}},
		},
	}
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	got, err := svc.Tables(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables) != 1 || got.Tables[0].Name != "items" || got.Truncated {
		t.Fatalf("tables = %#v", got)
	}
	if cat.LastTablesDB != "project_a" {
		t.Fatalf("LastTablesDB = %q", cat.LastTablesDB)
	}
	cat.LastTablesDB = ""
	if _, err := svc.Tables(context.Background(), "postgres"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("protected: %v", err)
	}
	if cat.LastTablesDB != "" {
		t.Fatal("ListTables must not run for a protected name")
	}
}

func TestServiceTablesCapsAt500(t *testing.T) {
	items := make([]TableItem, 501)
	for i := range items {
		items[i] = TableItem{Schema: "public", Name: "t"}
	}
	svc := NewService(&MemoryCatalog{
		Rows:   []CatalogRow{projectRow("project_a", "project_a_role")},
		Tables: map[string][]TableItem{"project_a": items},
	}, NewPolicy(config.Config{}))
	got, err := svc.Tables(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables) != 500 || !got.Truncated {
		t.Fatalf("cap = %#v", got)
	}
}

func TestServiceTablesEmptyManageableDatabase(t *testing.T) {
	got, err := NewService(&MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
	}, NewPolicy(config.Config{})).Tables(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Tables == nil || len(got.Tables) != 0 || got.Truncated {
		t.Fatalf("empty = %#v", got)
	}
}

func TestServiceTablesMapsCanaryErrors(t *testing.T) {
	svc := NewService(&MemoryCatalog{
		Rows:      []CatalogRow{projectRow("project_a", "project_a_role")},
		TablesErr: errors.New("postgresql://canary:secret@127.0.0.1/db"),
	}, NewPolicy(config.Config{}))
	_, err := svc.Tables(context.Background(), "project_a")
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
	if _, err := svc.Tables(context.Background(), "bad-name"); !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("tables: %v", err)
	}
}
