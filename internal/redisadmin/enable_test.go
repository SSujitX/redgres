package redisadmin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const enableACLLine = "user project_a on #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 ~project_a:* -@all +echo +get +ping"

func TestSetEnabledDisableThenEnablePreservesRules(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{enableACLLine}}
	svc := NewService(mem)

	disabled, err := svc.SetEnabled(context.Background(), "project_a", false)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled {
		t.Fatalf("enabled after disable: %#v", disabled)
	}
	assertEnablePreservedInspect(t, disabled)
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("disable SETUSER calls = %d", len(mem.ACLSetUserCalls))
	}
	assertOnOffOnly(t, mem.ACLSetUserCalls[0], "project_a", "off")
	assertACLLinePreserved(t, mem.ACLLines, "project_a", "off")

	enabled, err := svc.SetEnabled(context.Background(), "project_a", true)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.Enabled {
		t.Fatalf("enabled after enable: %#v", enabled)
	}
	assertEnablePreservedInspect(t, enabled)
	if len(mem.ACLSetUserCalls) != 2 {
		t.Fatalf("enable SETUSER calls = %d", len(mem.ACLSetUserCalls))
	}
	assertOnOffOnly(t, mem.ACLSetUserCalls[1], "project_a", "on")
	assertACLLinePreserved(t, mem.ACLLines, "project_a", "on")
}

func TestSetEnabledRejectsProtectedWithoutSetUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{
		"user default on nopass ~* &* +@all",
		"user admin on ~* -@all +ping",
		"user redact_admin on ~* -@all +ping",
		"user ops_admin on ~* -@all +ping",
		"user project_a on ~project_a:* -@all +ping",
	}}
	svc := NewServiceAdmin(mem, "ops_admin")
	for _, name := range []string{"default", "admin", "redact_admin", "ops_admin", "OPS_ADMIN"} {
		if _, err := svc.SetEnabled(context.Background(), name, false); !errors.Is(err, ErrProtectedUser) {
			t.Fatalf("%s err = %v", name, err)
		}
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("ACLSetUser called on protected: %#v", mem.ACLSetUserCalls)
	}
}

func TestSetEnabledMissingUserDoesNotSetUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{"user project_a on ~project_a:* -@all +ping"}}
	svc := NewService(mem)
	if _, err := svc.SetEnabled(context.Background(), "missing", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("ACLSetUser called on missing: %#v", mem.ACLSetUserCalls)
	}
}

func TestSetEnabledIdempotentStillSetUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{enableACLLine}}
	svc := NewService(mem)

	on, err := svc.SetEnabled(context.Background(), "project_a", true)
	if err != nil {
		t.Fatal(err)
	}
	if !on.Enabled {
		t.Fatalf("on→on enabled = %#v", on)
	}
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("on→on SETUSER calls = %d", len(mem.ACLSetUserCalls))
	}
	assertOnOffOnly(t, mem.ACLSetUserCalls[0], "project_a", "on")

	mem.ACLLines = []string{"user project_a off #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 ~project_a:* -@all +echo +get +ping"}
	mem.ACLSetUserCalls = nil
	off, err := svc.SetEnabled(context.Background(), "project_a", false)
	if err != nil {
		t.Fatal(err)
	}
	if off.Enabled {
		t.Fatalf("off→off enabled = %#v", off)
	}
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("off→off SETUSER calls = %d", len(mem.ACLSetUserCalls))
	}
	assertOnOffOnly(t, mem.ACLSetUserCalls[0], "project_a", "off")
}

func TestSetEnabledTogglesLimitedAndCustomUsers(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{
		"user limited on ~objects:* &* +@all -@admin +ping",
		"user custom on ~custom:* -@all +get +set",
	}}
	svc := NewService(mem)

	limited, err := svc.GetUser(context.Background(), "limited")
	if err != nil {
		t.Fatal(err)
	}
	if limited.Preset != PresetCustom || limited.RuleFidelity != RuleLimited {
		t.Fatalf("limited inspect = %#v", limited)
	}
	toggled, err := svc.SetEnabled(context.Background(), "limited", false)
	if err != nil {
		t.Fatal(err)
	}
	if toggled.Enabled {
		t.Fatalf("limited still enabled: %#v", toggled)
	}
	if toggled.KeyPattern != limited.KeyPattern || toggled.Preset != limited.Preset {
		t.Fatalf("limited labels changed: before %#v after %#v", limited, toggled)
	}
	if strings.Join(toggled.Commands, ",") != strings.Join(limited.Commands, ",") {
		t.Fatalf("limited commands changed: %#v → %#v", limited.Commands, toggled.Commands)
	}
	if strings.Join(toggled.Categories, ",") != strings.Join(limited.Categories, ",") {
		t.Fatalf("limited categories changed: %#v → %#v", limited.Categories, toggled.Categories)
	}

	custom, err := svc.GetUser(context.Background(), "custom")
	if err != nil {
		t.Fatal(err)
	}
	if custom.Preset != PresetCustom {
		t.Fatalf("custom preset = %q", custom.Preset)
	}
	disabled, err := svc.SetEnabled(context.Background(), "custom", false)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled || disabled.Preset != PresetCustom {
		t.Fatalf("custom after disable = %#v", disabled)
	}
	if strings.Join(disabled.Commands, ",") != strings.Join(custom.Commands, ",") {
		t.Fatalf("custom commands changed: %#v → %#v", custom.Commands, disabled.Commands)
	}
	if len(mem.ACLSetUserCalls) != 2 {
		t.Fatalf("SETUSER calls = %d", len(mem.ACLSetUserCalls))
	}
	assertOnOffOnly(t, mem.ACLSetUserCalls[0], "limited", "off")
	assertOnOffOnly(t, mem.ACLSetUserCalls[1], "custom", "off")
}

func TestSetEnabledMapsSetUserModifierErrorWithoutCanary(t *testing.T) {
	mem := &MemoryClient{
		ACLLines:      []string{enableACLLine},
		ACLSetUserErr: errors.New("ERR Error in ACL SETUSER modifier '>canary-secret': Syntax error"),
	}
	svc := NewService(mem)
	_, err := svc.SetEnabled(context.Background(), "project_a", false)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), ">") {
		t.Fatalf("leaked SETUSER modifier: %v", err)
	}
}

func TestSetEnabledNilClientIsNotConfigured(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.SetEnabled(context.Background(), "project_a", false); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

func assertOnOffOnly(t *testing.T, call ACLSetUserCall, username, flag string) {
	t.Helper()
	if call.Username != username {
		t.Fatalf("username = %q want %q", call.Username, username)
	}
	if len(call.Rules) != 1 || call.Rules[0] != flag {
		t.Fatalf("rules = %#v want [%q]", call.Rules, flag)
	}
	for _, rule := range call.Rules {
		if rule == "reset" || rule == "resetpass" || rule == "resetkeys" || rule == "resetchannels" || rule == "nocommands" || rule == "-@all" {
			t.Fatalf("forbidden rule %q", rule)
		}
		if strings.HasPrefix(rule, "~") || strings.HasPrefix(rule, ">") {
			t.Fatalf("forbidden rule %q", rule)
		}
	}
}

func assertEnablePreservedInspect(t *testing.T, u User) {
	t.Helper()
	if u.Username != "project_a" || u.KeyPattern != "project_a:*" {
		t.Fatalf("identity = %#v", u)
	}
	if u.Preset != PresetCustom || u.RuleFidelity != RuleExact || u.Protected {
		t.Fatalf("labels = %#v", u)
	}
	if strings.Join(u.Commands, ",") != "echo,get,ping" {
		t.Fatalf("commands = %#v", u.Commands)
	}
}

func assertACLLinePreserved(t *testing.T, lines []string, username, flag string) {
	t.Helper()
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "user" || fields[1] != username {
			continue
		}
		if !strings.Contains(line, flag) {
			t.Fatalf("missing %q in %q", flag, line)
		}
		if !strings.Contains(line, "~project_a:*") || !strings.Contains(line, "-@all") {
			t.Fatalf("lost key/category rules: %q", line)
		}
		if !strings.Contains(line, "+echo") || !strings.Contains(line, "+get") || !strings.Contains(line, "+ping") {
			t.Fatalf("lost commands: %q", line)
		}
		if !strings.Contains(line, "#9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08") {
			t.Fatalf("lost hash: %q", line)
		}
		return
	}
	t.Fatalf("user %q not in ACL lines %#v", username, lines)
}
