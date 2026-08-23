package postgresadmin

import "errors"

var (
	ErrInvalidIdentifier = errors.New("invalid identifier")
	ErrNotFound          = errors.New("not found")
	ErrUnavailable       = errors.New("dependency unavailable")
)
