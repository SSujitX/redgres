package config

import (
	"strings"
	"testing"
)

func TestLoadRowDeleteUnsetIsFalse(t *testing.T) {
	isolateConfig(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FeaturePostgresRowDelete {
		t.Fatal("unset REDGRES_FEATURE_POSTGRES_ROW_DELETE must be false")
	}
}

func TestLoadRowDeleteTruthy(t *testing.T) {
	for _, raw := range []string{"1", "true", "TRUE", "yes", "on"} {
		isolateConfig(t)
		t.Setenv("REDGRES_FEATURE_POSTGRES_ROW_DELETE", raw)
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("%q: Load: %v", raw, err)
		}
		if !cfg.FeaturePostgresRowDelete {
			t.Fatalf("%q: FeaturePostgresRowDelete = false", raw)
		}
	}
}

func TestLoadRowDeleteFalsey(t *testing.T) {
	for _, raw := range []string{"0", "false", "FALSE", "no", "off"} {
		isolateConfig(t)
		t.Setenv("REDGRES_FEATURE_POSTGRES_ROW_DELETE", raw)
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("%q: Load: %v", raw, err)
		}
		if cfg.FeaturePostgresRowDelete {
			t.Fatalf("%q: FeaturePostgresRowDelete = true", raw)
		}
	}
}

func TestLoadRowDeleteInvalidNamesEnvNeverEchoesValue(t *testing.T) {
	isolateConfig(t)
	canary := "maybe-canary-secret"
	t.Setenv("REDGRES_FEATURE_POSTGRES_ROW_DELETE", canary)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected invalid flag to fail Load")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_FEATURE_POSTGRES_ROW_DELETE") {
		t.Fatalf("error %q does not name the env var", msg)
	}
	if strings.Contains(msg, canary) {
		t.Fatalf("error %q echoed the value", msg)
	}
}

func TestLoadIgnoresTruncateAndDropKeys(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_FEATURE_POSTGRES_DROP", "true")
	t.Setenv("REDGRES_FEATURE_POSTGRES_TRUNCATE", "not-a-bool")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FeaturePostgresRowDelete {
		t.Fatal("drop/truncate keys must not enable row delete")
	}
}
