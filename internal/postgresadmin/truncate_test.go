package postgresadmin

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
)

func TestFormatTruncateSQLQuotesIdentifiers(t *testing.T) {
	got, err := formatTruncateSQL([]TableItem{
		{Schema: "public", Name: "items"},
		{Schema: "other", Name: "t2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `TRUNCATE TABLE "public"."items", "other"."t2" RESTART IDENTITY`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	hyphen, err := formatTruncateSQL([]TableItem{{Schema: "app-data", Name: "user-data"}})
	if err != nil {
		t.Fatal(err)
	}
	if hyphen != `TRUNCATE TABLE "app-data"."user-data" RESTART IDENTITY` {
		t.Fatalf("got %s", hyphen)
	}
	if strings.Contains(got, "CASCADE") || strings.Contains(got, "CONTINUE IDENTITY") || strings.Contains(got, " ONLY ") {
		t.Fatalf("forbidden clause in %s", got)
	}
	if strings.Count(got, "TRUNCATE") != 1 {
		t.Fatalf("must be one statement: %s", got)
	}
	if _, err := formatTruncateSQL(nil); err == nil {
		t.Fatal("empty table list must fail closed")
	}
	if _, err := formatTruncateSQL([]TableItem{{Schema: "", Name: "items"}}); err == nil {
		t.Fatal("empty schema must fail closed")
	}
	if _, err := formatTruncateSQL([]TableItem{{Schema: "public", Name: "a\x00b"}}); err == nil {
		t.Fatal("NUL table must fail closed")
	}
}

func truncateCatalog() *MemoryCatalog {
	return &MemoryCatalog{
		Rows: []CatalogRow{
			projectRow("project_a", "project_a_role"),
			projectRow("postgres", "postgres"),
			{Name: "closed_db", Owner: "project_a_role", AllowConn: false},
			{Name: "templ_db", Owner: "project_a_role", AllowConn: true, IsTemplate: true},
		},
		Tables: map[string][]TableItem{
			"project_a": {
				{Schema: "public", Name: "items"},
				{Schema: "other", Name: "t2"},
			},
		},
	}
}

func TestServiceTruncateProtectedNoSQL(t *testing.T) {
	cat := truncateCatalog()
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	if _, err := svc.Truncate(context.Background(), "postgres"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("protected: %v", err)
	}
	if cat.TruncateCalls != 0 || cat.LastTablesDB != "" {
		t.Fatalf("SQL on protected: calls=%d tables=%q", cat.TruncateCalls, cat.LastTablesDB)
	}
}

func TestServiceTruncateDisallowConnAndTemplateNoSQL(t *testing.T) {
	cat := truncateCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	if _, err := svc.Truncate(context.Background(), "closed_db"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("allowconn: %v", err)
	}
	if _, err := svc.Truncate(context.Background(), "templ_db"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("template: %v", err)
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL on unmanageable: %d", cat.TruncateCalls)
	}
}

func TestServiceTruncateMissingNoSQL(t *testing.T) {
	cat := truncateCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	if _, err := svc.Truncate(context.Background(), "missing_db"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL on missing: %d", cat.TruncateCalls)
	}
}

func TestServiceTruncateEmptyTables(t *testing.T) {
	cat := truncateCatalog()
	cat.Tables["project_a"] = nil
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.Truncate(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Truncated != 0 || got.TotalTables != 0 || got.Failed == nil || len(got.Failed) != 0 {
		t.Fatalf("got = %#v", got)
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL on empty: %d", cat.TruncateCalls)
	}
}

func TestServiceTruncateTableListCapNoSQL(t *testing.T) {
	cat := truncateCatalog()
	items := make([]TableItem, listCap+1)
	for i := range items {
		items[i] = TableItem{Schema: "public", Name: "t" + strconv.Itoa(i)}
	}
	cat.Tables["project_a"] = items
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.Truncate(context.Background(), "project_a")
	var truncated TableListTruncated
	if !errors.As(err, &truncated) || !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if truncated.Error() != tableListTruncatedMessage {
		t.Fatalf("message = %q", truncated.Error())
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL on truncated list: %d", cat.TruncateCalls)
	}
}

func TestServiceTruncateEmptyCatalogNameNoSQL(t *testing.T) {
	cat := truncateCatalog()
	cat.Tables["project_a"] = []TableItem{{Schema: "", Name: "items"}}
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.Truncate(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL on empty catalog name: %d", cat.TruncateCalls)
	}
}

func TestServiceTruncateSuccess(t *testing.T) {
	cat := truncateCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.Truncate(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Truncated != 2 || got.TotalTables != 2 || got.Failed == nil || len(got.Failed) != 0 {
		t.Fatalf("got = %#v", got)
	}
	if cat.TruncateCalls != 1 || cat.LastTruncateDB != "project_a" || len(cat.LastTruncateTables) != 2 {
		t.Fatalf("truncate = db=%q n=%d calls=%d", cat.LastTruncateDB, len(cat.LastTruncateTables), cat.TruncateCalls)
	}
}

func TestServiceTruncateMapsCanaryErrors(t *testing.T) {
	cat := truncateCatalog()
	cat.TruncateErr = errors.New("postgresql://canary:secret@127.0.0.1/db")
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.Truncate(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("leaked %v", err)
	}
}

func TestServiceTruncateLockConflictNoSecondSQL(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:            []CatalogRow{projectRow("project_a", "project_a_role")},
		Tables:          map[string][]TableItem{"project_a": {{Schema: "public", Name: "items"}}},
		TruncateStarted: make(chan struct{}, 1),
		TruncateHold:    make(chan struct{}),
	}
	svc := NewService(cat, NewPolicy(config.Config{}))
	done := make(chan error, 1)
	go func() {
		_, err := svc.Truncate(context.Background(), "project_a")
		done <- err
	}()
	select {
	case <-cat.TruncateStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first truncate did not reach SQL")
	}
	_, err := svc.Truncate(context.Background(), "project_a")
	var inProgress TruncateInProgress
	if !errors.As(err, &inProgress) || !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("second err = %v", err)
	}
	if inProgress.Error() != truncateInProgressMessage {
		t.Fatalf("message = %q", inProgress.Error())
	}
	close(cat.TruncateHold)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("first truncate: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first truncate blocked")
	}
	if cat.TruncateCalls != 1 {
		t.Fatalf("TruncateCalls = %d", cat.TruncateCalls)
	}
}
