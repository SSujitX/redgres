package redisadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const (
	officialACLHash = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	officialAntirez = "user antirez on #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 ~objects:* &* +@all -@admin -@dangerous"
	officialDefault = "user default on nopass ~* &* +@all"
)

// v1CacheReadWriteFromRedisUI is the inspect-only redis-ui InferPreset
// cache-read-write command set (connectionSafe + cache extras), transcribed
// from redis-ui internal/redisadmin/presets.go as independent test data.
var v1CacheReadWriteFromRedisUI = []string{
	"ping", "echo", "hello", "quit",
	"get", "set", "mget", "mset", "del", "unlink", "exists", "expire", "pexpire",
	"expireat", "pexpireat", "ttl", "pttl", "persist", "type", "strlen",
	"getrange", "setrange", "append", "incr", "incrby", "decr", "decrby",
	"getdel", "getex", "setex", "psetex", "setnx", "scan",
	"hget", "hset", "hmget", "hgetall", "hdel", "hexists", "hkeys", "hvals",
	"hlen", "hincrby", "hincrbyfloat", "hsetnx", "hscan", "hrandfield",
	"sadd", "srem", "sismember", "smembers", "scard", "sscan", "spop",
	"srandmember", "smove",
	"lpush", "rpush", "lpop", "rpop", "lrange", "llen", "lindex", "lset",
	"ltrim", "linsert", "lpos", "lmove",
	"zadd", "zrem", "zrange", "zrevrange", "zrangebyscore", "zcard", "zscore",
	"zrank", "zrevrank", "zcount", "zincrby", "zscan", "zpopmin", "zpopmax", "zmscore",
	"getbit", "setbit", "bitcount", "bitpos",
}

func TestParseACLOfficialListLines(t *testing.T) {
	def, ok := parseACLLine(officialDefault)
	if !ok {
		t.Fatal("expected default user to parse")
	}
	if def.Username != "default" || !def.Enabled || def.KeyPattern != "*" {
		t.Fatalf("default = %#v", def)
	}
	if !def.Protected || def.Preset != PresetCustom || def.RuleFidelity != RuleLimited {
		t.Fatalf("default labels = %#v", def)
	}
	if def.QueueKind != "" {
		t.Fatalf("default queue_kind = %q", def.QueueKind)
	}

	u, ok := parseACLLine(officialAntirez)
	if !ok {
		t.Fatal("expected antirez to parse")
	}
	if u.Username != "antirez" || !u.Enabled || u.KeyPattern != "objects:*" {
		t.Fatalf("antirez = %#v", u)
	}
	if u.Protected || u.Preset != PresetCustom || u.RuleFidelity != RuleLimited {
		t.Fatalf("antirez labels = %#v", u)
	}
	if len(u.Commands) != 0 {
		t.Fatalf("antirez commands = %#v", u.Commands)
	}
	wantCats := []string{"@admin", "@all", "@dangerous"}
	if strings.Join(u.Categories, ",") != strings.Join(wantCats, ",") {
		t.Fatalf("antirez categories = %#v", u.Categories)
	}
}

func TestParseACLStripsHashAndPlaintextCanary(t *testing.T) {
	line := "user project_a on #" + officialACLHash + " >canary-secret ~project_a:* -@all +ping"
	u, ok := parseACLLine(line)
	if !ok {
		t.Fatal("expected named user to parse")
	}
	if u.Username != "project_a" || !u.Enabled || u.KeyPattern != "project_a:*" {
		t.Fatalf("user = %#v", u)
	}
	raw, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	assertNoACLSecret(t, string(raw))
	assertNoACLSecret(t, u.Username+u.KeyPattern+u.Preset+strings.Join(u.Commands, ",")+strings.Join(u.Categories, ","))
}

func TestParseACLCategoryOnlyIsCustomLimited(t *testing.T) {
	u, ok := parseACLLine("user reader on ~cache:* +@read")
	if !ok {
		t.Fatal("expected reader to parse")
	}
	if u.Preset != PresetCustom || u.RuleFidelity != RuleLimited {
		t.Fatalf("labels = %#v", u)
	}
	if len(u.Commands) != 0 {
		t.Fatalf("commands = %#v want empty", u.Commands)
	}
	if strings.Join(u.Categories, ",") != "@read" {
		t.Fatalf("categories = %#v", u.Categories)
	}
}

func TestParseACLExactV1CacheReadWrite(t *testing.T) {
	u, ok := parseACLLine(aclLineWithCommands("project_a", "project_a:*", v1CacheReadWriteFromRedisUI))
	if !ok {
		t.Fatal("expected project_a to parse")
	}
	if u.Preset != PresetCacheReadWrite || u.RuleFidelity != RuleExact {
		t.Fatalf("labels = %#v", u)
	}
	if u.QueueKind != "" {
		t.Fatalf("queue_kind = %q", u.QueueKind)
	}
	if u.KeyPattern != "project_a:*" || !u.Enabled {
		t.Fatalf("user = %#v", u)
	}
}

func TestParseACLProtectedAdminUsername(t *testing.T) {
	u, ok := parseACLLine("user ops_admin on ~* -@all +ping")
	if !ok {
		t.Fatal("expected ops_admin to parse")
	}
	u.Protected = IsProtectedUsername(u.Username, "ops_admin")
	if !u.Protected {
		t.Fatal("ops_admin should be protected via admin URL username")
	}
	if !IsProtectedUsername("OPS_ADMIN", "ops_admin") {
		t.Fatal("EqualFold admin username should be protected")
	}
	if !IsProtectedUsername("default", "") || !IsProtectedUsername("admin", "") || !IsProtectedUsername("redact_admin", "") {
		t.Fatal("reserved names should be protected")
	}
	if IsProtectedUsername("project_a", "ops_admin") {
		t.Fatal("project_a should not be protected")
	}
}

func TestParseACLDropsUnnamedAndKeepsNamedUnmodelable(t *testing.T) {
	if _, ok := parseACLLine("not a user line"); ok {
		t.Fatal("unnamed line must be dropped")
	}
	if _, ok := parseACLLine("user"); ok {
		t.Fatal("user without name must be dropped")
	}
	u, ok := parseACLLine("user weird %R~foo leftover-token")
	if !ok {
		t.Fatal("named unmodelable line must still be listed")
	}
	if u.Username != "weird" || u.Preset != PresetCustom || u.RuleFidelity != RuleLimited {
		t.Fatalf("weird = %#v", u)
	}
}

func TestServiceListUsersTruncatesAt500(t *testing.T) {
	lines := make([]string, 0, 501)
	for i := 0; i < 501; i++ {
		lines = append(lines, fmt.Sprintf("user u%03d on ~u%03d:* -@all +ping", i, i))
	}
	svc := NewService(&MemoryClient{ACLLines: lines})
	list, err := svc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if !list.Truncated || len(list.Users) != 500 {
		t.Fatalf("len=%d truncated=%v", len(list.Users), list.Truncated)
	}
	if list.Users[0].Username != "u000" || list.Users[499].Username != "u499" {
		t.Fatalf("sort/cap = %q … %q", list.Users[0].Username, list.Users[499].Username)
	}
	got, err := svc.GetUser(context.Background(), "u500")
	if err != nil {
		t.Fatalf("GetUser u500: %v", err)
	}
	if got.Username != "u500" {
		t.Fatalf("u500 = %#v", got)
	}
}

func TestServiceListUsersClassifiesACLErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "noauth", err: errors.New("NOAUTH Authentication required. password=canary-secret host=10.0.0.1"), want: ErrAuthFailed},
		{name: "wrongpass", err: errors.New("WRONGPASS invalid username-password pair. password=canary-secret"), want: ErrAuthFailed},
		{name: "noperm", err: errors.New("NOPERM this user has no permissions to run the 'acl|list' command host=10.0.0.1"), want: ErrPermissionDenied},
		{name: "dial", err: errors.New("dial tcp 10.0.0.1:6379: connect: connection refused password=canary-secret"), want: ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(&MemoryClient{ACLListErr: tc.err})
			list, err := svc.ListUsers(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v want %v", err, tc.want)
			}
			assertNoRedisCanary(t, err.Error())
			if list.Users == nil {
				t.Fatal("users slice must not be nil on error")
			}
			_, getErr := svc.GetUser(context.Background(), "project_a")
			if !errors.Is(getErr, tc.want) {
				t.Fatalf("GetUser err = %v want %v", getErr, tc.want)
			}
			assertNoRedisCanary(t, getErr.Error())
		})
	}
}

func TestServiceListUsersDefaultACLListEmpty(t *testing.T) {
	svc := NewService(&MemoryClient{})
	list, err := svc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if list.Truncated || list.Users == nil || len(list.Users) != 0 {
		t.Fatalf("list = %#v", list)
	}
}

func TestServiceGetUserNotFound(t *testing.T) {
	svc := NewService(&MemoryClient{ACLLines: []string{"user project_a on ~project_a:* -@all +ping"}})
	_, err := svc.GetUser(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceProtectedUsersAreGetable(t *testing.T) {
	svc := &Service{
		client:    &MemoryClient{ACLLines: []string{"user ops_admin on ~* -@all +ping", "user default on nopass ~* &* +@all"}},
		adminUser: "ops_admin",
	}
	for _, name := range []string{"ops_admin", "default"} {
		u, err := svc.GetUser(context.Background(), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !u.Protected {
			t.Fatalf("%s protected = false", name)
		}
	}
}

func aclLineWithCommands(username, pattern string, commands []string) string {
	var b strings.Builder
	b.WriteString("user ")
	b.WriteString(username)
	b.WriteString(" on ~")
	b.WriteString(pattern)
	b.WriteString(" -@all")
	for _, c := range commands {
		b.WriteString(" +")
		b.WriteString(c)
	}
	return b.String()
}

func assertNoACLSecret(t *testing.T, raw string) {
	t.Helper()
	for _, leak := range []string{
		officialACLHash,
		"#" + officialACLHash,
		"canary-secret",
		">canary-secret",
		"acl_rule",
		"nopass",
	} {
		if strings.Contains(raw, leak) {
			t.Fatalf("leaked %q in %s", leak, raw)
		}
	}
}
