package redisadmin

import "errors"

var (
	ErrNotConfigured    = errors.New("not configured")
	ErrUnavailable      = errors.New("dependency unavailable")
	ErrAuthFailed       = errors.New("auth failed")
	ErrPermissionDenied = errors.New("permission denied")
)
