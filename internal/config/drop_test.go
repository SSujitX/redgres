package config

import (
	"strings"
	"testing"
)

func TestLoadDropUnsetIsFalse(t *testing.T) {
	isolateConfig(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FeaturePostgresDrop {
		t.Fatal("unset REDGRES_FEATURE_POSTGRES_DROP must be false")
	}
}

func TestLoadDropTruthy(t *testing.T) {
	for _, raw := range []string{"1", "true", "TRUE", "yes", "on"} {
		isolateConfig(t)
		t.Setenv("REDGRES_FEATURE_POSTGRES_DROP", raw)
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("%q: Load: %v", raw, err)
		}
		if !cfg.FeaturePostgresDrop {
			t.Fatalf("%q: FeaturePostgresDrop = false", raw)
		}
	}
}

func TestLoadDropFalsey(t *testing.T) {
	for _, raw := range []string{"0", "false", "FALSE", "no", "off"} {
		isolateConfig(t)
		t.Setenv("REDGRES_FEATURE_POSTGRES_DROP", raw)
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("%q: Load: %v", raw, err)
		}
		if cfg.FeaturePostgresDrop {
			t.Fatalf("%q: FeaturePostgresDrop = true", raw)
		}
	}
}

func TestLoadDropInvalidNamesEnvNeverEchoesValue(t *testing.T) {
	isolateConfig(t)
	canary := "maybe-canary-secret"
	t.Setenv("REDGRES_FEATURE_POSTGRES_DROP", canary)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected invalid flag to fail Load")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_FEATURE_POSTGRES_DROP") {
		t.Fatalf("error %q does not name the env var", msg)
	}
	if strings.Contains(msg, canary) {
		t.Fatalf("error %q echoed the value", msg)
	}
}

func TestLoadTruncateDoesNotEnableDrop(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_FEATURE_POSTGRES_TRUNCATE", "true")
	t.Setenv("ENABLE_DESTRUCTIVE_ACTIONS", "true")
	t.Setenv("REDGRES_FEATURE_POSTGRES_ROW_DELETE", "true")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FeaturePostgresDrop {
		t.Fatal("truncate/row-delete keys must not enable drop")
	}
	if !cfg.FeaturePostgresTruncate {
		t.Fatal("truncate should still load independently")
	}
	if !cfg.FeaturePostgresRowDelete {
		t.Fatal("row delete should still load independently")
	}
}

func TestLoadRowDeleteDoesNotEnableDrop(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_FEATURE_POSTGRES_ROW_DELETE", "true")
	t.Setenv("ENABLE_DESTRUCTIVE_ACTIONS", "true")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FeaturePostgresDrop {
		t.Fatal("row-delete key must not enable drop")
	}
	if cfg.FeaturePostgresTruncate {
		t.Fatal("row-delete key must not enable truncate")
	}
}

func TestLoadDropDoesNotEnableRowDelete(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_FEATURE_POSTGRES_DROP", "true")
	t.Setenv("ENABLE_DESTRUCTIVE_ACTIONS", "true")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.FeaturePostgresDrop {
		t.Fatal("DROP key should enable drop")
	}
	if cfg.FeaturePostgresRowDelete {
		t.Fatal("DROP key must not enable row delete")
	}
	if cfg.FeaturePostgresTruncate {
		t.Fatal("DROP key must not enable truncate")
	}
}
