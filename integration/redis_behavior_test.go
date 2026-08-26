package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/redisadmin"
	"github.com/redis/go-redis/v9"
)

const behaviorTimeout = 45 * time.Second

// openLiveRedis opens the admin service against the REDGRES_TEST_REDIS_URL_FILE
// endpoint and returns the service plus the dial address for user connections.
func openLiveRedis(t *testing.T) (*redisadmin.Service, string) {
	t.Helper()
	clearInheritedRedgresEnv(t)
	urlFile, ok := liveRedisEnv(t)
	if !ok {
		t.Skip(skipLiveEnv)
	}
	raw, err := os.ReadFile(urlFile)
	if err != nil {
		t.Fatalf("read redis url file: %v", err)
	}
	opts, err := redis.ParseURL(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	cfg := config.Config{
		Environment:         config.EnvironmentDevelopment,
		RedisAdminURLFile:   urlFile,
		RedisExpectedSeries: liveRedisExpectedSeries(t),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	svc, closer, err := redisadmin.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if closer != nil {
		t.Cleanup(closer)
	}
	return svc, opts.Addr
}

// liveRedisClient dials Redis as username/password (empty username = default
// user) with no retries so auth-failure assertions see the real outcome.
func liveRedisClient(t *testing.T, addr, username, password string) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr:       addr,
		Username:   username,
		Password:   password,
		MaxRetries: 0,
	})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createLiveRedisUser(t *testing.T, svc *redisadmin.Service, username, prefix, preset, queueKind string, commands []string) redisadmin.CreateResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), behaviorTimeout)
	defer cancel()
	res, err := svc.CreateUser(ctx, username, prefix, preset, queueKind, commands)
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", username, err)
	}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), behaviorTimeout)
		defer cleanCancel()
		_ = svc.DeleteUser(cleanCtx, username)
	})
	return res
}

func TestLiveRedisCreateIsolationAndInspect(t *testing.T) {
	svc, addr := openLiveRedis(t)
	const name = "redgres_it_ro"
	res := createLiveRedisUser(t, svc, name, "redgres:it", redisadmin.PresetReadOnly, "", nil)
	if res.Password == "" {
		t.Fatal("create returned empty password")
	}
	if !res.User.Enabled {
		t.Fatal("created user is disabled")
	}
	if res.User.KeyPattern != "redgres:it:*" {
		t.Fatalf("key pattern = %q want redgres:it:*", res.User.KeyPattern)
	}
	if res.User.Preset != redisadmin.PresetReadOnly {
		t.Fatalf("preset = %q", res.User.Preset)
	}
	if len(res.User.Commands) == 0 {
		t.Fatal("read-only preset produced no commands")
	}
	for _, cmd := range res.User.Commands {
		if cmd == "set" || cmd == "flushall" || cmd == "config" {
			t.Fatalf("read-only preset grants %q", cmd)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), behaviorTimeout)
	defer cancel()

	u, err := svc.GetUser(ctx, name)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !u.Enabled || u.KeyPattern != "redgres:it:*" {
		t.Fatalf("GetUser mismatch: %+v", u)
	}
	if u.Preset != redisadmin.PresetReadOnly || u.RuleFidelity != redisadmin.RuleExact {
		t.Fatalf("GetUser preset = %q fidelity = %q want read-only/exact (REDIS-002 inspect)", u.Preset, u.RuleFidelity)
	}
	listed, err := svc.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	found := false
	for _, item := range listed.Users {
		if item.Username == name {
			found = true
			if item.Protected {
				t.Fatal("created user reported protected")
			}
		}
	}
	if !found {
		t.Fatalf("ListUsers missing %s", name)
	}
	search, err := svc.Search(ctx, "redgres_it", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	searched := false
	for _, hit := range search.Hits {
		if hit.Name == name {
			searched = true
		}
	}
	if !searched {
		t.Fatal("Search missing created user")
	}

	client := liveRedisClient(t, addr, name, res.Password)
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("user ping: %v", err)
	}
	admin := liveRedisClient(t, addr, "", "")
	if err := admin.Set(ctx, "redgres:it:item", "v", 0).Err(); err != nil {
		t.Fatalf("admin seed: %v", err)
	}
	if v, err := client.Get(ctx, "redgres:it:item").Result(); err != nil || v != "v" {
		t.Fatalf("read within prefix = %q err %v", v, err)
	}
	if err := client.Set(ctx, "redgres:it:item", "x", 0).Err(); err == nil {
		t.Fatal("read-only user wrote a key")
	}
	if err := client.Get(ctx, "other:key").Err(); err == nil {
		t.Fatal("cross-prefix read allowed")
	}
	if err := client.Do(ctx, "FLUSHALL").Err(); err == nil {
		t.Fatal("FLUSHALL allowed")
	}
	if err := client.Do(ctx, "CONFIG", "GET", "maxmemory").Err(); err == nil {
		t.Fatal("CONFIG allowed")
	}
}

func TestLiveRedisPatchKeepsPasswordAndRotateInvalidates(t *testing.T) {
	svc, addr := openLiveRedis(t)
	const name = "redgres_it_rw"
	res := createLiveRedisUser(t, svc, name, "redgres:it", redisadmin.PresetReadOnly, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), behaviorTimeout)
	defer cancel()

	u, err := svc.UpdatePermissions(ctx, name, "redgres:new", redisadmin.PresetCacheReadWrite, "", nil)
	if err != nil {
		t.Fatalf("UpdatePermissions: %v", err)
	}
	if u.KeyPattern != "redgres:new:*" {
		t.Fatalf("patched key pattern = %q", u.KeyPattern)
	}
	if u.Preset != redisadmin.PresetCacheReadWrite {
		t.Fatalf("patched preset = %q", u.Preset)
	}
	// PATCH preserves the password.
	client := liveRedisClient(t, addr, name, res.Password)
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("old password rejected after PATCH: %v", err)
	}
	// New grants are effective.
	if err := client.Set(ctx, "redgres:new:item", "v", 0).Err(); err != nil {
		t.Fatalf("set after PATCH: %v", err)
	}
	// Old scope is no longer writable.
	if err := client.Set(ctx, "redgres:it:item", "v", 0).Err(); err == nil {
		t.Fatal("old prefix still writable after PATCH")
	}

	rot, err := svc.RotateUser(ctx, name)
	if err != nil {
		t.Fatalf("RotateUser: %v", err)
	}
	if rot.Password == "" {
		t.Fatal("rotate returned empty password")
	}
	oldClient := liveRedisClient(t, addr, name, res.Password)
	if err := oldClient.Ping(ctx).Err(); err == nil {
		t.Fatal("old password still accepted after rotate")
	}
	newClient := liveRedisClient(t, addr, name, rot.Password)
	if err := newClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("new password ping: %v", err)
	}
}

func TestLiveRedisEnableDisable(t *testing.T) {
	svc, addr := openLiveRedis(t)
	const name = "redgres_it_toggle"
	res := createLiveRedisUser(t, svc, name, "redgres:it", redisadmin.PresetReadOnly, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), behaviorTimeout)
	defer cancel()

	u, err := svc.SetEnabled(ctx, name, false)
	if err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if u.Enabled {
		t.Fatal("SetEnabled(false) reported enabled")
	}
	disabled := liveRedisClient(t, addr, name, res.Password)
	if err := disabled.Ping(ctx).Err(); err == nil {
		t.Fatal("disabled user still authenticates")
	}

	u, err = svc.SetEnabled(ctx, name, true)
	if err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if !u.Enabled {
		t.Fatal("SetEnabled(true) reported disabled")
	}
	enabled := liveRedisClient(t, addr, name, res.Password)
	if err := enabled.Ping(ctx).Err(); err != nil {
		t.Fatalf("re-enabled user ping: %v", err)
	}
}

func TestLiveRedisDeleteAndGuards(t *testing.T) {
	svc, _ := openLiveRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), behaviorTimeout)
	defer cancel()

	const name = "redgres_it_del"
	createLiveRedisUser(t, svc, name, "redgres:it", redisadmin.PresetReadOnly, "", nil)

	// Duplicate create conflicts while the user exists.
	if _, err := svc.CreateUser(ctx, name, "redgres:it", redisadmin.PresetReadOnly, "", nil); !errors.Is(err, redisadmin.ErrConflict) {
		t.Fatalf("duplicate create = %v, want ErrConflict", err)
	}
	if err := svc.DeleteUser(ctx, name); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := svc.GetUser(ctx, name); !errors.Is(err, redisadmin.ErrNotFound) {
		t.Fatalf("GetUser after delete = %v, want ErrNotFound", err)
	}
	listed, err := svc.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	for _, item := range listed.Users {
		if item.Username == name {
			t.Fatal("deleted user still listed")
		}
	}

	// Protected names cannot be created.
	if _, err := svc.CreateUser(ctx, "default", "redgres:it", redisadmin.PresetReadOnly, "", nil); !errors.Is(err, redisadmin.ErrProtectedUser) {
		t.Fatalf("create protected = %v, want ErrProtectedUser", err)
	}
	// Commands outside the allow-list fail closed before Redis.
	if _, err := svc.CreateUser(ctx, "redgres_it_bad", "redgres:it", redisadmin.PresetCustom, "", []string{"CONFIG"}); !errors.Is(err, redisadmin.ErrInvalidCommands) {
		t.Fatalf("create with CONFIG = %v, want ErrInvalidCommands", err)
	}
}

func TestLiveRedisQueueWorkerStreamsWorkload(t *testing.T) {
	svc, addr := openLiveRedis(t)
	const name = "redgres_it_q"
	res := createLiveRedisUser(t, svc, name, "redgres:q", redisadmin.PresetQueueWorker, redisadmin.QueueStreams, nil)
	if res.User.Preset != redisadmin.PresetQueueWorker || res.User.QueueKind != redisadmin.QueueStreams {
		t.Fatalf("queue worker = %+v", res.User)
	}
	ctx, cancel := context.WithTimeout(context.Background(), behaviorTimeout)
	defer cancel()
	client := liveRedisClient(t, addr, name, res.Password)
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("user ping: %v", err)
	}
	id, err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: "redgres:q:events",
		Values: map[string]any{"msg": "hello"},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}
	if _, err := client.XGroupCreate(ctx, "redgres:q:events", "group1", "0").Result(); err != nil {
		t.Fatalf("XGroupCreate: %v", err)
	}
	msgs, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    "group1",
		Consumer: "c1",
		Streams:  []string{"redgres:q:events", ">"},
		Count:    1,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup: %v", err)
	}
	if len(msgs) != 1 || len(msgs[0].Messages) != 1 || msgs[0].Messages[0].ID != id {
		t.Fatalf("XReadGroup unexpected: %+v", msgs)
	}
	if n, err := client.XAck(ctx, "redgres:q:events", "group1", id).Result(); err != nil || n != 1 {
		t.Fatalf("XAck n=%d err=%v", n, err)
	}
	if n, err := client.XLen(ctx, "redgres:q:events").Result(); err != nil || n != 1 {
		t.Fatalf("XLen n=%d err=%v", n, err)
	}
	if _, err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: "other:events",
		Values: map[string]any{"msg": "x"},
	}).Result(); err == nil {
		t.Fatal("cross-prefix XAdd allowed")
	}
}

// TestLiveRedisQueueWorkerListsWorkload runs a representative Lists queue
// workload (REDIS-004) against real Redis within the queue-worker/lists key
// prefix and asserts cross-prefix denial.
func TestLiveRedisQueueWorkerListsWorkload(t *testing.T) {
	svc, addr := openLiveRedis(t)
	const name = "redgres_it_ql"
	res := createLiveRedisUser(t, svc, name, "redgres:q", redisadmin.PresetQueueWorker, redisadmin.QueueLists, nil)
	if res.User.Preset != redisadmin.PresetQueueWorker || res.User.QueueKind != redisadmin.QueueLists {
		t.Fatalf("queue worker = %+v", res.User)
	}
	ctx, cancel := context.WithTimeout(context.Background(), behaviorTimeout)
	defer cancel()
	client := liveRedisClient(t, addr, name, res.Password)
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("user ping: %v", err)
	}
	if n, err := client.LPush(ctx, "redgres:q:jobs", "a", "b", "c").Result(); err != nil || n != 3 {
		t.Fatalf("LPush n=%d err=%v", n, err)
	}
	if n, err := client.RPush(ctx, "redgres:q:jobs", "d").Result(); err != nil || n != 4 {
		t.Fatalf("RPush n=%d err=%v", n, err)
	}
	if v, err := client.RPopLPush(ctx, "redgres:q:jobs", "redgres:q:done").Result(); err != nil || v != "d" {
		t.Fatalf("RPopLPush = %q err %v", v, err)
	}
	if n, err := client.LLen(ctx, "redgres:q:jobs").Result(); err != nil || n != 3 {
		t.Fatalf("LLen n=%d err=%v", n, err)
	}
	vals, err := client.LRange(ctx, "redgres:q:jobs", 0, -1).Result()
	if err != nil || len(vals) != 3 {
		t.Fatalf("LRange = %v err %v", vals, err)
	}
	if v, err := client.LPop(ctx, "redgres:q:jobs").Result(); err != nil || v != "c" {
		t.Fatalf("LPop = %q err %v", v, err)
	}
	if _, err := client.LPush(ctx, "other:queue", "x").Result(); err == nil {
		t.Fatal("cross-prefix LPush allowed")
	}
}

// TestLiveRedisQueueWorkerSortedSetsWorkload runs a representative Sorted
// Sets queue workload (REDIS-004) against real Redis within the
// queue-worker/sorted-sets key prefix and asserts cross-prefix denial.
func TestLiveRedisQueueWorkerSortedSetsWorkload(t *testing.T) {
	svc, addr := openLiveRedis(t)
	const name = "redgres_it_qz"
	res := createLiveRedisUser(t, svc, name, "redgres:q", redisadmin.PresetQueueWorker, redisadmin.QueueSortedSets, nil)
	if res.User.Preset != redisadmin.PresetQueueWorker || res.User.QueueKind != redisadmin.QueueSortedSets {
		t.Fatalf("queue worker = %+v", res.User)
	}
	ctx, cancel := context.WithTimeout(context.Background(), behaviorTimeout)
	defer cancel()
	client := liveRedisClient(t, addr, name, res.Password)
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("user ping: %v", err)
	}
	if n, err := client.ZAdd(ctx, "redgres:q:rank",
		redis.Z{Score: 1, Member: "job1"}, redis.Z{Score: 2, Member: "job2"}, redis.Z{Score: 3, Member: "job3"}).Result(); err != nil || n != 3 {
		t.Fatalf("ZAdd n=%d err=%v", n, err)
	}
	if n, err := client.ZCard(ctx, "redgres:q:rank").Result(); err != nil || n != 3 {
		t.Fatalf("ZCard n=%d err=%v", n, err)
	}
	if s, err := client.ZScore(ctx, "redgres:q:rank", "job2").Result(); err != nil || s != 2 {
		t.Fatalf("ZScore = %v err %v", s, err)
	}
	vals, err := client.ZRange(ctx, "redgres:q:rank", 0, -1).Result()
	if err != nil || len(vals) != 3 || vals[0] != "job1" {
		t.Fatalf("ZRange = %v err %v", vals, err)
	}
	popped, err := client.ZPopMin(ctx, "redgres:q:rank").Result()
	if err != nil || len(popped) != 1 || popped[0].Member != "job1" {
		t.Fatalf("ZPopMin = %v err %v", popped, err)
	}
	if _, err := client.ZAdd(ctx, "other:rank", redis.Z{Score: 1, Member: "x"}).Result(); err == nil {
		t.Fatal("cross-prefix ZAdd allowed")
	}
}
