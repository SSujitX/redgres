package redisadmin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const rotateACLLine = "user project_a on #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 ~project_a:* -@all +echo +get +ping"

func TestRotateUserResetpassPasswordOnlyPreservesGrants(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{rotateACLLine}}
	svc := NewService(mem)

	got, err := svc.RotateUser(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.User.Username != "project_a" || !got.User.Enabled || got.User.KeyPattern != "project_a:*" {
		t.Fatalf("user = %#v", got.User)
	}
	if got.User.Preset != PresetCustom || got.User.RuleFidelity != RuleExact || got.User.Protected {
		t.Fatalf("labels = %#v", got.User)
	}
	if strings.Join(got.User.Commands, ",") != "echo,get,ping" {
		t.Fatalf("commands = %#v", got.User.Commands)
	}
	if len(got.Password) != 32 {
		t.Fatalf("password length = %d", len(got.Password))
	}
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("SETUSER calls = %d", len(mem.ACLSetUserCalls))
	}
	assertRotateRulesOnly(t, mem.ACLSetUserCalls[0], "project_a", got.Password)
	assertRotateLinePreserved(t, mem.ACLLines, "project_a", got.Password)
	if strings.Contains(got.User.Username, ">") || strings.Contains(strings.Join(got.User.Commands, ","), ">") {
		t.Fatalf("inspect leaked password marker: %#v", got.User)
	}
}

func TestRotateUserStripsPasswordTokensKeepsChannels(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{
		"user project_a on nopass >canary-secret #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 !deadbeef ~project_a:* &* -@all +echo +get +ping",
	}}
	svc := NewService(mem)
	got, err := svc.RotateUser(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if !got.User.Enabled || got.User.KeyPattern != "project_a:*" {
		t.Fatalf("user = %#v", got.User)
	}
	line := aclLineFor(t, mem.ACLLines, "project_a")
	if strings.Contains(line, "canary-secret") || strings.Contains(line, "nopass") || strings.Contains(line, "#9f86d081") || strings.Contains(line, "!deadbeef") {
		t.Fatalf("old password tokens remain: %q", line)
	}
	if !strings.Contains(line, "on") || !strings.Contains(line, "~project_a:*") || !strings.Contains(line, "&*") || !strings.Contains(line, "-@all") {
		t.Fatalf("lost grants: %q", line)
	}
	if !strings.Contains(line, ">"+got.Password) {
		t.Fatalf("missing new password token: %q", line)
	}
	if strings.Contains(line, "resetpass") || strings.Contains(line, "reset ") {
		t.Fatalf("stored modifier on line: %q", line)
	}
}

func TestRotateUserRejectsProtectedWithoutSetUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{
		"user default on nopass ~* &* +@all",
		"user admin on ~* -@all +ping",
		"user redact_admin on ~* -@all +ping",
		"user ops_admin on ~* -@all +ping",
		"user project_a on ~project_a:* -@all +ping",
	}}
	svc := NewServiceAdmin(mem, "ops_admin")
	for _, name := range []string{"default", "admin", "redact_admin", "ops_admin", "OPS_ADMIN"} {
		if _, err := svc.RotateUser(context.Background(), name); !errors.Is(err, ErrProtectedUser) {
			t.Fatalf("%s err = %v", name, err)
		}
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("ACLSetUser called on protected: %#v", mem.ACLSetUserCalls)
	}
}

func TestRotateUserMissingUserDoesNotSetUser(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{"user project_a on ~project_a:* -@all +ping"}}
	svc := NewService(mem)
	if _, err := svc.RotateUser(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("ACLSetUser called on missing: %#v", mem.ACLSetUserCalls)
	}
}

func TestRotateUserRotatesLimitedCustomAndDisabled(t *testing.T) {
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
	limited, err := svc.RotateUser(context.Background(), "limited")
	if err != nil {
		t.Fatal(err)
	}
	if limited.User.KeyPattern != limitedBefore.KeyPattern || limited.User.Preset != limitedBefore.Preset || !limited.User.Enabled {
		t.Fatalf("limited labels changed: before %#v after %#v", limitedBefore, limited.User)
	}
	if strings.Join(limited.User.Commands, ",") != strings.Join(limitedBefore.Commands, ",") {
		t.Fatalf("limited commands changed: %#v → %#v", limitedBefore.Commands, limited.User.Commands)
	}
	if strings.Join(limited.User.Categories, ",") != strings.Join(limitedBefore.Categories, ",") {
		t.Fatalf("limited categories changed: %#v → %#v", limitedBefore.Categories, limited.User.Categories)
	}

	customBefore, err := svc.GetUser(context.Background(), "custom")
	if err != nil {
		t.Fatal(err)
	}
	custom, err := svc.RotateUser(context.Background(), "custom")
	if err != nil {
		t.Fatal(err)
	}
	if custom.User.Enabled != customBefore.Enabled || custom.User.Preset != PresetCustom {
		t.Fatalf("custom after rotate = %#v", custom.User)
	}
	if strings.Join(custom.User.Commands, ",") != strings.Join(customBefore.Commands, ",") {
		t.Fatalf("custom commands changed: %#v → %#v", customBefore.Commands, custom.User.Commands)
	}

	disabled, err := svc.RotateUser(context.Background(), "disabled")
	if err != nil {
		t.Fatal(err)
	}
	if disabled.User.Enabled {
		t.Fatalf("disabled became enabled: %#v", disabled.User)
	}
	if disabled.User.KeyPattern != "project_a:*" || strings.Join(disabled.User.Commands, ",") != "echo,get,ping" {
		t.Fatalf("disabled grants lost: %#v", disabled.User)
	}
	if len(mem.ACLSetUserCalls) != 3 {
		t.Fatalf("SETUSER calls = %d", len(mem.ACLSetUserCalls))
	}
	assertRotateRulesOnly(t, mem.ACLSetUserCalls[0], "limited", limited.Password)
	assertRotateRulesOnly(t, mem.ACLSetUserCalls[1], "custom", custom.Password)
	assertRotateRulesOnly(t, mem.ACLSetUserCalls[2], "disabled", disabled.Password)
}

func TestRotateUserInspectOmitsCanaryPassword(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{
		"user project_a on >canary-secret ~project_a:* -@all +echo +get +ping",
	}}
	svc := NewService(mem)
	got, err := svc.RotateUser(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.User.Username, "canary") || strings.Contains(got.User.KeyPattern, "canary") {
		t.Fatalf("inspect leaked canary: %#v", got.User)
	}
	if strings.Contains(strings.Join(got.User.Commands, ","), "canary") || strings.Contains(strings.Join(got.User.Categories, ","), ">") {
		t.Fatalf("inspect leaked password: %#v", got.User)
	}
	line := aclLineFor(t, mem.ACLLines, "project_a")
	if strings.Contains(line, "canary-secret") {
		t.Fatalf("canary remained on ACL line: %q", line)
	}
	if strings.Contains(line, ">"+got.Password) && strings.Contains(got.Password, "canary") {
		t.Fatal("generated password collided with canary")
	}
}

func TestRotateUserMapsSetUserModifierErrorWithoutCanary(t *testing.T) {
	mem := &MemoryClient{
		ACLLines:      []string{rotateACLLine},
		ACLSetUserErr: errors.New("ERR Error in ACL SETUSER modifier '>canary-secret': Syntax error"),
	}
	svc := NewService(mem)
	_, err := svc.RotateUser(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), ">") {
		t.Fatalf("leaked SETUSER modifier: %v", err)
	}
}

func TestRotateUserNilClientIsNotConfigured(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.RotateUser(context.Background(), "project_a"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

func assertRotateRulesOnly(t *testing.T, call ACLSetUserCall, username, password string) {
	t.Helper()
	if call.Username != username {
		t.Fatalf("username = %q want %q", call.Username, username)
	}
	if len(call.Rules) != 2 || call.Rules[0] != "resetpass" || call.Rules[1] != ">"+password {
		t.Fatalf("rules = %#v want [resetpass >password]", call.Rules)
	}
	for _, rule := range call.Rules {
		if rule == "reset" || rule == "resetkeys" || rule == "resetchannels" || rule == "nocommands" || rule == "-@all" || rule == "on" || rule == "off" {
			t.Fatalf("forbidden rule %q", rule)
		}
		if strings.HasPrefix(rule, "~") {
			t.Fatalf("forbidden key rule %q", rule)
		}
	}
}

func assertRotateLinePreserved(t *testing.T, lines []string, username, password string) {
	t.Helper()
	line := aclLineFor(t, lines, username)
	if !strings.Contains(line, "on") {
		t.Fatalf("lost on/off: %q", line)
	}
	if !strings.Contains(line, "~project_a:*") || !strings.Contains(line, "-@all") {
		t.Fatalf("lost key/category rules: %q", line)
	}
	if !strings.Contains(line, "+echo") || !strings.Contains(line, "+get") || !strings.Contains(line, "+ping") {
		t.Fatalf("lost commands: %q", line)
	}
	if strings.Contains(line, "#9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08") {
		t.Fatalf("old hash remains: %q", line)
	}
	if !strings.Contains(line, ">"+password) {
		t.Fatalf("missing rotated password token: %q", line)
	}
}

func aclLineFor(t *testing.T, lines []string, username string) string {
	t.Helper()
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "user" && fields[1] == username {
			return line
		}
	}
	t.Fatalf("user %q not in ACL lines %#v", username, lines)
	return ""
}
