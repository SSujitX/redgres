package postgresadmin

import (
	"context"
	"errors"
	"fmt"
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
	if _, err := svc.SecurityOverview(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("security: %v", err)
	}
	var nilSvc *Service
	if _, err := nilSvc.SecurityOverview(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil service: %v", err)
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
	_, err = svc.SecurityOverview(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("security: %v", err)
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("security leaked %v", err)
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

func securityOverviewCatalog() *MemoryCatalog {
	return &MemoryCatalog{
		Rows: []CatalogRow{
			{
				Name: "postgres", Owner: "postgres", AllowConn: true, ConnectionCount: 1,
				OwnerIsSuperuser: true, OwnerCanLogin: true, OwnerCreatedb: true, OwnerCreaterole: true, OwnerReplication: true,
			},
			{Name: "database_console_vault", Owner: "postgres", AllowConn: true},
			{Name: "template0", Owner: "postgres", AllowConn: false, IsTemplate: true},
			{Name: "template1", Owner: "postgres", AllowConn: true, IsTemplate: true},
			{
				Name: "project_a", Owner: "project_a_role", AllowConn: true, PublicCanConnect: true, ConnectionCount: 2,
				OwnerCanLogin: true,
			},
			{Name: "no_connect", Owner: "app_role", AllowConn: false, PublicCanConnect: true},
			{Name: "owned_by_admin", Owner: "redgres_console", AllowConn: true},
			{Name: "zeta_last", Owner: "project_z_role", AllowConn: true},
		},
		Connections: []ConnectionGroup{
			{Database: "project_a", User: "project_a_role", Client: "10.0.0.1", Application: "app", State: "active", Count: 2},
			{Database: "postgres", User: "postgres", Client: "local", Application: "redgres", State: "idle", Count: 1},
		},
	}
}

func TestServiceSecurityOverviewOmitsTemplatesIncludesProtected(t *testing.T) {
	svc := NewService(securityOverviewCatalog(), NewPolicy(config.Config{
		PostgresDatabase: "postgres",
		PostgresUser:     "redgres_console",
	}))
	got, err := svc.SecurityOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.SavedCredential.Status != "not_available" || got.SavedCredential.Reason != "vault_not_implemented" {
		t.Fatalf("credential = %#v", got.SavedCredential)
	}
	if got.Databases == nil || got.Connections == nil {
		t.Fatalf("arrays must not be nil: %#v", got)
	}
	if got.Truncated {
		t.Fatal("truncated")
	}
	if got.Summary != (SecuritySummary{DatabaseCount: 6, PublicConnectCount: 2, ActiveConnectionCount: 3, ConnectionGroupCount: 2}) {
		t.Fatalf("summary = %#v", got.Summary)
	}
	names := make([]string, len(got.Databases))
	protected := map[string]bool{}
	for i, row := range got.Databases {
		names[i] = row.Name
		protected[row.Name] = row.Protected
	}
	wantNames := []string{"database_console_vault", "no_connect", "owned_by_admin", "postgres", "project_a", "zeta_last"}
	if strings.Join(names, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("names = %v", names)
	}
	if protected["postgres"] != true || protected["database_console_vault"] != true || protected["no_connect"] != true || protected["owned_by_admin"] != true {
		t.Fatalf("protected flags = %#v", protected)
	}
	if protected["project_a"] || protected["zeta_last"] {
		t.Fatalf("manageable marked protected: %#v", protected)
	}
	listed, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Databases) != 2 || listed.Databases[0].Name != "project_a" {
		t.Fatalf("list must still omit protected: %#v", listed)
	}
	var vault, project SecurityDatabase
	for _, row := range got.Databases {
		switch row.Name {
		case "database_console_vault":
			vault = row
		case "project_a":
			project = row
		}
	}
	if vault.Owner != "postgres" || vault.PublicCanConnect || vault.ActiveConnections != 0 {
		t.Fatalf("vault row = %#v", vault)
	}
	if project.Owner != "project_a_role" || !project.PublicCanConnect || !project.OwnerCanLogin || project.ActiveConnections != 2 {
		t.Fatalf("project row = %#v", project)
	}
	if len(got.Connections) != 2 || got.Connections[0].Database != "postgres" || got.Connections[1].Count != 2 {
		t.Fatalf("connections = %#v", got.Connections)
	}
}

func TestServiceSecurityOverviewEmptyArraysAreNonNil(t *testing.T) {
	got, err := testService(nil).SecurityOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Databases == nil || len(got.Databases) != 0 || got.Connections == nil || len(got.Connections) != 0 {
		t.Fatalf("empty = %#v", got)
	}
	if got.SavedCredential.Reason != "vault_not_implemented" {
		t.Fatalf("credential = %#v", got.SavedCredential)
	}
}

func TestServiceSecurityOverviewNormalizesConnectionLabels(t *testing.T) {
	svc := NewService(&MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
		Connections: []ConnectionGroup{
			{Count: 4},
		},
	}, NewPolicy(config.Config{}))
	got, err := svc.SecurityOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Connections) != 1 {
		t.Fatalf("connections = %#v", got.Connections)
	}
	row := got.Connections[0]
	if row.Database != "(none)" || row.User != "(unknown)" || row.Client != "local" || row.Application != "—" || row.State != "unknown" || row.Count != 4 {
		t.Fatalf("labels = %#v", row)
	}
	if got.Summary.ActiveConnectionCount != 4 || got.Summary.ConnectionGroupCount != 1 {
		t.Fatalf("summary = %#v", got.Summary)
	}
}

func TestServiceSecurityOverviewCapsAndTruncates(t *testing.T) {
	rows := make([]CatalogRow, 0, 502)
	rows = append(rows, CatalogRow{Name: "template0", Owner: "postgres", IsTemplate: true})
	for i := 0; i < 501; i++ {
		rows = append(rows, projectRow("db"+itoa3(i), "role_"+itoa3(i)))
	}
	groups := make([]ConnectionGroup, 501)
	for i := range groups {
		groups[i] = ConnectionGroup{Database: "db" + itoa3(i), User: "u", Client: "local", Application: "a", State: "idle", Count: 2}
	}
	got, err := NewService(&MemoryCatalog{Rows: rows, Connections: groups}, NewPolicy(config.Config{})).SecurityOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Databases) != 500 || len(got.Connections) != 500 {
		t.Fatalf("cap = truncated=%v dbs=%d conns=%d", got.Truncated, len(got.Databases), len(got.Connections))
	}
	if got.Summary.DatabaseCount != 501 || got.Summary.PublicConnectCount != 0 || got.Summary.ActiveConnectionCount != 1002 || got.Summary.ConnectionGroupCount != 501 {
		t.Fatalf("summary before cap = %#v", got.Summary)
	}
	if got.Databases[0].Name != "db000" || got.Databases[499].Name != "db499" {
		t.Fatalf("db order = %s..%s", got.Databases[0].Name, got.Databases[499].Name)
	}
}

func TestServiceSecurityOverviewTruncatesIfEitherListExceeds(t *testing.T) {
	rows := []CatalogRow{projectRow("project_a", "project_a_role")}
	groups := make([]ConnectionGroup, 501)
	for i := range groups {
		groups[i] = ConnectionGroup{Database: "g" + itoa3(i), User: "u", Client: "local", Application: "a", State: "idle", Count: 1}
	}
	got, err := NewService(&MemoryCatalog{Rows: rows, Connections: groups}, NewPolicy(config.Config{})).SecurityOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Databases) != 1 || len(got.Connections) != 500 || got.Summary.ConnectionGroupCount != 501 {
		t.Fatalf("either cap = %#v", got)
	}
}

func TestServiceSecurityOverviewMapsConnectionCanaryErrors(t *testing.T) {
	svc := NewService(&MemoryCatalog{
		Rows:           []CatalogRow{projectRow("project_a", "project_a_role")},
		ConnectionsErr: errors.New("postgresql://canary:secret@127.0.0.1/db"),
	}, NewPolicy(config.Config{}))
	_, err := svc.SecurityOverview(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("leaked %v", err)
	}
}

func TestConnectionGroupsSQLDoesNotQueryVault(t *testing.T) {
	blob := strings.ToLower(listConnectionGroupsSQL + catalogSQL)
	for _, needle := range []string{"project_credentials", "encrypted_password", "query_text", "query,", "pg_stat_activity.query"} {
		if strings.Contains(blob, needle) {
			t.Fatalf("forbidden %q in catalog SQL", needle)
		}
	}
	if !strings.Contains(listConnectionGroupsSQL, "backend_type = 'client backend'") {
		t.Fatal("must filter client backends")
	}
	if !strings.Contains(listConnectionGroupsSQL, "pid <> pg_backend_pid()") {
		t.Fatal("must exclude the inspecting backend")
	}
	if !strings.Contains(listConnectionGroupsSQL, "GROUP BY datname, usename, client_addr, application_name, state") {
		t.Fatal("must group like database-app get_security_overview")
	}
}

func itoa3(n int) string {
	return fmt.Sprintf("%03d", n)
}
