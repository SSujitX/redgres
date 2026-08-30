package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistToolPublicURLsUpserts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redgres.env")
	original := "REDGRES_ENVIRONMENT=production\nREDGRES_BASE_URL=http://203.0.113.10:8989\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	rewritten, err := PersistToolPublicURLs(path, "https://pgadmin.example.com", "https://redis.example.com")
	if err != nil || !rewritten {
		t.Fatalf("PersistToolPublicURLs: rewritten=%v err=%v", rewritten, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "REDGRES_PGADMIN_URL=https://pgadmin.example.com\n") {
		t.Fatalf("missing pgadmin url: %s", text)
	}
	if !strings.Contains(text, "REDGRES_REDISINSIGHT_URL=https://redis.example.com\n") {
		t.Fatalf("missing redisinsight url: %s", text)
	}
	if !strings.Contains(text, "REDGRES_BASE_URL=http://203.0.113.10:8989\n") {
		t.Fatalf("unrelated key changed: %s", text)
	}

	rewritten, err = PersistToolPublicURLs(path, "https://pgadmin.other.example", "https://redis.other.example")
	if err != nil || !rewritten {
		t.Fatalf("second persist: rewritten=%v err=%v", rewritten, err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text = string(got)
	if strings.Count(text, "REDGRES_PGADMIN_URL=") != 1 || !strings.Contains(text, "REDGRES_PGADMIN_URL=https://pgadmin.other.example\n") {
		t.Fatalf("pgadmin not replaced once: %s", text)
	}
}

func TestPersistToolPublicURLsMissingFileIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.env")
	rewritten, err := PersistToolPublicURLs(path, "https://pgadmin.example.com", "https://redis.example.com")
	if err != nil || rewritten {
		t.Fatalf("missing file should be no-op: rewritten=%v err=%v", rewritten, err)
	}
}

func TestPersistToolPublicURLsRejectsHTTP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.env")
	if err := os.WriteFile(path, []byte("REDGRES_ENVIRONMENT=production\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PersistToolPublicURLs(path, "http://pgadmin.example.com", "https://redis.example.com"); err == nil {
		t.Fatal("expected http pgadmin url to fail")
	}
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), "PGADMIN") {
		t.Fatal("failed persist must not rewrite the file")
	}
}
