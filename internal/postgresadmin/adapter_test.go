package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPooledPoolConfigUsesSimpleProtocolAndPgbouncerDB(t *testing.T) {
	cfg := config.Config{
		PostgresHost:       "127.0.0.1",
		PostgresPort:       "5432",
		PostgresDatabase:   "postgres",
		PostgresUser:       "redgres_console",
		PostgresSSLMode:    "prefer",
		PostgresPooledPort: "6432",
	}
	poolCfg, err := pooledPoolConfig(cfg, "unused-password")
	if err != nil {
		t.Fatal(err)
	}
	if poolCfg.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeSimpleProtocol {
		t.Fatalf("DefaultQueryExecMode = %v want QueryExecModeSimpleProtocol", poolCfg.ConnConfig.DefaultQueryExecMode)
	}
	if poolCfg.ConnConfig.DefaultQueryExecMode == pgx.QueryExecModeExec {
		t.Fatal("must not use QueryExecModeExec (extended protocol)")
	}
	if poolCfg.ConnConfig.Database != "pgbouncer" {
		t.Fatalf("database = %q", poolCfg.ConnConfig.Database)
	}
	if poolCfg.ConnConfig.Port != 6432 {
		t.Fatalf("port = %d", poolCfg.ConnConfig.Port)
	}
	for _, fb := range poolCfg.ConnConfig.Fallbacks {
		if fb.Port != 6432 {
			t.Fatalf("sslmode=prefer fallback port = %d want 6432 (post-parse port loss)", fb.Port)
		}
	}
	if poolCfg.ConnConfig.Host != "127.0.0.1" {
		t.Fatalf("host = %q", poolCfg.ConnConfig.Host)
	}
	if poolCfg.ConnConfig.User != "redgres_console" {
		t.Fatalf("user = %q", poolCfg.ConnConfig.User)
	}
	if poolCfg.MinConns != 0 {
		t.Fatalf("MinConns = %d (startup must not connect)", poolCfg.MinConns)
	}
	if poolCfg.ShouldPing == nil {
		t.Fatal("pooled pool must disable acquire Ping")
	}
	if poolCfg.ShouldPing(context.Background(), pgxpool.ShouldPingParams{IdleDuration: time.Hour}) {
		t.Fatal("ShouldPing must not ping the PgBouncer console")
	}
}

func TestAdminPoolConfigKeepsExtendedProtocolAndCatalogDB(t *testing.T) {
	cfg := config.Config{
		PostgresHost:     "127.0.0.1",
		PostgresPort:     "5432",
		PostgresDatabase: "postgres",
		PostgresUser:     "redgres_console",
		PostgresSSLMode:  "prefer",
	}
	poolCfg, err := adminPoolConfig(cfg, "unused-password")
	if err != nil {
		t.Fatal(err)
	}
	if poolCfg.ConnConfig.DefaultQueryExecMode == pgx.QueryExecModeSimpleProtocol {
		t.Fatal("admin pool must not use QueryExecModeSimpleProtocol")
	}
	if poolCfg.ConnConfig.Database != "postgres" {
		t.Fatalf("database = %q", poolCfg.ConnConfig.Database)
	}
	if poolCfg.ConnConfig.Port != 5432 {
		t.Fatalf("port = %d", poolCfg.ConnConfig.Port)
	}
	for _, fb := range poolCfg.ConnConfig.Fallbacks {
		if fb.Port != 5432 {
			t.Fatalf("sslmode=prefer fallback port = %d want 5432 (post-parse port loss)", fb.Port)
		}
	}
}

func TestPoolConfigPreferFallbackCarriesCustomPort(t *testing.T) {
	cfg := config.Config{
		PostgresHost:       "127.0.0.1",
		PostgresPort:       "55432",
		PostgresDatabase:   "postgres",
		PostgresUser:       "redgres_console",
		PostgresSSLMode:    "prefer",
		PostgresPooledPort: "56432",
	}
	admin, err := adminPoolConfig(cfg, "unused-password")
	if err != nil {
		t.Fatal(err)
	}
	if admin.ConnConfig.Port != 55432 {
		t.Fatalf("admin port = %d want 55432", admin.ConnConfig.Port)
	}
	if len(admin.ConnConfig.Fallbacks) == 0 {
		t.Fatal("sslmode=prefer must produce a plaintext fallback")
	}
	for _, fb := range admin.ConnConfig.Fallbacks {
		if fb.Port != 55432 {
			t.Fatalf("admin fallback port = %d want 55432 (post-parse port loss)", fb.Port)
		}
	}
	pooled, err := pooledPoolConfig(cfg, "unused-password")
	if err != nil {
		t.Fatal(err)
	}
	if pooled.ConnConfig.Port != 56432 {
		t.Fatalf("pooled port = %d want 56432", pooled.ConnConfig.Port)
	}
	for _, fb := range pooled.ConnConfig.Fallbacks {
		if fb.Port != 56432 {
			t.Fatalf("pooled fallback port = %d want 56432 (post-parse port loss)", fb.Port)
		}
	}
}

func TestCatalogSQLStaysOnDirectAndPooledProbeIsShowVersion(t *testing.T) {
	if strings.Contains(catalogSQL, "SHOW VERSION") || strings.Contains(catalogSQL, "pgbouncer") {
		t.Fatal("catalog SQL must stay on the 5432 admin path")
	}
	if pooledShowVersionSQL != "SHOW VERSION" {
		t.Fatalf("pooled probe SQL = %q", pooledShowVersionSQL)
	}
	if strings.Contains(listConnectionGroupsSQL, "SHOW VERSION") {
		t.Fatal("connection groups SQL must stay on 5432")
	}
}

func TestPoolCatalogPingPooledNilObserverIsNotConfigured(t *testing.T) {
	var c PoolCatalog
	if err := c.PingPooled(context.Background()); err != ErrNotConfigured {
		t.Fatalf("err = %v", err)
	}
}

func TestMapVaultErrorMapsMissingCatalogAndUndefinedTable(t *testing.T) {
	for _, code := range []string{"3D000", "42P01"} {
		err := mapVaultError(&pgconn.PgError{Code: code, Message: "canary postgresql://secret"})
		if !errors.Is(err, ErrVaultUnavailable) {
			t.Fatalf("%s: %v", code, err)
		}
		if errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s mapped to ErrUnavailable", code)
		}
		if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "postgresql://") {
			t.Fatalf("leaked %v", err)
		}
	}
	if !errors.Is(mapVaultError(ErrUnavailable), ErrVaultUnavailable) {
		t.Fatal("connectTarget ErrUnavailable must remap")
	}
}

func TestPoolCatalogSavedRoleNamesEmptyDoesNotRequirePool(t *testing.T) {
	var c PoolCatalog
	got, err := c.SavedRoleNames(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %#v", got)
	}
}

func TestPoolCatalogSavedRoleNamesNilPoolIsVaultUnavailable(t *testing.T) {
	var c PoolCatalog
	_, err := c.SavedRoleNames(context.Background(), []string{"project_a_role"})
	if !errors.Is(err, ErrVaultUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, ErrUnavailable) {
		t.Fatal("must not return ErrUnavailable")
	}
}

func TestCheckServerVersionNumConsumesRelease(t *testing.T) {
	ok := []struct {
		raw      string
		expected int
	}{
		{raw: "170011", expected: 0},
		{raw: "180006", expected: 0},
		{raw: "170011", expected: 17},
		{raw: "180006", expected: 18},
	}
	for _, tc := range ok {
		if err := checkServerVersionNum(tc.raw, tc.expected); err != nil {
			t.Fatalf("raw=%s expected=%d: %v", tc.raw, tc.expected, err)
		}
	}

	denied := []struct {
		raw      string
		expected int
	}{
		{raw: "garbage", expected: 0},
		{raw: "160000", expected: 0},
		{raw: "190000", expected: 0},
		{raw: "180006", expected: 17},
		{raw: "170011", expected: 18},
		{raw: "180006", expected: 19},
		{raw: "", expected: 0},
	}
	for _, tc := range denied {
		err := checkServerVersionNum(tc.raw, tc.expected)
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("raw=%s expected=%d: err=%v", tc.raw, tc.expected, err)
		}
	}
}
