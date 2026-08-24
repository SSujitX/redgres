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
	if _, err := svc.Rows(context.Background(), "project_a", "public", "items", "", 0, 50); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("rows: %v", err)
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

func TestMemoryCatalogListTablesValidatesName(t *testing.T) {
	cat := &MemoryCatalog{}
	if _, err := cat.ListTables(context.Background(), "bad-name"); !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("err = %v", err)
	}
	if cat.LastTablesDB != "" {
		t.Fatal("invalid name must not record LastTablesDB")
	}
}

func TestListTablesSQLIsBounded(t *testing.T) {
	if !strings.Contains(listTablesSQL, "LIMIT 501") {
		t.Fatal("listTablesSQL must bound the adapter fetch")
	}
	if !strings.Contains(tableSearchPath, "pg_temp") {
		t.Fatal("search_path must include pg_temp so it is not implicit-first")
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
	if _, err := svc.Rows(context.Background(), "bad-name", "public", "items", "", 0, 50); !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("rows db: %v", err)
	}
}

func TestServiceRowsRequiresManageableDatabase(t *testing.T) {
	cat := &MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
		TableData: map[string]MemoryTable{
			"project_a.public.items": {Columns: []string{"id"}, Rows: []map[string]any{{"id": 1}}},
		},
	}
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	got, err := svc.Rows(context.Background(), "project_a", "public", "items", "", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 || got.Total != 1 || got.Limit != 50 {
		t.Fatalf("page = %#v", got)
	}
	if cat.LastRowsKey != "project_a.public.items" {
		t.Fatalf("LastRowsKey = %q", cat.LastRowsKey)
	}
	cat.LastRowsKey = ""
	if _, err := svc.Rows(context.Background(), "postgres", "public", "items", "", 0, 50); !errors.Is(err, ErrNotFound) {
		t.Fatalf("protected: %v", err)
	}
	if cat.LastRowsKey != "" {
		t.Fatal("ListRows must not run for a protected database")
	}
}

func TestServiceRowsClampsLimitAndOffset(t *testing.T) {
	cat := &MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
		TableData: map[string]MemoryTable{
			"project_a.public.items": {
				Columns: []string{"id"},
				Rows:    []map[string]any{{"id": 1}, {"id": 2}},
			},
		},
	}
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.Rows(context.Background(), "project_a", "public", "items", "", -3, 501)
	if err != nil {
		t.Fatal(err)
	}
	if got.Offset != 0 || got.Limit != 50 || len(got.Rows) != 2 {
		t.Fatalf("clamped = %#v", got)
	}
}

func TestServiceRowsEmptyExistingTable(t *testing.T) {
	svc := NewService(&MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
		TableData: map[string]MemoryTable{
			"project_a.public.items": {Columns: []string{"id"}},
		},
	}, NewPolicy(config.Config{}))
	got, err := svc.Rows(context.Background(), "project_a", "public", "items", "", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Columns) != 1 || got.Rows == nil || len(got.Rows) != 0 || got.Total != 0 {
		t.Fatalf("empty = %#v", got)
	}
}

func TestServiceRowsMissingTable(t *testing.T) {
	svc := NewService(&MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
	}, NewPolicy(config.Config{}))
	if _, err := svc.Rows(context.Background(), "project_a", "public", "missing", "", 0, 50); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, err := svc.Rows(context.Background(), "project_a", "pg_catalog", "pg_class", "", 0, 50); !errors.Is(err, ErrNotFound) {
		t.Fatalf("catalog: %v", err)
	}
}

func TestServiceRowsMapsCanaryErrors(t *testing.T) {
	svc := NewService(&MemoryCatalog{
		Rows:    []CatalogRow{projectRow("project_a", "project_a_role")},
		RowsErr: errors.New("postgresql://canary:secret@127.0.0.1/db"),
	}, NewPolicy(config.Config{}))
	_, err := svc.Rows(context.Background(), "project_a", "public", "items", "", 0, 50)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("leaked %v", err)
	}
}

func protectedSearchCatalog() []CatalogRow {
	return []CatalogRow{
		projectRow("postgres", "postgres"),
		projectRow("database_console_vault", "postgres"),
		{Name: "template0", Owner: "postgres", AllowConn: false, IsTemplate: true},
		projectRow("owned_by_admin", "redgres_console"),
		{Name: "no_connect", Owner: "app_role", AllowConn: false},
		projectRow("project_a", "project_a_role"),
	}
}

func TestServiceSearchOmitsProtectedNames(t *testing.T) {
	svc := testService(protectedSearchCatalog())
	got, err := svc.Search(context.Background(), "project", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || got.Hits[0].Name != "project_a" || got.Truncated {
		t.Fatalf("project = %#v", got)
	}
	for _, q := range []string{"postgres", "template", "vault"} {
		empty, err := svc.Search(context.Background(), q, 20)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if len(empty.Hits) != 0 || empty.Truncated {
			t.Fatalf("%s = %#v", q, empty)
		}
	}
	owner, err := svc.Search(context.Background(), "project_a_role", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(owner.Hits) != 0 {
		t.Fatalf("owner match = %#v", owner)
	}
}

func TestServiceSearchIsCaseInsensitiveOnName(t *testing.T) {
	svc := testService(protectedSearchCatalog())
	got, err := svc.Search(context.Background(), "PROJECT", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || got.Hits[0].Name != "project_a" {
		t.Fatalf("search = %#v", got)
	}
}

func TestServiceSearchRespectsLimitAndTruncation(t *testing.T) {
	svc := testService([]CatalogRow{
		projectRow("project_a", "project_a_role"),
		projectRow("project_b", "project_b_role"),
	})
	got, err := svc.Search(context.Background(), "project", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || got.Hits[0].Name != "project_a" || !got.Truncated {
		t.Fatalf("limit = %#v", got)
	}
}

func TestServiceSearchUnavailableWithoutCatalog(t *testing.T) {
	svc := NewService(nil, NewPolicy(config.Config{}))
	if _, err := svc.Search(context.Background(), "project", 20); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	var nilSvc *Service
	if _, err := nilSvc.Search(context.Background(), "project", 20); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil service: %v", err)
	}
}

func TestServiceSearchMapsCanaryErrors(t *testing.T) {
	svc := NewService(&MemoryCatalog{Err: errors.New("postgresql://canary:secret@127.0.0.1/db")}, NewPolicy(config.Config{}))
	_, err := svc.Search(context.Background(), "project", 20)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("leaked %v", err)
	}
}
