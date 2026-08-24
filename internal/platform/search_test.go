package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResourceGroupsAlwaysTwoWithEmptyRedis(t *testing.T) {
	got := ResourceGroups(PostgresSearch{
		Status: "ok",
		Names:  []string{"project_a"},
	}, RedisSearch{Status: "not_configured"})
	if len(got) != 2 {
		t.Fatalf("len = %d %#v", len(got), got)
	}
	if got[0].ID != "postgres_databases" || got[0].Status != "ok" || got[0].Service != "postgres" {
		t.Fatalf("postgres = %#v", got[0])
	}
	if len(got[0].Hits) != 1 || got[0].Hits[0].ID != "postgres_database:project_a" || got[0].Hits[0].Type != "postgres_database" || got[0].Hits[0].Label != "project_a" {
		t.Fatalf("hits = %#v", got[0].Hits)
	}
	if got[1].ID != "redis_acl_users" || got[1].Status != "not_configured" || got[1].Truncated || len(got[1].Hits) != 0 {
		t.Fatalf("redis = %#v", got[1])
	}
	if got[1].Hits == nil {
		t.Fatal("redis hits must be empty array not nil")
	}
}

func TestResourceGroupsPostgresUnavailableKeepsRedis(t *testing.T) {
	got := ResourceGroups(PostgresSearch{
		Status: "unavailable",
		Names:  []string{"canary-secret"},
	}, RedisSearch{Status: "not_configured"})
	if got[0].Status != "unavailable" || len(got[0].Hits) != 0 || got[0].Truncated {
		t.Fatalf("postgres = %#v", got[0])
	}
	if got[1].ID != "redis_acl_users" || got[1].Status != "not_configured" || len(got[1].Hits) != 0 {
		t.Fatalf("redis = %#v", got[1])
	}
}

func TestResourceGroupsRedisOkHits(t *testing.T) {
	got := ResourceGroups(PostgresSearch{Status: "not_configured"}, RedisSearch{
		Status: "ok",
		Names:  []string{"project_a"},
	})
	if got[0].Status != "not_configured" || len(got[0].Hits) != 0 {
		t.Fatalf("postgres = %#v", got[0])
	}
	if got[1].ID != "redis_acl_users" || got[1].Status != "ok" || got[1].Service != "redis" {
		t.Fatalf("redis = %#v", got[1])
	}
	if len(got[1].Hits) != 1 {
		t.Fatalf("hits = %#v", got[1].Hits)
	}
	hit := got[1].Hits[0]
	if hit.ID != "redis_acl_user:project_a" || hit.Type != "redis_acl_user" || hit.Label != "project_a" {
		t.Fatalf("hit = %#v", hit)
	}
}

func TestResourceGroupsPostgresDownKeepsRedisOkHits(t *testing.T) {
	got := ResourceGroups(PostgresSearch{
		Status: "unavailable",
		Names:  []string{"should-not-appear"},
	}, RedisSearch{
		Status: "ok",
		Names:  []string{"project_a"},
	})
	if got[0].Status != "unavailable" || len(got[0].Hits) != 0 {
		t.Fatalf("postgres = %#v", got[0])
	}
	if got[1].Status != "ok" || len(got[1].Hits) != 1 || got[1].Hits[0].ID != "redis_acl_user:project_a" {
		t.Fatalf("redis = %#v", got[1])
	}
}

func TestResourceGroupsRedisUnavailableKeepsPostgresHits(t *testing.T) {
	got := ResourceGroups(PostgresSearch{
		Status: "ok",
		Names:  []string{"project_a"},
	}, RedisSearch{
		Status: "unavailable",
		Names:  []string{"should-not-appear"},
	})
	if got[0].Status != "ok" || len(got[0].Hits) != 1 || got[0].Hits[0].Label != "project_a" {
		t.Fatalf("postgres = %#v", got[0])
	}
	if got[1].Status != "unavailable" || len(got[1].Hits) != 0 || got[1].Truncated {
		t.Fatalf("redis = %#v", got[1])
	}
	if got[1].Hits == nil {
		t.Fatal("redis hits must be empty array not nil")
	}
}

func TestResourceGroupsNeverNotImplemented(t *testing.T) {
	cases := []struct {
		name string
		pg   PostgresSearch
		rd   RedisSearch
	}{
		{name: "not_configured", pg: PostgresSearch{Status: "not_configured"}, rd: RedisSearch{Status: "not_configured"}},
		{name: "unavailable", pg: PostgresSearch{Status: "unavailable"}, rd: RedisSearch{Status: "unavailable"}},
		{name: "ok", pg: PostgresSearch{Status: "ok"}, rd: RedisSearch{Status: "ok"}},
		{name: "empty_status", pg: PostgresSearch{}, rd: RedisSearch{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResourceGroups(tc.pg, tc.rd)
			for _, group := range got {
				if group.Status == "not_implemented" {
					t.Fatalf("status not_implemented: %#v", group)
				}
			}
		})
	}
}

func TestResourceGroupsHitFieldsOnlyAndOmitsCanary(t *testing.T) {
	got := ResourceGroups(PostgresSearch{
		Status:    "ok",
		Truncated: true,
		Names:     []string{"project_a"},
	}, RedisSearch{
		Status:    "ok",
		Truncated: true,
		Names:     []string{"project_a"},
	})
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "canary") || strings.Contains(body, "password") || strings.Contains(body, "owner") || strings.Contains(body, "saved_credential") {
		t.Fatalf("unexpected keys: %s", body)
	}
	hitRaw, err := json.Marshal(got[0].Hits[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(hitRaw, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 {
		t.Fatalf("hit fields = %#v", fields)
	}
	if fields["id"] != "postgres_database:project_a" || fields["type"] != "postgres_database" || fields["label"] != "project_a" {
		t.Fatalf("hit = %#v", fields)
	}
	redisHitRaw, err := json.Marshal(got[1].Hits[0])
	if err != nil {
		t.Fatal(err)
	}
	var redisFields map[string]any
	if err := json.Unmarshal(redisHitRaw, &redisFields); err != nil {
		t.Fatal(err)
	}
	if len(redisFields) != 3 {
		t.Fatalf("redis hit fields = %#v", redisFields)
	}
	if redisFields["id"] != "redis_acl_user:project_a" || redisFields["type"] != "redis_acl_user" || redisFields["label"] != "project_a" {
		t.Fatalf("redis hit = %#v", redisFields)
	}
	unavailable := ResourceGroups(
		PostgresSearch{Status: "unavailable", Names: []string{"password=canary-secret host=10.0.0.1"}},
		RedisSearch{Status: "unavailable", Names: []string{"#canary-hash >canary-password"}},
	)
	blob, err := json.Marshal(unavailable)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "canary-secret") || strings.Contains(string(blob), "10.0.0.1") || strings.Contains(string(blob), "password=") {
		t.Fatalf("leaked canary: %s", blob)
	}
	if strings.Contains(string(blob), "canary-hash") || strings.Contains(string(blob), "canary-password") {
		t.Fatalf("leaked redis canary: %s", blob)
	}
}
