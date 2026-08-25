package redisadmin

import "errors"

var (
	ErrNotConfigured    = errors.New("not configured")
	ErrUnavailable      = errors.New("dependency unavailable")
	ErrAuthFailed       = errors.New("auth failed")
	ErrPermissionDenied = errors.New("permission denied")
	ErrNotFound         = errors.New("not found")
	ErrInvalidUsername  = errors.New("invalid username")
	ErrInvalidPrefix    = errors.New("invalid key prefix")
	ErrInvalidPreset    = errors.New("invalid preset")
	ErrInvalidQueueKind = errors.New("invalid queue kind")
	ErrProtectedUser    = errors.New("protected resource")
	ErrConflict         = errors.New("conflict")
)
