package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCollectMixedStateOKPostgresUnavailable(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return errors.New("boom") },
		nil,
		nil,
		false,
	)
	if len(got) < 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].ID != "redgres_state" || got[0].State != "ok" || got[0].Reason != "" {
		t.Fatalf("state = %#v", got[0])
	}
	if got[1].ID != "postgres_direct" || got[1].State != "unavailable" || got[1].Reason != "unreachable" {
		t.Fatalf("postgres = %#v", got[1])
	}
}

func TestCollectReverseMixedStateUnavailablePostgresOK(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return errors.New("boom") },
		func(context.Context) error { return nil },
		nil,
		nil,
		false,
	)
	if got[0].ID != "redgres_state" || got[0].State != "unavailable" || got[0].Reason != "unreachable" {
		t.Fatalf("state = %#v", got[0])
	}
	if got[1].ID != "postgres_direct" || got[1].State != "ok" || got[1].Reason != "" {
		t.Fatalf("postgres = %#v", got[1])
	}
}

func TestCollectNilPostgresPingIsNotConfigured(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		nil,
		nil,
		nil,
		false,
	)
	if got[1].ID != "postgres_direct" || got[1].State != "not_configured" || got[1].Reason != "" {
		t.Fatalf("postgres = %#v", got[1])
	}
}

func TestCollectPostgresNotConfiguredError(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return ErrNotConfigured },
		nil,
		nil,
		false,
	)
	if got[1].ID != "postgres_direct" || got[1].State != "not_configured" || got[1].Reason != "" {
		t.Fatalf("postgres = %#v", got[1])
	}
}

func TestCollectNilRedisPingIsNotConfigured(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		nil,
		nil,
		false,
	)
	if got[3].ID != "redis" || got[3].State != "not_configured" || got[3].Reason != "" {
		t.Fatalf("redis = %#v", got[3])
	}
}

func TestCollectRedisNotConfiguredError(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		nil,
		func(context.Context) error { return ErrNotConfigured },
		false,
	)
	if got[3].ID != "redis" || got[3].State != "not_configured" || got[3].Reason != "" {
		t.Fatalf("redis = %#v", got[3])
	}
}

func TestCollectRedisUnavailable(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		nil,
		func(context.Context) error { return errors.New("boom") },
		false,
	)
	if got[3].ID != "redis" || got[3].State != "unavailable" || got[3].Reason != "unreachable" {
		t.Fatalf("redis = %#v", got[3])
	}
}

func TestCollectRedisOK(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		nil,
		func(context.Context) error { return nil },
		false,
	)
	if got[3].ID != "redis" || got[3].State != "ok" || got[3].Reason != "" {
		t.Fatalf("redis = %#v", got[3])
	}
}

func TestCollectPostgresAndRedisIndependent(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return errors.New("pg-down") },
		nil,
		func(context.Context) error { return nil },
		false,
	)
	if got[1].ID != "postgres_direct" || got[1].State != "unavailable" || got[1].Reason != "unreachable" {
		t.Fatalf("postgres = %#v", got[1])
	}
	if got[3].ID != "redis" || got[3].State != "ok" || got[3].Reason != "" {
		t.Fatalf("redis = %#v", got[3])
	}
}

func TestCollectReturnsFixedFiveComponents(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		nil,
		nil,
		false,
	)
	wantIDs := []string{"redgres_state", "postgres_direct", "pgbouncer", "redis", "tool_links"}
	wantStates := []string{"ok", "ok", "not_configured", "not_configured", "not_configured"}
	if len(got) != 5 {
		t.Fatalf("len = %d %#v", len(got), got)
	}
	for i, id := range wantIDs {
		if got[i].ID != id || got[i].State != wantStates[i] || got[i].Reason != "" {
			t.Fatalf("index %d = %#v want id=%s state=%s", i, got[i], id, wantStates[i])
		}
	}
	if got[2].State == "not_implemented" {
		t.Fatalf("pgbouncer must not emit not_implemented: %#v", got[2])
	}
}

func TestCollectPostgresOKPgbouncerUnavailable(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		func(context.Context) error { return errors.New("pgbouncer-down") },
		nil,
		false,
	)
	if got[1].ID != "postgres_direct" || got[1].State != "ok" || got[1].Reason != "" {
		t.Fatalf("postgres = %#v", got[1])
	}
	if got[2].ID != "pgbouncer" || got[2].State != "unavailable" || got[2].Reason != "unreachable" {
		t.Fatalf("pgbouncer = %#v", got[2])
	}
}

func TestCollectPostgresUnavailablePgbouncerOK(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return errors.New("pg-down") },
		func(context.Context) error { return nil },
		nil,
		false,
	)
	if got[1].ID != "postgres_direct" || got[1].State != "unavailable" || got[1].Reason != "unreachable" {
		t.Fatalf("postgres = %#v", got[1])
	}
	if got[2].ID != "pgbouncer" || got[2].State != "ok" || got[2].Reason != "" {
		t.Fatalf("pgbouncer = %#v", got[2])
	}
}

func TestCollectNilPgbouncerPingIsNotConfigured(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		nil,
		nil,
		false,
	)
	if got[2].ID != "pgbouncer" || got[2].State != "not_configured" || got[2].Reason != "" {
		t.Fatalf("pgbouncer = %#v", got[2])
	}
}

func TestCollectPgbouncerNotConfiguredError(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		func(context.Context) error { return ErrNotConfigured },
		nil,
		false,
	)
	if got[2].ID != "pgbouncer" || got[2].State != "not_configured" || got[2].Reason != "" {
		t.Fatalf("pgbouncer = %#v", got[2])
	}
}

func TestCollectOmitsCanaryHostAndPassword(t *testing.T) {
	canary := errors.New("password=canary-secret host=10.0.0.1 PgBouncer 1.24.1")
	got := Collect(context.Background(),
		func(context.Context) error { return canary },
		func(context.Context) error { return canary },
		func(context.Context) error { return canary },
		func(context.Context) error { return canary },
		false,
	)
	for _, item := range got {
		blob := item.ID + item.State + item.Reason
		if strings.Contains(blob, "canary-secret") || strings.Contains(blob, "10.0.0.1") || strings.Contains(blob, "password=") || strings.Contains(blob, "host=") || strings.Contains(blob, "PgBouncer 1.24.1") {
			t.Fatalf("leaked canary in %#v", item)
		}
	}
}

func TestCollectToolLinksNotConfiguredWhenFalse(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		nil,
		nil,
		nil,
		false,
	)
	if got[4].ID != "tool_links" || got[4].State != "not_configured" || got[4].Reason != "" {
		t.Fatalf("tool_links = %#v", got[4])
	}
	if got[4].State == "unavailable" || got[4].State == "not_implemented" {
		t.Fatalf("tool_links must not be unavailable or not_implemented: %#v", got[4])
	}
}

func TestCollectToolLinksOKWhenTrue(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		nil,
		nil,
		nil,
		true,
	)
	if got[4].ID != "tool_links" || got[4].State != "ok" || got[4].Reason != "" {
		t.Fatalf("tool_links = %#v", got[4])
	}
}

func TestCollectPostgresUnavailableKeepsToolLinksOK(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return errors.New("pg-down") },
		nil,
		nil,
		true,
	)
	if got[1].ID != "postgres_direct" || got[1].State != "unavailable" || got[1].Reason != "unreachable" {
		t.Fatalf("postgres = %#v", got[1])
	}
	if got[4].ID != "tool_links" || got[4].State != "ok" || got[4].Reason != "" {
		t.Fatalf("tool_links = %#v", got[4])
	}
}

func TestCollectToolLinksIndependentOfRedis(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		nil,
		nil,
		func(context.Context) error { return errors.New("redis-down") },
		true,
	)
	if got[3].ID != "redis" || got[3].State != "unavailable" {
		t.Fatalf("redis = %#v", got[3])
	}
	if got[4].ID != "tool_links" || got[4].State != "ok" || got[4].Reason != "" {
		t.Fatalf("tool_links = %#v", got[4])
	}
}
