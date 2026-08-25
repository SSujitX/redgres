package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
)

func TestPrimaryKeySQLUsesInformationSchema(t *testing.T) {
	for _, need := range []string{
		"information_schema.table_constraints",
		"information_schema.key_column_usage",
		"constraint_catalog",
		"constraint_schema",
		"constraint_name",
		"table_schema",
		"table_name",
		"PRIMARY KEY",
		"kcu.ordinal_position",
		"$1",
		"$2",
	} {
		if !strings.Contains(primaryKeySQL, need) {
			t.Fatalf("primaryKeySQL missing %q", need)
		}
	}
	for _, forbidden := range []string{"::regclass", "indisprimary", "LIMIT 1"} {
		if strings.Contains(primaryKeySQL, forbidden) {
			t.Fatalf("primaryKeySQL must not contain %q", forbidden)
		}
	}
}

func TestFormatDeleteRowsSQLQuotesIdentifiers(t *testing.T) {
	got, err := formatDeleteRowsSQL("public", "items", "id", 2)
	if err != nil {
		t.Fatal(err)
	}
	want := `DELETE FROM "public"."items" WHERE "id" IN ($1, $2)`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	hyphen, err := formatDeleteRowsSQL("public", "items", "user-data", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hyphen, `"user-data"`) || strings.Contains(hyphen, "::regclass") {
		t.Fatalf("got %s", hyphen)
	}
	if _, err := formatDeleteRowsSQL("public", "items", "", 1); err == nil {
		t.Fatal("empty PK must fail closed")
	}
	if _, err := formatDeleteRowsSQL("public", "items", "a\x00b", 1); err == nil {
		t.Fatal("NUL PK must fail closed")
	}
	if _, err := formatDeleteRowsSQL("public", "items", "id", 0); err == nil {
		t.Fatal("zero values must fail")
	}
	if _, err := formatDeleteRowsSQL("public", "items", "id", MaxRowDeleteValues+1); err == nil {
		t.Fatal("over cap must fail")
	}
}

func itemsCatalog() *MemoryCatalog {
	return &MemoryCatalog{
		Rows: []CatalogRow{
			projectRow("project_a", "project_a_role"),
			projectRow("postgres", "postgres"),
		},
		TableData: map[string]MemoryTable{
			"project_a.public.items": {
				Columns:    []string{"id", "name"},
				Rows:       []map[string]any{{"id": 1, "name": "a"}},
				PrimaryKey: []string{"id"},
			},
			"project_a.public.no_pk": {
				Columns: []string{"name"},
			},
			"project_a.public.composite": {
				Columns:    []string{"org_id", "id"},
				PrimaryKey: []string{"org_id", "id"},
			},
		},
	}
}

func TestServicePrimaryKey(t *testing.T) {
	cat := itemsCatalog()
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	got, err := svc.PrimaryKey(context.Background(), "project_a", "public", "items")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "id" {
		t.Fatalf("got %#v", got)
	}
	empty, err := svc.PrimaryKey(context.Background(), "project_a", "public", "no_pk")
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("no pk = %#v", empty)
	}
	composite, err := svc.PrimaryKey(context.Background(), "project_a", "public", "composite")
	if err != nil {
		t.Fatal(err)
	}
	if len(composite) != 2 || composite[0] != "org_id" || composite[1] != "id" {
		t.Fatalf("composite = %#v", composite)
	}
	if _, err := svc.PrimaryKey(context.Background(), "project_a", "public", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestServicePrimaryKeyProtectedNoCatalog(t *testing.T) {
	cat := itemsCatalog()
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	if _, err := svc.PrimaryKey(context.Background(), "postgres", "public", "items"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("protected: %v", err)
	}
	if cat.LastPrimaryKeyKey != "" {
		t.Fatalf("PrimaryKey ran for protected db: %q", cat.LastPrimaryKeyKey)
	}
}

func TestServiceDeleteRowsProtectedNoDML(t *testing.T) {
	cat := itemsCatalog()
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	if _, err := svc.DeleteRows(context.Background(), "postgres", "public", "items", []any{"1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("protected: %v", err)
	}
	if cat.DeleteRowsCalls != 0 || cat.LastPrimaryKeyKey != "" {
		t.Fatalf("DML/PK on protected: calls=%d pk=%q", cat.DeleteRowsCalls, cat.LastPrimaryKeyKey)
	}
}

func TestServiceDeleteRowsNoPKNoDML(t *testing.T) {
	cat := itemsCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.DeleteRows(context.Background(), "project_a", "public", "no_pk", []any{"1"})
	var field FieldError
	if !errors.As(err, &field) || field.Field != "primary_key" || field.Message != missingSingleColumnPKMessage {
		t.Fatalf("err = %#v", err)
	}
	if cat.DeleteRowsCalls != 0 {
		t.Fatalf("DML without single-column PK: %d", cat.DeleteRowsCalls)
	}
}

func TestServiceDeleteRowsCompositeNoDML(t *testing.T) {
	cat := itemsCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.DeleteRows(context.Background(), "project_a", "public", "composite", []any{"1"})
	var field FieldError
	if !errors.As(err, &field) || field.Field != "primary_key" {
		t.Fatalf("err = %#v", err)
	}
	if cat.DeleteRowsCalls != 0 {
		t.Fatalf("DML on composite PK: %d", cat.DeleteRowsCalls)
	}
}

func TestServiceDeleteRowsSuccess(t *testing.T) {
	cat := itemsCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	n, err := svc.DeleteRows(context.Background(), "project_a", "public", "items", []any{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d", n)
	}
	if cat.DeleteRowsCalls != 1 || cat.LastDeleteColumn != "id" || len(cat.LastDeleteValues) != 2 {
		t.Fatalf("delete = key=%q col=%q vals=%#v calls=%d", cat.LastDeleteKey, cat.LastDeleteColumn, cat.LastDeleteValues, cat.DeleteRowsCalls)
	}
}

func TestServiceDeleteRowsMapsCanaryErrors(t *testing.T) {
	cat := itemsCatalog()
	cat.DeleteRowsErr = errors.New("postgresql://canary:secret@127.0.0.1/db")
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.DeleteRows(context.Background(), "project_a", "public", "items", []any{"1"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("leaked %v", err)
	}
}
