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

const duplicateInProgressMessage = "A database duplicate is already in progress."

const truncateInProgressMessage = "A truncate is already in progress."

const tableListTruncatedMessage = "Table list is truncated. Truncate cannot run."

const isolationChangedMessage = "The source database ownership or CONNECT ACL changed during duplicate. The clone was rolled back."

const duplicateSameOwnerMessage = "Choose a different project user than the source database owner."

// DuplicateInProgress is a 409 distinct from rotate's in-progress copy.
type DuplicateInProgress struct{}

func (DuplicateInProgress) Error() string {
	return duplicateInProgressMessage
}

func (DuplicateInProgress) Unwrap() error {
	return ErrOperationInProgress
}

// TruncateInProgress is a 409 distinct from rotate/duplicate in-progress copy.
type TruncateInProgress struct{}

func (TruncateInProgress) Error() string {
	return truncateInProgressMessage
}

func (TruncateInProgress) Unwrap() error {
	return ErrOperationInProgress
}

// TableListTruncated is a 409 conflict when GET-tables would be truncated.
type TableListTruncated struct{}

func (TableListTruncated) Error() string {
	return tableListTruncatedMessage
}

func (TableListTruncated) Unwrap() error {
	return ErrConflict
}

// IsolationChanged is a 503 after the clone is rolled back.
type IsolationChanged struct{}

func (IsolationChanged) Error() string {
	return isolationChangedMessage
}

func (IsolationChanged) Unwrap() error {
	return ErrUnavailable
}

// FieldError is a 400 with fields.database or fields.owner.
type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "validation error"
}

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
