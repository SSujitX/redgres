package postgresadmin

import "errors"

var (
	ErrInvalidIdentifier   = errors.New("invalid identifier")
	ErrNotFound            = errors.New("not found")
	ErrUnavailable         = errors.New("dependency unavailable")
	ErrNotConfigured       = errors.New("not configured")
	ErrVaultUnavailable    = errors.New("vault unavailable")
	ErrProtected           = errors.New("protected resource")
	ErrConflict            = errors.New("conflict")
	ErrOperationInProgress = errors.New("operation in progress")
	ErrVaultUnsynced       = errors.New("vault unsynced after rotate")
)

const vaultUnsyncedMessage = "The PostgreSQL password was changed but the vault could not be saved. Rotate again."

// VaultUnsynced is a 503 after ALTER succeeded but vault upsert failed.
type VaultUnsynced struct {
	Database string
	Owner    string
}

func (VaultUnsynced) Error() string {
	return vaultUnsyncedMessage
}

func (VaultUnsynced) Unwrap() error {
	return ErrVaultUnsynced
}

const (
	conflictFieldDatabase = "database"
	conflictFieldOwner    = "owner"
)

// Conflict is a 409 with fields.database or fields.owner.
type Conflict struct {
	Field string
}

func (c Conflict) Error() string {
	switch c.Field {
	case conflictFieldDatabase:
		return "A PostgreSQL database with this name already exists"
	case conflictFieldOwner:
		return "A PostgreSQL role with this name already exists"
	default:
		return "conflict"
	}
}

func (c Conflict) Unwrap() error {
	return ErrConflict
}
