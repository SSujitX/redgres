package config

import (
	"strings"
	"testing"
)

func TestLoadTruncateUnsetIsFalse(t *testing.T) {
	isolateConfig(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FeaturePostgresTruncate {
		t.Fatal("unset REDGRES_FEATURE_POSTGRES_TRUNCATE must be false")
	}
}

func TestLoadTruncateTruthy(t *testing.T) {
	for _, raw := range []string{"1", "true", "TRUE", "yes", "on"} {
		isolateConfig(t)
		t.Setenv("REDGRES_FEATURE_POSTGRES_TRUNCATE", raw)
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("%q: Load: %v", raw, err)
		}
		if !cfg.FeaturePostgresTruncate {
			t.Fatalf("%q: FeaturePostgresTruncate = false", raw)
		}
	}
}

func TestLoadTruncateFalsey(t *testing.T) {
	for _, raw := range []string{"0", "false", "FALSE", "no", "off"} {
		isolateConfig(t)
		t.Setenv("REDGRES_FEATURE_POSTGRES_TRUNCATE", raw)
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("%q: Load: %v", raw, err)
		}
		if cfg.FeaturePostgresTruncate {
			t.Fatalf("%q: FeaturePostgresTruncate = true", raw)
		}
	}
}

func TestLoadTruncateInvalidNamesEnvNeverEchoesValue(t *testing.T) {
	isolateConfig(t)
	canary := "maybe-canary-secret"
	t.Setenv("REDGRES_FEATURE_POSTGRES_TRUNCATE", canary)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected invalid flag to fail Load")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_FEATURE_POSTGRES_TRUNCATE") {
		t.Fatalf("error %q does not name the env var", msg)
	}
	if strings.Contains(msg, canary) {
		t.Fatalf("error %q echoed the value", msg)
	}
}

func TestLoadDropDoesNotEnableTruncate(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_FEATURE_POSTGRES_DROP", "true")
	t.Setenv("ENABLE_DESTRUCTIVE_ACTIONS", "true")
	t.Setenv("REDGRES_FEATURE_POSTGRES_ROW_DELETE", "true")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FeaturePostgresTruncate {
		t.Fatal("drop/row-delete keys must not enable truncate")
	}
	if !cfg.FeaturePostgresRowDelete {
		t.Fatal("row delete should still load independently")
	}
}
