package postgresadmin

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Truncate(ctx context.Context, database string) (TruncateResult, error) {
	empty := TruncateResult{Failed: []string{}}
	if err := ValidateIdentifier(database); err != nil {
		return empty, err
	}
	if s == nil || s.catalog == nil {
		return empty, ErrUnavailable
	}
	if !s.tryLockTruncate(database) {
		return empty, TruncateInProgress{}
	}
	defer s.unlockTruncate(database)
	if s.policy.DatabaseDenied(database) {
		return empty, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, database)
	if err != nil {
		return empty, mapCatalogError(err)
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
		return empty, ErrNotFound
	}
	items, err := s.catalog.ListTables(ctx, row.Name)
	if err != nil {
		return empty, mapCatalogError(err)
	}
	if len(items) > listCap {
		return empty, TableListTruncated{}
	}
	if len(items) == 0 {
		return TruncateResult{Truncated: 0, Failed: []string{}, TotalTables: 0}, nil
	}
	if _, err := formatTruncateSQL(items); err != nil {
		return empty, mapCatalogError(err)
	}
	if err := s.catalog.Truncate(ctx, row.Name, items); err != nil {
		return empty, mapCatalogError(err)
	}
	n := len(items)
	return TruncateResult{Truncated: n, Failed: []string{}, TotalTables: n}, nil
}

func (s *Service) tryLockTruncate(database string) bool {
	s.truncateMu.Lock()
	defer s.truncateMu.Unlock()
	if s.truncating == nil {
		s.truncating = map[string]struct{}{}
	}
	if _, held := s.truncating[database]; held {
		return false
	}
	s.truncating[database] = struct{}{}
	return true
}

func (s *Service) unlockTruncate(database string) {
	s.truncateMu.Lock()
	defer s.truncateMu.Unlock()
	delete(s.truncating, database)
}

func (c PoolCatalog) Truncate(ctx context.Context, database string, tables []TableItem) error {
	if err := ValidateIdentifier(database); err != nil {
		return err
	}
	sql, err := formatTruncateSQL(tables)
	if err != nil {
		return err
	}
	conn, connectCtx, closeConn, err := c.connectTarget(ctx, database)
	if err != nil {
		return err
	}
	defer closeConn()
	if _, err := conn.Exec(connectCtx, sql); err != nil {
		return ErrUnavailable
	}
	return nil
}

func formatTruncateSQL(tables []TableItem) (string, error) {
	if len(tables) == 0 {
		return "", ErrUnavailable
	}
	parts := make([]string, 0, len(tables))
	for _, item := range tables {
		if _, err := QuoteCatalogIdentifier(item.Schema); err != nil {
			return "", ErrUnavailable
		}
		if _, err := QuoteCatalogIdentifier(item.Name); err != nil {
			return "", ErrUnavailable
		}
		parts = append(parts, pgx.Identifier{item.Schema, item.Name}.Sanitize())
	}
	return "TRUNCATE TABLE " + strings.Join(parts, ", ") + " RESTART IDENTITY", nil
}
