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
	got, err := svc.CreateUser(context.Background(), "project_a", "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.User.Username != "project_a" || !got.User.Enabled || got.User.KeyPattern != "project_a:*" {
		t.Fatalf("user = %#v", got.User)
	}
	if got.User.Preset != PresetCacheReadWrite || got.User.Protected || got.User.RuleFidelity != RuleExact {
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
	assertCreateRules(t, call.Rules, got.Password, "project_a:*")
}

func TestCreateUserRejectsProtectedAndDuplicate(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{"user project_a on ~project_a:* -@all +ping"}}
	svc := NewServiceAdmin(mem, "ops_admin")
	if _, err := svc.CreateUser(context.Background(), "admin", "project_a"); !errors.Is(err, ErrProtectedUser) {
		t.Fatalf("admin err = %v", err)
	}
	if _, err := svc.CreateUser(context.Background(), "ops_admin", "ops"); !errors.Is(err, ErrProtectedUser) {
		t.Fatalf("configured admin err = %v", err)
	}
	if _, err := svc.CreateUser(context.Background(), "project_a", "project_a"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate err = %v", err)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("ACLSetUser called on reject: %#v", mem.ACLSetUserCalls)
	}
}

func TestCreateUserNilClientIsNotConfigured(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.CreateUser(context.Background(), "project_a", "project_a"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateUserMapsRedisFailure(t *testing.T) {
	svc := NewService(&MemoryClient{ACLListErr: errors.New("NOAUTH Authentication required. canary-secret")})
	_, err := svc.CreateUser(context.Background(), "project_a", "project_a")
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
	_, err := svc.CreateUser(context.Background(), "project_a", "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), ">") {
		t.Fatalf("leaked SETUSER modifier: %v", err)
	}
}

func assertCreateRules(t *testing.T, rules []string, password, pattern string) {
	t.Helper()
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
	hasPing, hasGet := false, false
	for _, rule := range rules[6:] {
		if rule == "+PING" {
			hasPing = true
		}
		if rule == "+GET" {
			hasGet = true
		}
		upper := strings.ToUpper(rule)
		if upper == "+@ALL" || strings.Contains(upper, "+ACL") || strings.Contains(upper, "+CONFIG") || strings.Contains(upper, "+FLUSHALL") {
			t.Fatalf("dangerous rule %q in %s", rule, joined)
		}
	}
	if !hasPing && !hasGet {
		t.Fatalf("missing +PING or +GET in %s", joined)
	}
}
