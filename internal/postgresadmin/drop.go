package postgresadmin

import (
	"context"
	"errors"
)

func (s *Service) SystemIdentifier(ctx context.Context) (string, error) {
	if s == nil || s.catalog == nil {
		return "", ErrUnavailable
	}
	return s.catalog.SystemIdentifier(ctx)
}

func formatOperatorDropDatabase(database string) (string, error) {
	quotedDB, err := QuoteIdentifier(database)
	if err != nil {
		return "", err
	}
	return "DROP DATABASE " + quotedDB, nil
}

func (s *Service) Drop(ctx context.Context, database string) (DropResult, error) {
	if err := ValidateIdentifier(database); err != nil {
		return DropResult{}, err
	}
	if s == nil || s.catalog == nil {
		return DropResult{}, ErrUnavailable
	}
	if err := s.lockDrop(database); err != nil {
		return DropResult{}, err
	}
	defer s.unlockDrop(database)
	if s.policy.DatabaseDenied(database) {
		return DropResult{}, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, database)
	if err != nil {
		return DropResult{}, mapCatalogError(err)
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
		return DropResult{}, ErrNotFound
	}
	if err := s.catalog.TerminateSessions(ctx, row.Name); err != nil {
		return DropResult{}, dropExecError(err)
	}
	if err := s.catalog.DropDatabase(ctx, row.Name); err != nil {
		return DropResult{}, dropExecError(err)
	}
	result := DropResult{Dropped: row.Name, Owner: row.Owner}
	droppedRole, err := s.dropOwnerIfUnowned(ctx, row.Owner)
	if err != nil {
		return result, err
	}
	result.DroppedRole = droppedRole
	return result, nil
}

func dropExecError(err error) error {
	if errors.Is(err, ErrInvalidIdentifier) {
		return err
	}
	return ErrUnavailable
}

func (s *Service) dropOwnerIfUnowned(ctx context.Context, owner string) (string, error) {
	if owner == "" || s.policy.OwnerDenied(owner) {
		return "", nil
	}
	n, err := s.catalog.OwnedDatabaseCount(ctx, owner)
	if err != nil || n != 0 {
		return "", nil
	}
	if err := s.catalog.DropRole(ctx, owner); err != nil {
		return "", RoleDropFailed{}
	}
	if err := s.catalog.DeleteCredential(ctx, owner); err != nil {
		return "", VaultDeleteFailed{}
	}
	return owner, nil
}

func (s *Service) lockDrop(database string) error {
	// Lock order: truncateMu then dropMu.
	s.truncateMu.Lock()
	s.dropMu.Lock()
	defer s.dropMu.Unlock()
	defer s.truncateMu.Unlock()
	if _, held := s.truncating[database]; held {
		return TruncateInProgress{}
	}
	if s.dropping == nil {
		s.dropping = map[string]struct{}{}
	}
	if _, held := s.dropping[database]; held {
		return DropInProgress{}
	}
	s.dropping[database] = struct{}{}
	return nil
}

func (s *Service) unlockDrop(database string) {
	s.dropMu.Lock()
	defer s.dropMu.Unlock()
	delete(s.dropping, database)
}

func (c PoolCatalog) DropDatabase(ctx context.Context, database string) error {
	sql, err := formatOperatorDropDatabase(database)
	if err != nil {
		return err
	}
	return c.execSimple(ctx, sql)
}
