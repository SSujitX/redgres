package redisadmin

import "errors"

var (
	ErrNotConfigured = errors.New("not configured")
	ErrUnavailable   = errors.New("dependency unavailable")
)
