package integration

import (
	"context"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

// TestLivePostgresSecurityOverview executes the PG-012 cluster security
// overview SQL against a real PostgreSQL server. On a stock image the
// manageable database list is empty and the vault database is absent, so the
// overview must still succeed with a vault not_available state and no error.
func TestLivePostgresSecurityOverview(t *testing.T) {
	clearInheritedRedgresEnv(t)
	host, port, passwordFile, ok := livePostgresEnv(t)
	if !ok {
		t.Skip(skipLiveEnv)
	}
	cfg := config.Config{
		Environment:           config.EnvironmentDevelopment,
		PostgresHost:          host,
		PostgresPort:          port,
		PostgresDatabase:      "postgres",
		PostgresUser:          "postgres",
		PostgresPasswordFile:  passwordFile,
		PostgresSSLMode:       "prefer",
		PostgresExpectedMajor: livePostgresExpectedMajor(t),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	svc, closer, err := postgresadmin.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closer()

	ov, err := svc.SecurityOverview(ctx)
	if err != nil {
		t.Fatalf("SecurityOverview: %v", err)
	}
	if ov.Summary.DatabaseCount < 0 || ov.Summary.ActiveConnectionCount < 0 || ov.Summary.ConnectionGroupCount < 0 {
		t.Fatalf("negative summary: %+v", ov.Summary)
	}
	switch ov.SavedCredential.Status {
	case "ok", "present", "missing", "not_available":
	default:
		t.Fatalf("SavedCredential status = %q", ov.SavedCredential.Status)
	}
	t.Logf("security overview: dbs=%d public_connect=%d active=%d groups=%d vault=%s",
		ov.Summary.DatabaseCount, ov.Summary.PublicConnectCount, ov.Summary.ActiveConnectionCount,
		ov.Summary.ConnectionGroupCount, ov.SavedCredential.Status)
}
