package redisadmin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCreateUserBuildsSafeCacheReadWriteRules(t *testing.T) {
	mem := &MemoryClient{}
	svc := NewService(mem)
	got, err := svc.CreateUser(context.Background(), "project_a", "project_a", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.User.Username != "project_a" || !got.User.Enabled || got.User.KeyPattern != "project_a:*" {
		t.Fatalf("user = %#v", got.User)
	}
	if got.User.Preset != PresetCacheReadWrite || got.User.QueueKind != "" || got.User.Protected || got.User.RuleFidelity != RuleExact {
		t.Fatalf("labels = %#v", got.User)
	}
	if len(got.Password) != 32 {
		t.Fatalf("password length = %d", len(got.Password))
	}
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("ACLSetUser calls = %d", len(mem.ACLSetUserCalls))
	}
	call := mem.ACLSetUserCalls[0]
	if call.Username != "project_a" {
		t.Fatalf("username = %q", call.Username)
	}
	assertCreateRules(t, call.Rules, got.Password, "project_a:*", inspectCacheReadWrite)
	if !equalSet(got.User.Commands, inspectCacheReadWrite) {
		t.Fatalf("commands = %#v", got.User.Commands)
	}
}

func TestCreateUserCustomAllowList(t *testing.T) {
	mem := &MemoryClient{}
	svc := NewService(mem)
	got, err := svc.CreateUser(context.Background(), "project_a", "project_a", PresetCustom, "", []string{"ECHO", " get ", "ping", "GET"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.User.Enabled || got.User.Protected || got.User.RuleFidelity != RuleExact {
		t.Fatalf("user = %#v", got.User)
	}
	if got.User.Preset != PresetCustom || got.User.QueueKind != "" {
		t.Fatalf("labels = %#v", got.User)
	}
	want := []string{"echo", "get", "ping"}
	if !equalSet(got.User.Commands, want) {
		t.Fatalf("commands = %#v want %#v", got.User.Commands, want)
	}
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("ACLSetUser calls = %d", len(mem.ACLSetUserCalls))
	}
	assertCreateRules(t, mem.ACLSetUserCalls[0].Rules, got.Password, "project_a:*", want)
}

func TestCreateUserCustomMatchingNamedSetInfersNamedPreset(t *testing.T) {
	mem := &MemoryClient{}
	svc := NewService(mem)
	got, err := svc.CreateUser(context.Background(), "project_a", "project_a", PresetCustom, "", inspectReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.User.Preset != PresetReadOnly {
		t.Fatalf("inferPreset = %q want %s", got.User.Preset, PresetReadOnly)
	}
	if got.User.QueueKind != "" {
		t.Fatalf("queue_kind = %q", got.User.QueueKind)
	}
	if !equalSet(got.User.Commands, inspectReadOnly) {
		t.Fatalf("commands mismatch")
	}
	assertCreateRules(t, mem.ACLSetUserCalls[0].Rules, got.Password, "project_a:*", inspectReadOnly)
}

func TestCreateUserNamedEmptyCommandsStillSucceeds(t *testing.T) {
	cases := []struct {
		name     string
		preset   string
		commands []string
	}{
		{name: "omitted", preset: "", commands: nil},
		{name: "empty-array", preset: PresetReadOnly, commands: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &MemoryClient{}
			svc := NewService(mem)
			got, err := svc.CreateUser(context.Background(), "project_a", "project_a", tc.preset, "", tc.commands)
			if err != nil {
				t.Fatal(err)
			}
			if len(mem.ACLSetUserCalls) != 1 {
				t.Fatalf("ACLSetUser calls = %d", len(mem.ACLSetUserCalls))
			}
			wantPreset := PresetCacheReadWrite
			wantCmds := inspectCacheReadWrite
			if tc.preset == PresetReadOnly {
				wantPreset = PresetReadOnly
				wantCmds = inspectReadOnly
			}
			if got.User.Preset != wantPreset {
				t.Fatalf("preset = %q", got.User.Preset)
			}
			assertCreateRules(t, mem.ACLSetUserCalls[0].Rules, got.Password, "project_a:*", wantCmds)
		})
	}
}

func TestCreateUserNamedPresetsGrantMatchingInspectSets(t *testing.T) {
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
			mem := &MemoryClient{}
			svc := NewService(mem)
			got, err := svc.CreateUser(context.Background(), "project_a", "project_a", tc.preset, tc.queueKind, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.User.Preset != tc.preset || got.User.QueueKind != tc.wantQueue {
				t.Fatalf("labels preset=%q queue=%q", got.User.Preset, got.User.QueueKind)
			}
			if !got.User.Enabled || got.User.Protected || got.User.RuleFidelity != RuleExact {
				t.Fatalf("user = %#v", got.User)
			}
			if len(mem.ACLSetUserCalls) != 1 {
				t.Fatalf("ACLSetUser calls = %d", len(mem.ACLSetUserCalls))
			}
			assertCreateRules(t, mem.ACLSetUserCalls[0].Rules, got.Password, "project_a:*", tc.wantCmds)
			if !equalSet(got.User.Commands, tc.wantCmds) {
				t.Fatalf("result commands mismatch")
			}
		})
	}
}

func TestCreateUserRejectsCustomUnknownAndQueueKindMismatch(t *testing.T) {
	cases := []struct {
		name      string
		preset    string
		queueKind string
		commands  []string
		want      error
	}{
		{name: "custom-empty", preset: PresetCustom, want: ErrInvalidCommands},
		{name: "custom-with-kind", preset: PresetCustom, queueKind: QueueLists, commands: []string{"get"}, want: ErrInvalidQueueKind},
		{name: "unknown", preset: "not-a-preset", want: ErrInvalidPreset},
		{name: "named-with-commands", preset: PresetReadOnly, commands: []string{"get"}, want: ErrInvalidCommands},
		{name: "empty-preset-with-commands", commands: []string{"get"}, want: ErrInvalidCommands},
		{name: "queue-missing-kind", preset: PresetQueueWorker, want: ErrInvalidQueueKind},
		{name: "queue-bad-kind", preset: PresetQueueWorker, queueKind: "jobs", want: ErrInvalidQueueKind},
		{name: "cache-with-kind", preset: PresetCacheReadWrite, queueKind: QueueLists, want: ErrInvalidQueueKind},
		{name: "readonly-with-kind", preset: PresetReadOnly, queueKind: QueueStreams, want: ErrInvalidQueueKind},
		{name: "empty-preset-with-kind", queueKind: QueueLists, want: ErrInvalidQueueKind},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &MemoryClient{ACLListErr: errors.New("acl list should not run")}
			svc := NewService(mem)
			_, err := svc.CreateUser(context.Background(), "project_a", "project_a", tc.preset, tc.queueKind, tc.commands)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v want %v", err, tc.want)
			}
			if len(mem.ACLSetUserCalls) != 0 {
				t.Fatalf("ACLSetUser called on reject: %#v", mem.ACLSetUserCalls)
			}
		})
	}
}

func TestCreateUserRejectsProtectedAndDuplicate(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{"user project_a on ~project_a:* -@all +ping"}}
	svc := NewServiceAdmin(mem, "ops_admin")
	if _, err := svc.CreateUser(context.Background(), "admin", "project_a", "", "", nil); !errors.Is(err, ErrProtectedUser) {
		t.Fatalf("admin err = %v", err)
	}
	if _, err := svc.CreateUser(context.Background(), "ops_admin", "ops", "", "", nil); !errors.Is(err, ErrProtectedUser) {
		t.Fatalf("configured admin err = %v", err)
	}
	if _, err := svc.CreateUser(context.Background(), "project_a", "project_a", "", "", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate err = %v", err)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("ACLSetUser called on reject: %#v", mem.ACLSetUserCalls)
	}
}

func TestCreateUserNilClientIsNotConfigured(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.CreateUser(context.Background(), "project_a", "project_a", "", "", nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateUserMapsRedisFailure(t *testing.T) {
	svc := NewService(&MemoryClient{ACLListErr: errors.New("NOAUTH Authentication required. canary-secret")})
	_, err := svc.CreateUser(context.Background(), "project_a", "project_a", "", "", nil)
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") {
		t.Fatalf("leaked canary: %v", err)
	}
}

func TestCreateUserMapsSetUserModifierError(t *testing.T) {
	svc := NewService(&MemoryClient{
		ACLSetUserErr: errors.New("ERR Error in ACL SETUSER modifier '>canary-secret': Syntax error"),
	})
	_, err := svc.CreateUser(context.Background(), "project_a", "project_a", "", "", nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), ">") {
		t.Fatalf("leaked SETUSER modifier: %v", err)
	}
}

func assertCreateRules(t *testing.T, rules []string, password, pattern string, wantCmds []string) {
	t.Helper()
	if len(rules) < 6 {
		t.Fatalf("rules too short: %#v", rules)
	}
	joined := strings.Join(rules, " ")
	if rules[0] != "reset" || rules[1] != "on" {
		t.Fatalf("prefix rules = %#v", rules[:2])
	}
	if rules[2] != ">"+password {
		t.Fatalf("password rule = %q", rules[2])
	}
	if rules[3] != "~"+pattern {
		t.Fatalf("key rule = %q", rules[3])
	}
	if rules[4] != "resetchannels" || rules[5] != "-@all" {
		t.Fatalf("channel/category rules = %#v", rules[4:6])
	}
	for _, rule := range rules {
		if rule == "nocommands" {
			t.Fatalf("create SETUSER has nocommands: %#v", rules)
		}
	}
	upperJoined := strings.ToUpper(joined)
	for _, bad := range []string{"+@ALL", "+ACL", "+CONFIG", "+FLUSHALL", "+FLUSHDB", "+SCRIPT", "+EVAL"} {
		if strings.Contains(upperJoined, bad) {
			t.Fatalf("dangerous %s in %s", bad, joined)
		}
	}
	for _, rule := range rules[6:] {
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

func grantedCommands(rules []string) []string {
	out := make([]string, 0)
	for _, rule := range rules {
		if !strings.HasPrefix(rule, "+") {
			continue
		}
		cmd := strings.ToLower(strings.TrimPrefix(rule, "+"))
		if cmd == "" || strings.HasPrefix(cmd, "@") {
			continue
		}
		out = append(out, cmd)
	}
	return uniqueSorted(out)
}
