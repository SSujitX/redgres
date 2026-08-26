package operations

import (
	"context"
	"errors"
	"time"
)

const (
	MaxRuntime            = 15 * time.Minute
	TerminalRetention     = 30 * 24 * time.Hour
	MaxTerminalOperations = 10000

	ActionDuplicate Action = "postgres.database.duplicate"

	StatusQueued        Status = "queued"
	StatusRunning       Status = "running"
	StatusCompensating  Status = "compensating"
	StatusInterrupted   Status = "interrupted"
	StatusIndeterminate Status = "indeterminate"
	StatusSucceeded     Status = "succeeded"
	StatusFailed        Status = "failed"
	StatusCanceled      Status = "canceled"

	PhaseAccepted              Phase = "accepted"
	PhaseCloning               Phase = "cloning"
	PhaseTransferringOwnership Phase = "transferring_ownership"
	PhaseVaulting              Phase = "vaulting"
	PhaseCompensating          Phase = "compensating"

	ResourceDatabase  ResourceKind = "postgres.database"
	ResourceRole      ResourceKind = "postgres.role"
	ResourceRedisUser ResourceKind = "redis.user"
)

var (
	ErrNotFound     = errors.New("operation not found")
	ErrInvalidID    = errors.New("operation id is invalid")
	ErrConflict     = errors.New("operation conflict")
	ErrIllegalEdge  = errors.New("illegal operation transition")
	ErrUnsafeResult = errors.New("operation result is not allowed")
	ErrUnsafeError  = errors.New("operation error is not allowed")
	ErrLockHeld     = errors.New("operation lock is held")
)

type Action string
type Status string
type Phase string
type ResourceKind string

type Operation struct {
	ID                string
	Action            Action
	Status            Status
	Phase             Phase
	Actor             string
	Target            string
	AcceptedRequestID string
	Result            *DuplicateResult
	Error             *OperationError
	CreatedAt         time.Time
	UpdatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
}

type DuplicateResult struct {
	Database string `json:"database"`
	Owner    string `json:"owner"`
	Source   string `json:"source"`
}

type OperationError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type ResourceLock struct {
	Kind ResourceKind
	Name string
}

type Transition struct {
	From       Status
	To         Status
	Phase      Phase
	Result     *DuplicateResult
	Error      *OperationError
	StartedAt  *time.Time
	FinishedAt *time.Time
	KeepLocks  bool
}

type ProbeOutcome struct {
	CloneExists    bool
	RoleExists     bool
	VaultRowExists bool
	Indeterminate  bool
}

type Probe interface {
	DuplicateState(ctx context.Context, op Operation) (ProbeOutcome, error)
}

type Compensator interface {
	CompensateDuplicate(ctx context.Context, op Operation) error
}

type Store interface {
	Get(ctx context.Context, id string) (Operation, error)
	ListQueued(ctx context.Context) ([]Operation, error)
	InsertQueued(ctx context.Context, op Operation, locks []ResourceLock) error
	Transition(ctx context.Context, id string, change Transition) error
	Reconcile(ctx context.Context, probe Probe, compensator Compensator, now time.Time) error
}
