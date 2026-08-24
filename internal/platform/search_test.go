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
	})
	if len(got) != 2 {
		t.Fatalf("len = %d %#v", len(got), got)
	}
	if got[0].ID != "postgres_databases" || got[0].Status != "ok" || got[0].Service != "postgres" {
		t.Fatalf("postgres = %#v", got[0])
	}
	if len(got[0].Hits) != 1 || got[0].Hits[0].ID != "postgres_database:project_a" || got[0].Hits[0].Type != "postgres_database" || got[0].Hits[0].Label != "project_a" {
		t.Fatalf("hits = %#v", got[0].Hits)
	}
	if got[1].ID != "redis_acl_users" || got[1].Status != "not_implemented" || got[1].Truncated || len(got[1].Hits) != 0 {
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
	})
	if got[0].Status != "unavailable" || len(got[0].Hits) != 0 || got[0].Truncated {
		t.Fatalf("postgres = %#v", got[0])
	}
	if got[1].ID != "redis_acl_users" || got[1].Status != "not_implemented" || len(got[1].Hits) != 0 {
		t.Fatalf("redis = %#v", got[1])
	}
}

func TestResourceGroupsHitFieldsOnlyAndOmitsCanary(t *testing.T) {
	got := ResourceGroups(PostgresSearch{
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
	unavailable := ResourceGroups(PostgresSearch{Status: "unavailable", Names: []string{"password=canary-secret host=10.0.0.1"}})
	blob, err := json.Marshal(unavailable)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "canary-secret") || strings.Contains(string(blob), "10.0.0.1") || strings.Contains(string(blob), "password=") {
		t.Fatalf("leaked canary: %s", blob)
	}
}
