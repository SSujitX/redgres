package redisadmin

import (
	"context"
	"errors"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/redis/go-redis/v9"
)

type discardLog struct{}

func (discardLog) Printf(context.Context, string, ...any) {}

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

func Open(_ context.Context, cfg config.Config) (*Service, func(), error) {
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
	redis.SetLogger(discardLog{})
	opts, err := redis.ParseURL(adminURL)
	adminURL = ""
	if err != nil {
		return nil, noop, errors.New("REDGRES_REDIS_ADMIN_URL_FILE: invalid value")
	}
	if opts.TLSConfig != nil && opts.TLSConfig.InsecureSkipVerify {
		return nil, noop, errors.New("REDGRES_REDIS_ADMIN_URL_FILE: invalid value")
	}
	opts.MaxRetries = 1
	client := redis.NewClient(opts)
	return NewService(goRedisClient{inner: client}), func() { _ = client.Close() }, nil
}
