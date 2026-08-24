package redisadmin

import (
	"sort"
	"strings"
)

const (
	PresetCacheReadWrite = "cache-read-write"
	PresetReadOnly       = "read-only"
	PresetQueueWorker    = "queue-worker"
	PresetCustom         = "custom"

	QueueLists      = "lists"
	QueueStreams    = "streams"
	QueueSortedSets = "sorted-sets"

	RuleExact   = "exact"
	RuleLimited = "limited"

	maxACLUsers    = 500
	maxACLCommands = 256
)

var inspectConnectionSafe = []string{"ping", "echo", "hello", "quit"}

// cache-read-write command set transcribed from redis-ui InferPreset data
// (redis-ui internal/redisadmin/presets.go). Used to infer inspect presets
// and to grant explicit +CMD rules on create.
var inspectCacheReadWrite = concat(inspectConnectionSafe, []string{
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
})

var inspectReadOnly = concat(inspectConnectionSafe, []string{
	"get", "mget", "exists", "ttl", "pttl", "type", "strlen", "getrange", "scan",
	"hget", "hmget", "hgetall", "hexists", "hkeys", "hvals", "hlen", "hscan", "hrandfield",
	"sismember", "smembers", "scard", "sscan", "srandmember",
	"lrange", "llen", "lindex", "lpos",
	"zrange", "zrevrange", "zrangebyscore", "zcard", "zscore", "zrank", "zrevrank",
	"zcount", "zscan", "zmscore",
	"getbit", "bitcount", "bitpos",
})

var inspectQueueLists = concat(inspectConnectionSafe, []string{
	"lpush", "rpush", "lpop", "rpop", "blpop", "brpop", "llen", "lrange", "lindex",
	"ltrim", "lmove", "blmove", "lpos", "linsert", "rpoplpush", "brpoplpush", "lmpop", "blmpop",
})

var inspectQueueStreams = concat(inspectConnectionSafe, []string{
	"xadd", "xread", "xreadgroup", "xgroup", "xack", "xpending", "xclaim", "xautoclaim",
	"xlen", "xrange", "xrevrange", "xdel", "xtrim", "xinfo", "xsetid",
})

var inspectQueueSortedSets = concat(inspectConnectionSafe, []string{
	"zadd", "zrem", "zrange", "zrevrange", "zpopmin", "zpopmax", "bzpopmin", "bzpopmax",
	"zcard", "zscore", "zrank", "zcount", "zincrby", "zmscore", "zrangebyscore",
	"zremrangebyrank", "zremrangebyscore", "zscan", "bzmpop", "zmpop",
})

type inspectPreset struct {
	preset    string
	queueKind string
}

func inferPreset(commands []string) inspectPreset {
	set := uniqueSorted(commands)
	switch {
	case equalSet(set, inspectCacheReadWrite):
		return inspectPreset{preset: PresetCacheReadWrite}
	case equalSet(set, inspectReadOnly):
		return inspectPreset{preset: PresetReadOnly}
	case equalSet(set, inspectQueueLists):
		return inspectPreset{preset: PresetQueueWorker, queueKind: QueueLists}
	case equalSet(set, inspectQueueStreams):
		return inspectPreset{preset: PresetQueueWorker, queueKind: QueueStreams}
	case equalSet(set, inspectQueueSortedSets):
		return inspectPreset{preset: PresetQueueWorker, queueKind: QueueSortedSets}
	default:
		return inspectPreset{preset: PresetCustom}
	}
}

func concat(parts ...[]string) []string {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]string, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return uniqueSorted(out)
}

func uniqueSorted(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func equalSet(a, b []string) bool {
	a = uniqueSorted(a)
	b = uniqueSorted(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
