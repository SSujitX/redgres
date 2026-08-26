package redisadmin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/release"
	"github.com/redis/go-redis/v9"
)

type discardLog struct{}

func (discardLog) Printf(context.Context, string, ...any) {}

func init() {
	// Discard library logs globally. Calling SetLogger from Open races with
	// leftover pool dials from a previous client (go-redis logger is process-wide).
	redis.SetLogger(discardLog{})
}

const redisInfoTimeout = 2 * time.Second

type goRedisClient struct {
	inner *redis.Client
}

func (c goRedisClient) Ping(ctx context.Context) error {
	if c.inner == nil {
		return ErrUnavailable
	}
	return c.inner.Ping(ctx).Err()
}

func (c goRedisClient) Info(ctx context.Context) (string, error) {
	if c.inner == nil {
		return "", ErrUnavailable
	}
	return c.inner.Info(ctx, "server", "clients", "memory", "stats").Result()
}

func (c goRedisClient) DBSize(ctx context.Context) (int64, error) {
	if c.inner == nil {
		return 0, ErrUnavailable
	}
	return c.inner.DBSize(ctx).Result()
}

func (c goRedisClient) ACLList(ctx context.Context) ([]string, error) {
	if c.inner == nil {
		return nil, ErrUnavailable
	}
	return c.inner.ACLList(ctx).Result()
}

func (c goRedisClient) ACLSetUser(ctx context.Context, username string, rules ...string) error {
	if c.inner == nil {
		return ErrUnavailable
	}
	return c.inner.ACLSetUser(ctx, username, rules...).Err()
}

func (c goRedisClient) ACLDelUser(ctx context.Context, username string) (int64, error) {
	if c.inner == nil {
		return 0, ErrUnavailable
	}
	return c.inner.ACLDelUser(ctx, username).Result()
}

func Open(ctx context.Context, cfg config.Config) (*Service, func(), error) {
	noop := func() {}
	if !cfg.RedisConfigured() {
		if cfg.Production() {
			return nil, noop, errors.New("REDGRES_REDIS_ADMIN_URL_FILE: production requires a complete administrative URL file")
		}
		return nil, noop, nil
	}
	adminURL, err := cfg.RedisAdminURL()
	if err != nil {
		return nil, noop, err
	}
	opts, err := redis.ParseURL(adminURL)
	adminURL = ""
	if err != nil {
		return nil, noop, errors.New("REDGRES_REDIS_ADMIN_URL_FILE: invalid value")
	}
	if opts.TLSConfig != nil && opts.TLSConfig.InsecureSkipVerify {
		return nil, noop, errors.New("REDGRES_REDIS_ADMIN_URL_FILE: invalid value")
	}
	adminUser := opts.Username
	opts.MaxRetries = 1
	client := redis.NewClient(opts)
	closer := func() { _ = client.Close() }
	probeCtx, cancel := context.WithTimeout(ctx, redisInfoTimeout)
	defer cancel()
	info, err := client.Info(probeCtx, "server").Result()
	if err != nil {
		closer()
		return nil, noop, ErrUnavailable
	}
	if err := checkServerSeries(info, cfg.RedisExpectedSeries); err != nil {
		closer()
		return nil, noop, err
	}
	return &Service{client: goRedisClient{inner: client}, adminUser: adminUser}, closer, nil
}

func checkServerSeries(info, expected string) error {
	version := parseInfo(info)["redis_version"]
	series, err := release.RedisSeriesFromVersion(version)
	if err != nil || !release.SupportedRedisSeries(series) {
		return ErrUnavailable
	}
	if expected != "" && (expected != series || !release.SupportedRedisSeries(expected)) {
		return fmt.Errorf("%w: REDGRES_REDIS_EXPECTED_SERIES", ErrUnavailable)
	}
	return nil
}
