package redisadmin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const updateACLLine = "user project_a on #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 ~project_a:* -@all +echo +get +ping"

func TestUpdatePermissionsNamedPresetsGrantMatchingInspectSets(t *testing.T) {
	cases := []struct {
		name      string
		preset    string
		queueKind string
		wantQueue string
		wantCmds  []string
	}{
		{name: "cache-read-write", preset: PresetCacheReadWrite, wantCmds: inspectCacheReadWrite},
		{name: "read-only", preset: PresetReadOnly, wantCmds: inspectReadOnly},
		{name: "queue-lists", preset: PresetQueueWorker, queueKind: QueueLists, wantQueue: QueueLists, wantCmds: inspectQueueLists},
		{name: "queue-streams", preset: PresetQueueWorker, queueKind: QueueStreams, wantQueue: QueueStreams, wantCmds: inspectQueueStreams},
		{name: "queue-sorted-sets", preset: PresetQueueWorker, queueKind: QueueSortedSets, wantQueue: QueueSortedSets, wantCmds: inspectQueueSortedSets},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &MemoryClient{ACLLines: []string{updateACLLine}}
			svc := NewService(mem)
			got, err := svc.UpdatePermissions(context.Background(), "project_a", "other_app", tc.preset, tc.queueKind, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.Username != "project_a" || !got.Enabled || got.KeyPattern != "other_app:*" {
				t.Fatalf("user = %#v", got)
			}
			if got.Preset != tc.preset || got.QueueKind != tc.wantQueue {
				t.Fatalf("labels preset=%q queue=%q", got.Preset, got.QueueKind)
			}
			if got.Protected || got.RuleFidelity != RuleExact {
				t.Fatalf("user = %#v", got)
			}
			if !equalSet(got.Commands, tc.wantCmds) {
				t.Fatalf("result commands mismatch")
			}
			if len(mem.ACLSetUserCalls) != 1 {
				t.Fatalf("ACLSetUser calls = %d", len(mem.ACLSetUserCalls))
			}
			assertUpdateRules(t, mem.ACLSetUserCalls[0].Rules, "other_app:*", tc.wantCmds)
			assertUpdateLinePreserved(t, mem.ACLLines, "project_a", "on", "other_app:*", "#9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
		})
	}
}

func TestUpdatePermissionsPreservesEnabledAndHash(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{updateACLLine}}
	svc := NewService(mem)
	got, err := svc.UpdatePermissions(context.Background(), "project_a", "project_b", PresetReadOnly, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Preset != PresetReadOnly || got.KeyPattern != "project_b:*" {
		t.Fatalf("user = %#v", got)
	}
	assertUpdateLinePreserved(t, mem.ACLLines, "project_a", "on", "project_b:*", "#9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
}

func TestUpdatePermissionsUpdatesCustomLimitedAndDisabled(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{
		"user limited on ~objects:* &* +@all -@admin +ping",
		"user custom on ~custom:* -@all +get +set",
		"user disabled off #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 ~project_a:* -@all +echo +get +ping",
	}}
	svc := NewService(mem)

	limitedBefore, err := svc.GetUser(context.Background(), "limited")
	if err != nil {
		t.Fatal(err)
	}
	if limitedBefore.Preset != PresetCustom || limitedBefore.RuleFidelity != RuleLimited {
		t.Fatalf("limited inspect = %#v", limitedBefore)
	}
	limited, err := svc.UpdatePermissions(context.Background(), "limited", "limited", PresetCacheReadWrite, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Enabled || limited.Preset != PresetCacheReadWrite || limited.RuleFidelity != RuleExact {
		t.Fatalf("limited after patch = %#v", limited)
	}
	if limited.KeyPattern != "limited:*" || !equalSet(limited.Commands, inspectCacheReadWrite) {
		t.Fatalf("limited grants = %#v", limited)
	}

	customBefore, err := svc.GetUser(context.Background(), "custom")
	if err != nil {
		t.Fatal(err)
	}
	if customBefore.Preset != PresetCustom {
		t.Fatalf("custom preset = %q", customBefore.Preset)
	}
	custom, err := svc.UpdatePermissions(context.Background(), "custom", "custom", PresetReadOnly, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !custom.Enabled || custom.Preset != PresetReadOnly || !equalSet(custom.Commands, inspectReadOnly) {
		t.Fatalf("custom after patch = %#v", custom)
	}

	disabled, err := svc.UpdatePermissions(context.Background(), "disabled", "project_b", PresetReadOnly, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled {
		t.Fatalf("disabled became enabled: %#v", disabled)
	}
	if disabled.KeyPattern != "project_b:*" || disabled.Preset != PresetReadOnly {
		t.Fatalf("disabled grants = %#v", disabled)
	}
	assertUpdateLinePreserved(t, mem.ACLLines, "disabled", "off", "project_b:*", "#9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
	if len(mem.ACLSetUserCalls) != 3 {
		t.Fatalf("SETUSER calls = %d", len(mem.ACLSetUserCalls))
	}
}

func TestUpdatePermissionsRejectsProtectedWithoutSetUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{
		"user default on nopass ~* &* +@all",
		"user admin on ~* -@all +ping",
		"user redact_admin on ~* -@all +ping",
		"user ops_admin on ~* -@all +ping",
		"user project_a on ~project_a:* -@all +ping",
	}}
	svc := NewServiceAdmin(mem, "ops_admin")
	for _, name := range []string{"default", "admin", "redact_admin", "ops_admin", "OPS_ADMIN"} {
		if _, err := svc.UpdatePermissions(context.Background(), name, "project_a", PresetReadOnly, "", nil); !errors.Is(err, ErrProtectedUser) {
			t.Fatalf("%s err = %v", name, err)
		}
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("ACLSetUser called on protected: %#v", mem.ACLSetUserCalls)
	}
}

func TestUpdatePermissionsMissingUserDoesNotSetUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{"user project_a on ~project_a:* -@all +ping"}}
	svc := NewService(mem)
	if _, err := svc.UpdatePermissions(context.Background(), "missing", "project_a", PresetReadOnly, "", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("ACLSetUser called on missing: %#v", mem.ACLSetUserCalls)
	}
}

func TestUpdatePermissionsEmptyPresetDoesNotDefault(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{updateACLLine}}
	svc := NewService(mem)
	if _, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", "", "", nil); !errors.Is(err, ErrInvalidPreset) {
		t.Fatalf("err = %v", err)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("ACLSetUser called on empty preset: %#v", mem.ACLSetUserCalls)
	}
	got, err := svc.GetUser(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Preset == PresetCacheReadWrite && equalSet(got.Commands, inspectCacheReadWrite) {
		t.Fatalf("empty preset defaulted to cache-read-write: %#v", got)
	}
}

func TestUpdatePermissionsRejectsCustomUnknownAndQueueKindMismatch(t *testing.T) {
	cases := []struct {
		name      string
		preset    string
		queueKind string
		commands  []string
		want      error
	}{
		{name: "custom-empty", preset: PresetCustom, want: ErrInvalidCommands},
		{name: "unknown", preset: "not-a-preset", want: ErrInvalidPreset},
		{name: "queue-missing-kind", preset: PresetQueueWorker, want: ErrInvalidQueueKind},
		{name: "queue-bad-kind", preset: PresetQueueWorker, queueKind: "jobs", want: ErrInvalidQueueKind},
		{name: "cache-with-kind", preset: PresetCacheReadWrite, queueKind: QueueLists, want: ErrInvalidQueueKind},
		{name: "readonly-with-kind", preset: PresetReadOnly, queueKind: QueueStreams, want: ErrInvalidQueueKind},
		{name: "named-with-commands", preset: PresetReadOnly, commands: []string{"get"}, want: ErrInvalidCommands},
		{name: "custom-with-kind", preset: PresetCustom, queueKind: QueueLists, commands: []string{"ping"}, want: ErrInvalidQueueKind},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &MemoryClient{ACLLines: []string{updateACLLine}}
			svc := NewService(mem)
			_, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", tc.preset, tc.queueKind, tc.commands)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v want %v", err, tc.want)
			}
			if len(mem.ACLSetUserCalls) != 0 {
				t.Fatalf("ACLSetUser called on reject: %#v", mem.ACLSetUserCalls)
			}
		})
	}
}

func TestUpdatePermissionsIdempotentStillSetUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{updateACLLine}}
	svc := NewService(mem)
	first, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", PresetReadOnly, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", PresetReadOnly, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Preset != PresetReadOnly || second.Preset != PresetReadOnly {
		t.Fatalf("presets = %q / %q", first.Preset, second.Preset)
	}
	if len(mem.ACLSetUserCalls) != 2 {
		t.Fatalf("SETUSER calls = %d", len(mem.ACLSetUserCalls))
	}
	assertUpdateRules(t, mem.ACLSetUserCalls[1].Rules, "project_a:*", inspectReadOnly)
}

func TestUpdatePermissionsMapsSetUserModifierErrorWithoutCanary(t *testing.T) {
	mem := &MemoryClient{
		ACLLines:      []string{updateACLLine},
		ACLSetUserErr: errors.New("ERR Error in ACL SETUSER modifier '>canary-secret': Syntax error"),
	}
	svc := NewService(mem)
	_, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", PresetReadOnly, "", nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), ">") {
		t.Fatalf("leaked SETUSER modifier: %v", err)
	}
}

func TestUpdatePermissionsNilClientIsNotConfigured(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", PresetReadOnly, "", nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

func assertUpdateRules(t *testing.T, rules []string, pattern string, wantCmds []string) {
	t.Helper()
	if len(rules) < 5 {
		t.Fatalf("rules too short: %#v", rules)
	}
	joined := strings.Join(rules, " ")
	if rules[0] != "resetkeys" {
		t.Fatalf("prefix rules = %#v", rules[:1])
	}
	if rules[1] != "~"+pattern {
		t.Fatalf("key rule = %q", rules[1])
	}
	if rules[2] != "resetchannels" || rules[3] != "nocommands" || rules[4] != "-@all" {
		t.Fatalf("channel/command reset rules = %#v", rules[2:5])
	}
	for _, rule := range rules {
		if rule == "reset" || rule == "resetpass" || rule == "on" || rule == "off" {
			t.Fatalf("forbidden rule %q", rule)
		}
		if strings.HasPrefix(rule, ">") {
			t.Fatalf("forbidden password rule %q", rule)
		}
	}
	upperJoined := strings.ToUpper(joined)
	for _, bad := range []string{"+@ALL", "+ACL", "+CONFIG", "+FLUSHALL", "+FLUSHDB", "+SCRIPT", "+EVAL"} {
		if strings.Contains(upperJoined, bad) {
			t.Fatalf("dangerous %s in %s", bad, joined)
		}
	}
	for _, rule := range rules[5:] {
		if !strings.HasPrefix(rule, "+") {
			t.Fatalf("non-grant after -@all: %q", rule)
		}
		cmd := strings.TrimPrefix(rule, "+")
		if strings.HasPrefix(cmd, "@") {
			t.Fatalf("category grant %q", rule)
		}
		if cmd != strings.ToUpper(cmd) {
			t.Fatalf("grant not upper: %q", rule)
		}
	}
	got := grantedCommands(rules)
	if !equalSet(got, wantCmds) {
		t.Fatalf("granted = %#v want %#v", got, wantCmds)
	}
}

func assertUpdateLinePreserved(t *testing.T, lines []string, username, flag, pattern, hash string) {
	t.Helper()
	line := aclLineFor(t, lines, username)
	if !strings.Contains(line, flag) {
		t.Fatalf("missing %q in %q", flag, line)
	}
	if !strings.Contains(line, "~"+pattern) {
		t.Fatalf("missing key pattern in %q", line)
	}
	if hash != "" && !strings.Contains(line, hash) {
		t.Fatalf("lost hash: %q", line)
	}
	for _, token := range []string{"resetkeys", "nocommands", "resetchannels", "resetpass", "reset "} {
		if strings.Contains(line, token) {
			t.Fatalf("persisted modifier %q in %q", token, line)
		}
	}
	if strings.Contains(line, ">") {
		t.Fatalf("persisted password token: %q", line)
	}
}
