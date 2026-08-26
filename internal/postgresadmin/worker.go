package postgresadmin

import (
	"context"
	"errors"

	"github.com/SSujitX/redgres/internal/operations"
)

// DuplicateAuditor records secret-safe duplicate outcomes. audit.Store satisfies it.
type DuplicateAuditor interface {
	Record(actor, action, target, outcome, requestID, clientIP string, metadata map[string]any) error
}

type duplicateCompensator struct {
	svc *Service
}

// NewDuplicateCompensator drops leftover clone, role, and vault row before terminal lock release.
func NewDuplicateCompensator(svc *Service) operations.Compensator {
	return duplicateCompensator{svc: svc}
}

func (c duplicateCompensator) CompensateDuplicate(ctx context.Context, op operations.Operation) error {
	if c.svc == nil || c.svc.catalog == nil {
		return ErrUnavailable
	}
	database, owner, source, ok := duplicateTargetNames(op)
	if !ok {
		return ErrUnavailable
	}
	cctx, cancel := compensateContext(ctx)
	defer cancel()

	if database != source && !c.svc.policy.DatabaseDenied(database) {
		exists, err := c.svc.catalog.DatabaseExists(cctx, database)
		if err != nil {
			return ErrUnavailable
		}
		if exists {
			if err := c.svc.catalog.TerminateAndDropDatabase(cctx, database); err != nil {
				return ErrUnavailable
			}
		}
	}

	roleExists, err := c.svc.catalog.RoleExists(cctx, owner)
	if err != nil {
		return ErrUnavailable
	}
	if !roleExists {
		return nil
	}
	n, err := c.svc.catalog.OwnedDatabaseCount(cctx, owner)
	if err != nil {
		return ErrUnavailable
	}
	if n > 0 {
		return nil
	}

	saved, err := c.svc.catalog.SavedRoleNames(cctx, []string{owner})
	if err != nil {
		return ErrUnavailable
	}
	if _, present := saved[owner]; present {
		if err := c.svc.catalog.DeleteCredential(cctx, owner); err != nil {
			return ErrUnavailable
		}
	}
	if c.svc.policy.OwnerDenied(owner) {
		return nil
	}
	if err := c.svc.catalog.DropRole(cctx, owner); err != nil {
		return ErrUnavailable
	}
	return nil
}

// RunQueuedDuplicates claims queued postgres.database.duplicate rows and runs TEMPLATE clone
// under operations.MaxRuntime. Parent ticks this after Open. It never retries interrupted work.
func RunQueuedDuplicates(ctx context.Context, store operations.Store, svc *Service, auditor DuplicateAuditor) error {
	if store == nil || svc == nil {
		return ErrUnavailable
	}
	queued, err := store.ListQueued(ctx)
	if err != nil {
		return err
	}
	compensator := NewDuplicateCompensator(svc)
	for _, op := range queued {
		if op.Action != operations.ActionDuplicate {
			continue
		}
		if err := runQueuedDuplicate(ctx, store, svc, compensator, auditor, op); err != nil {
			return err
		}
	}
	return nil
}

func runQueuedDuplicate(ctx context.Context, store operations.Store, svc *Service, compensator operations.Compensator, auditor DuplicateAuditor, op operations.Operation) error {
	if err := store.Transition(ctx, op.ID, operations.Transition{
		From:  operations.StatusQueued,
		To:    operations.StatusRunning,
		Phase: operations.PhaseCloning,
	}); err != nil {
		if errors.Is(err, operations.ErrIllegalEdge) {
			return nil
		}
		return err
	}

	workCtx, cancel := context.WithTimeout(ctx, operations.MaxRuntime)
	defer cancel()

	database, owner, source, ok := duplicateTargetNames(op)
	if !ok {
		return failQueuedDuplicate(ctx, store, compensator, op, ErrUnavailable)
	}

	created, err := svc.Duplicate(workCtx, source, database, owner)
	if err != nil {
		return failQueuedDuplicate(ctx, store, compensator, op, err)
	}

	result := operations.DuplicateResult{Database: created.Database, Owner: created.Owner, Source: source}
	if err := store.Transition(ctx, op.ID, operations.Transition{
		From:   operations.StatusRunning,
		To:     operations.StatusSucceeded,
		Phase:  operations.PhaseVaulting,
		Result: &result,
	}); err != nil {
		return err
	}
	if auditor == nil {
		return nil
	}
	_ = auditor.Record(op.Actor, "postgres.database.duplicate", created.Database, "success", op.AcceptedRequestID, "", map[string]any{
		"database":     created.Database,
		"owner":        created.Owner,
		"source":       source,
		"operation_id": op.ID,
	})
	return nil
}

func failQueuedDuplicate(ctx context.Context, store operations.Store, compensator operations.Compensator, op operations.Operation, cause error) error {
	fail := duplicateOperationError(cause)
	if err := store.Transition(ctx, op.ID, operations.Transition{
		From:  operations.StatusRunning,
		To:    operations.StatusCompensating,
		Phase: operations.PhaseCompensating,
	}); err != nil {
		return err
	}
	if compensator != nil && !skipDuplicateCompensation(cause) {
		if err := compensator.CompensateDuplicate(ctx, op); err != nil {
			return store.Transition(ctx, op.ID, operations.Transition{
				From:      operations.StatusCompensating,
				To:        operations.StatusIndeterminate,
				Phase:     operations.PhaseCompensating,
				KeepLocks: true,
				Error: &operations.OperationError{
					Code:    "compensation_incomplete",
					Message: "Compensation did not finish.",
				},
			})
		}
	}
	return store.Transition(ctx, op.ID, operations.Transition{
		From:  operations.StatusCompensating,
		To:    operations.StatusFailed,
		Phase: operations.PhaseCompensating,
		Error: fail,
	})
}

func skipDuplicateCompensation(err error) bool {
	var conflict Conflict
	var field FieldError
	var inProgress DuplicateInProgress
	if errors.As(err, &conflict) || errors.As(err, &field) || errors.As(err, &inProgress) {
		return true
	}
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrProtected)
}

func duplicateOperationError(err error) *operations.OperationError {
	var inProgress DuplicateInProgress
	if errors.As(err, &inProgress) {
		return &operations.OperationError{Code: "operation_in_progress", Message: duplicateInProgressMessage}
	}
	var isolation IsolationChanged
	if errors.As(err, &isolation) {
		return &operations.OperationError{Code: "dependency_unavailable", Message: isolationChangedMessage}
	}
	var field FieldError
	if errors.As(err, &field) {
		msg := field.Message
		if msg == "" {
			msg = "Invalid database name"
		}
		key := field.Field
		if key == "" {
			key = conflictFieldDatabase
		}
		return &operations.OperationError{Code: "validation_error", Message: msg, Fields: map[string]string{key: "invalid"}}
	}
	var conflict Conflict
	if errors.As(err, &conflict) {
		key := conflict.Field
		if key == "" {
			key = conflictFieldDatabase
		}
		return &operations.OperationError{Code: "conflict", Message: conflict.Error(), Fields: map[string]string{key: "exists"}}
	}
	switch {
	case errors.Is(err, ErrProtected):
		return &operations.OperationError{Code: "protected_resource", Message: "This PostgreSQL name is protected"}
	case errors.Is(err, ErrNotFound):
		return &operations.OperationError{Code: "not_found", Message: "Not found"}
	default:
		return &operations.OperationError{Code: "dependency_unavailable", Message: "PostgreSQL is unavailable"}
	}
}
