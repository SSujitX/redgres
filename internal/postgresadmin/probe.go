package postgresadmin

import (
	"context"

	"github.com/SSujitX/redgres/internal/operations"
)

type duplicateProbe struct {
	svc *Service
}

// NewDuplicateProbe inspects clone, role, and vault-row existence without decrypting.
func NewDuplicateProbe(svc *Service) operations.Probe {
	return duplicateProbe{svc: svc}
}

func (p duplicateProbe) DuplicateState(ctx context.Context, op operations.Operation) (operations.ProbeOutcome, error) {
	if p.svc == nil || p.svc.catalog == nil {
		return operations.ProbeOutcome{Indeterminate: true}, nil
	}
	database, owner, _, ok := duplicateTargetNames(op)
	if !ok {
		return operations.ProbeOutcome{Indeterminate: true}, nil
	}
	clone, err := p.svc.catalog.DatabaseExists(ctx, database)
	if err != nil {
		return operations.ProbeOutcome{}, ErrUnavailable
	}
	role, err := p.svc.catalog.RoleExists(ctx, owner)
	if err != nil {
		return operations.ProbeOutcome{}, ErrUnavailable
	}
	saved, err := p.svc.catalog.SavedRoleNames(ctx, []string{owner})
	if err != nil {
		return operations.ProbeOutcome{}, ErrUnavailable
	}
	_, vault := saved[owner]
	return operations.ProbeOutcome{
		CloneExists:    clone,
		RoleExists:     role,
		VaultRowExists: vault,
	}, nil
}

func duplicateTargetNames(op operations.Operation) (database, owner, source string, ok bool) {
	if op.Result != nil {
		database = op.Result.Database
		owner = op.Result.Owner
		source = op.Result.Source
	}
	if database == "" {
		database = op.Target
	}
	if database == "" || owner == "" {
		return "", "", "", false
	}
	return database, owner, source, true
}
