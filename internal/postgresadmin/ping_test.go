package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
)

func TestServicePingNilCatalogIsNotConfigured(t *testing.T) {
	svc := NewService(nil, NewPolicy(config.Config{}))
	if err := svc.Ping(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	var nilSvc *Service
	if err := nilSvc.Ping(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil service err = %v", err)
	}
}

func TestServicePingMapsMemoryCatalogPingErr(t *testing.T) {
	svc := NewService(&MemoryCatalog{PingErr: errors.New("password=canary-secret host=10.0.0.1")}, NewPolicy(config.Config{}))
	err := svc.Ping(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), "10.0.0.1") {
		t.Fatalf("leaked canary: %v", err)
	}
}

func TestServicePingPooledNilObserverIsNotConfigured(t *testing.T) {
	svc := NewService(nil, NewPolicy(config.Config{}))
	if err := svc.PingPooled(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	var nilSvc *Service
	if err := nilSvc.PingPooled(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil service err = %v", err)
	}
	mem := NewService(&MemoryCatalog{}, NewPolicy(config.Config{}))
	if err := mem.PingPooled(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("memory without pooled err = %v", err)
	}
}

func TestServicePingPooledMapsMemoryErrorUnavailable(t *testing.T) {
	canary := errors.New("password=canary-secret host=10.0.0.1 dbname=pgbouncer PgBouncer 1.24.1")
	svc := NewService(&MemoryCatalog{PooledConfigured: true, PingPooledErr: canary}, NewPolicy(config.Config{}))
	err := svc.PingPooled(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), "10.0.0.1") || strings.Contains(err.Error(), "PgBouncer 1.24.1") {
		t.Fatalf("leaked canary: %v", err)
	}
}

func TestServiceCatalogStaysOnDirectWhenPooledUnavailable(t *testing.T) {
	cat := &MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
		Tables: map[string][]TableItem{
			"project_a": {{Schema: "public", Name: "items"}},
		},
		TableData: map[string]MemoryTable{
			"project_a.public.items": {Columns: []string{"id"}, Rows: []map[string]any{{"id": int64(1)}}},
		},
		PooledConfigured: true,
		PingPooledErr:    errors.New("password=canary-secret host=10.0.0.1"),
	}
	svc := NewService(cat, NewPolicy(config.Config{
		PostgresDatabase: "postgres",
		PostgresUser:     "redgres_console",
	}))
	listed, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Databases) != 1 || listed.Databases[0].Name != "project_a" {
		t.Fatalf("list = %#v", listed)
	}
	if _, err := svc.Details(context.Background(), "project_a"); err != nil {
		t.Fatalf("details: %v", err)
	}
	if _, err := svc.Tables(context.Background(), "project_a"); err != nil {
		t.Fatalf("tables: %v", err)
	}
	if _, err := svc.Rows(context.Background(), "project_a", "public", "items", "", 0, 50); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if _, err := svc.SecurityOverview(context.Background()); err != nil {
		t.Fatalf("security: %v", err)
	}
	if err := svc.PingPooled(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ping pooled: %v", err)
	}
}
