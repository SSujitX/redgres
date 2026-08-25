package redisadmin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Frozen inspect membership from 74e41a2 (identical on 6ac634f). Independent of
// the inspect* variables so a silent grant-set edit fails this test.
var frozenConnectionSafe = []string{"ping", "echo", "hello", "quit"}

var frozenCacheReadWriteExtra = []string{
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

var frozenReadOnlyExtra = []string{
	"get", "mget", "exists", "ttl", "pttl", "type", "strlen", "getrange", "scan",
	"hget", "hmget", "hgetall", "hexists", "hkeys", "hvals", "hlen", "hscan", "hrandfield",
	"sismember", "smembers", "scard", "sscan", "srandmember",
	"lrange", "llen", "lindex", "lpos",
	"zrange", "zrevrange", "zrangebyscore", "zcard", "zscore", "zrank", "zrevrank",
	"zcount", "zscan", "zmscore",
	"getbit", "bitcount", "bitpos",
}

var frozenQueueListsExtra = []string{
	"lpush", "rpush", "lpop", "rpop", "blpop", "brpop", "llen", "lrange", "lindex",
	"ltrim", "lmove", "blmove", "lpos", "linsert", "rpoplpush", "brpoplpush", "lmpop", "blmpop",
}

var frozenQueueStreamsExtra = []string{
	"xadd", "xread", "xreadgroup", "xgroup", "xack", "xpending", "xclaim", "xautoclaim",
	"xlen", "xrange", "xrevrange", "xdel", "xtrim", "xinfo", "xsetid",
}

var frozenQueueSortedSetsExtra = []string{
	"zadd", "zrem", "zrange", "zrevrange", "zpopmin", "zpopmax", "bzpopmin", "bzpopmax",
	"zcard", "zscore", "zrank", "zcount", "zincrby", "zmscore", "zrangebyscore",
	"zremrangebyrank", "zremrangebyscore", "zscan", "bzmpop", "zmpop",
}

var allowListDangerousFixture = []string{
	"acl", "config", "debug", "module", "shutdown", "flushall", "flushdb",
	"eval", "evalsha", "script", "keys", "@all",
}

func TestInspectSetsEqual74e41a2(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{name: "cache-read-write", got: inspectCacheReadWrite, want: concat(frozenConnectionSafe, frozenCacheReadWriteExtra)},
		{name: "read-only", got: inspectReadOnly, want: concat(frozenConnectionSafe, frozenReadOnlyExtra)},
		{name: "queue-lists", got: inspectQueueLists, want: concat(frozenConnectionSafe, frozenQueueListsExtra)},
		{name: "queue-streams", got: inspectQueueStreams, want: concat(frozenConnectionSafe, frozenQueueStreamsExtra)},
		{name: "queue-sorted-sets", got: inspectQueueSortedSets, want: concat(frozenConnectionSafe, frozenQueueSortedSetsExtra)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !equalSet(tc.got, tc.want) {
				t.Fatalf("inspect membership drifted from 74e41a2")
			}
		})
	}
}

func TestAllowedCommandsIsUniqueSortedUnionOfNamedPresets(t *testing.T) {
	got := AllowedCommands()
	if got == nil {
		t.Fatal("AllowedCommands returned nil")
	}
	var union []string
	for _, p := range NamedPresets() {
		union = append(union, p.Commands...)
	}
	want := uniqueSorted(union)
	if !equalSet(got, want) {
		t.Fatalf("AllowedCommands is not the NamedPresets union")
	}
	if len(got) == 0 {
		t.Fatal("AllowedCommands empty")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("not unique-sorted: %#v", got)
		}
		if got[i] != strings.ToLower(got[i]) {
			t.Fatalf("not lowercase: %q", got[i])
		}
	}
}

func TestAllowedCommandsDisjointFromDangerousFixture(t *testing.T) {
	allowed := map[string]struct{}{}
	for _, cmd := range AllowedCommands() {
		allowed[cmd] = struct{}{}
	}
	for _, bad := range allowListDangerousFixture {
		if _, ok := allowed[bad]; ok {
			t.Fatalf("dangerous %q is in AllowedCommands", bad)
		}
	}
}

func TestUpdatePermissionsCustomGrantVectors(t *testing.T) {
	cases := []struct {
		name     string
		commands []string
		want     []string
	}{
		{name: "cache-read-write-subset", commands: []string{"ECHO", " get ", "ping", "GET"}, want: []string{"echo", "get", "ping"}},
		{name: "connection-safe-only", commands: []string{"quit", "hello", "echo", "ping"}, want: []string{"echo", "hello", "ping", "quit"}},
		{name: "queue-lists-subset", commands: []string{"lpush", "blpop", "rpop"}, want: []string{"blpop", "lpush", "rpop"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &MemoryClient{ACLLines: []string{updateACLLine}}
			svc := NewService(mem)
			got, err := svc.UpdatePermissions(context.Background(), "project_a", "other_app", PresetCustom, "", tc.commands)
			if err != nil {
				t.Fatal(err)
			}
			if got.Username != "project_a" || !got.Enabled || got.KeyPattern != "other_app:*" {
				t.Fatalf("user = %#v", got)
			}
			if got.Preset != PresetCustom {
				t.Fatalf("preset = %q", got.Preset)
			}
			if got.QueueKind != "" || got.Protected || got.RuleFidelity != RuleExact {
				t.Fatalf("user = %#v", got)
			}
			if !equalSet(got.Commands, tc.want) {
				t.Fatalf("commands = %#v want %#v", got.Commands, tc.want)
			}
			if len(mem.ACLSetUserCalls) != 1 {
				t.Fatalf("ACLSetUser calls = %d", len(mem.ACLSetUserCalls))
			}
			assertUpdateRules(t, mem.ACLSetUserCalls[0].Rules, "other_app:*", tc.want)
			assertUpdateLinePreserved(t, mem.ACLLines, "project_a", "on", "other_app:*", "#9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
		})
	}
}

func TestUpdatePermissionsCustomMatchingNamedSetInfersNamedPreset(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{updateACLLine}}
	svc := NewService(mem)
	got, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", PresetCustom, "", inspectReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Preset != PresetReadOnly {
		t.Fatalf("inferPreset = %q want %s", got.Preset, PresetReadOnly)
	}
	if !equalSet(got.Commands, inspectReadOnly) {
		t.Fatalf("commands mismatch")
	}
}

func TestUpdatePermissionsCustomRejectsBeforeRedis(t *testing.T) {
	cases := []struct {
		name     string
		commands []string
	}{
		{name: "flushall", commands: []string{"flushall"}},
		{name: "acl", commands: []string{"acl"}},
		{name: "config", commands: []string{"config"}},
		{name: "eval", commands: []string{"eval"}},
		{name: "at-all", commands: []string{"@all"}},
		{name: "plus", commands: []string{"+get"}},
		{name: "minus", commands: []string{"-@all"}},
		{name: "tilde", commands: []string{"~project:*"}},
		{name: "gt", commands: []string{">secret"}},
		{name: "pipe", commands: []string{"get|set"}},
		{name: "space", commands: []string{"get extra"}},
		{name: "reset", commands: []string{"reset"}},
		{name: "unknown", commands: []string{"notacommand"}},
		{name: "empty", commands: nil},
		{name: "blank", commands: []string{"", "  "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &MemoryClient{
				ACLLines:   []string{updateACLLine},
				ACLListErr: errors.New("acl list should not run"),
			}
			svc := NewService(mem)
			_, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", PresetCustom, "", tc.commands)
			if !errors.Is(err, ErrInvalidCommands) {
				t.Fatalf("err = %v", err)
			}
			if len(mem.ACLSetUserCalls) != 0 {
				t.Fatalf("SETUSER on reject: %#v", mem.ACLSetUserCalls)
			}
		})
	}
}

func TestUpdatePermissionsNamedRejectsCommandsBeforeRedis(t *testing.T) {
	mem := &MemoryClient{
		ACLLines:   []string{updateACLLine},
		ACLListErr: errors.New("acl list should not run"),
	}
	svc := NewService(mem)
	_, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", PresetReadOnly, "", []string{"get"})
	if !errors.Is(err, ErrInvalidCommands) {
		t.Fatalf("err = %v", err)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("SETUSER on named+commands: %#v", mem.ACLSetUserCalls)
	}
}

func TestUpdatePermissionsCustomTooManyCommands(t *testing.T) {
	mem := &MemoryClient{ACLLines: []string{updateACLLine}}
	svc := NewService(mem)
	cmds := make([]string, maxACLCommands+1)
	for i := range cmds {
		cmds[i] = "ping"
	}
	_, err := svc.UpdatePermissions(context.Background(), "project_a", "project_a", PresetCustom, "", cmds)
	if !errors.Is(err, ErrInvalidCommands) {
		t.Fatalf("err = %v", err)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("SETUSER on oversized commands: %#v", mem.ACLSetUserCalls)
	}
}
