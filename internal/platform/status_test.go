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
	)
	if got[3].ID != "redis" || got[3].State != "not_configured" || got[3].Reason != "" {
		t.Fatalf("redis = %#v", got[3])
	}
}

func TestCollectRedisNotConfiguredError(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		func(context.Context) error { return ErrNotConfigured },
	)
	if got[3].ID != "redis" || got[3].State != "not_configured" || got[3].Reason != "" {
		t.Fatalf("redis = %#v", got[3])
	}
}

func TestCollectRedisUnavailable(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		func(context.Context) error { return errors.New("boom") },
	)
	if got[3].ID != "redis" || got[3].State != "unavailable" || got[3].Reason != "unreachable" {
		t.Fatalf("redis = %#v", got[3])
	}
}

func TestCollectRedisOK(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
	)
	if got[3].ID != "redis" || got[3].State != "ok" || got[3].Reason != "" {
		t.Fatalf("redis = %#v", got[3])
	}
}

func TestCollectPostgresAndRedisIndependent(t *testing.T) {
	got := Collect(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { return errors.New("pg-down") },
		func(context.Context) error { return nil },
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
	)
	wantIDs := []string{"redgres_state", "postgres_direct", "pgbouncer", "redis", "tool_links"}
	wantStates := []string{"ok", "ok", "not_implemented", "not_configured", "not_configured"}
	if len(got) != 5 {
		t.Fatalf("len = %d %#v", len(got), got)
	}
	for i, id := range wantIDs {
		if got[i].ID != id || got[i].State != wantStates[i] || got[i].Reason != "" {
			t.Fatalf("index %d = %#v want id=%s state=%s", i, got[i], id, wantStates[i])
		}
	}
	if got[2].State != "not_implemented" {
		t.Fatalf("pgbouncer = %#v", got[2])
	}
}

func TestCollectOmitsCanaryHostAndPassword(t *testing.T) {
	canary := errors.New("password=canary-secret host=10.0.0.1")
	got := Collect(context.Background(),
		func(context.Context) error { return canary },
		func(context.Context) error { return canary },
		func(context.Context) error { return canary },
	)
	for _, item := range got {
		blob := item.ID + item.State + item.Reason
		if strings.Contains(blob, "canary-secret") || strings.Contains(blob, "10.0.0.1") || strings.Contains(blob, "password=") || strings.Contains(blob, "host=") {
			t.Fatalf("leaked canary in %#v", item)
		}
	}
}
