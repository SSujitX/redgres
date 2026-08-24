package postgresadmin

import (
	"context"
	"errors"
	"strings"
)

type Service struct {
	catalog Catalog
	policy  Policy
}

func NewService(catalog Catalog, policy Policy) *Service {
	return &Service{catalog: catalog, policy: policy}
}

func (s *Service) Ping(ctx context.Context) error {
	if s == nil || s.catalog == nil {
		return ErrNotConfigured
	}
	if err := s.catalog.Ping(ctx); err != nil {
		return mapCatalogError(err)
	}
	return nil
}

func (s *Service) List(ctx context.Context) (ListResult, error) {
	if s == nil || s.catalog == nil {
		return ListResult{}, ErrUnavailable
	}
	rows, err := s.catalog.List(ctx)
	if err != nil {
		return ListResult{}, mapCatalogError(err)
	}
	out := ListResult{Databases: make([]ListItem, 0, len(rows))}
	for _, row := range rows {
		if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
			continue
		}
		if len(out.Databases) >= listCap {
			out.Truncated = true
			break
		}
		out.Databases = append(out.Databases, ListItem{Name: row.Name, Owner: row.Owner})
	}
	return out, nil
}

func (s *Service) Search(ctx context.Context, q string, limit int) (SearchResult, error) {
	listed, err := s.List(ctx)
	if err != nil {
		return SearchResult{}, err
	}
	needle := strings.ToLower(q)
	out := SearchResult{Hits: make([]SearchHit, 0)}
	matched := 0
	for _, item := range listed.Databases {
		if needle == "" || !strings.Contains(strings.ToLower(item.Name), needle) {
			continue
		}
		matched++
		if limit > 0 && len(out.Hits) >= limit {
			continue
		}
		out.Hits = append(out.Hits, SearchHit{Name: item.Name})
	}
	if listed.Truncated || (limit > 0 && matched > limit) {
		out.Truncated = true
	}
	return out, nil
}

func (s *Service) Details(ctx context.Context, name string) (DatabaseDetails, error) {
	if err := ValidateIdentifier(name); err != nil {
		return DatabaseDetails{}, err
	}
	if s == nil || s.catalog == nil {
		return DatabaseDetails{}, ErrUnavailable
	}
	if s.policy.DatabaseDenied(name) {
		return DatabaseDetails{}, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, name)
	if err != nil {
		return DatabaseDetails{}, mapCatalogError(err)
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
		return DatabaseDetails{}, ErrNotFound
	}
	return DatabaseDetails{
		Name:            row.Name,
		Owner:           row.Owner,
		Size:            row.SizePretty,
		SizeBytes:       row.SizeBytes,
		Collation:       row.Collation,
		CType:           row.CType,
		LocaleProvider:  row.LocaleProvider,
		Locale:          row.Locale,
		ConnectionCount: row.ConnectionCount,
		Security: SecurityStatus{
			PublicCanConnect: row.PublicCanConnect,
			OwnerIsSuperuser: row.OwnerIsSuperuser,
			OwnerCanLogin:    row.OwnerCanLogin,
			OwnerCreatedb:    row.OwnerCreatedb,
			OwnerCreaterole:  row.OwnerCreaterole,
			OwnerReplication: row.OwnerReplication,
		},
		SavedCredential: vaultNotAvailable(),
	}, nil
}

func (s *Service) Tables(ctx context.Context, name string) (TableListResult, error) {
	if err := ValidateIdentifier(name); err != nil {
		return TableListResult{}, err
	}
	if s == nil || s.catalog == nil {
		return TableListResult{}, ErrUnavailable
	}
	if s.policy.DatabaseDenied(name) {
		return TableListResult{}, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, name)
	if err != nil {
		return TableListResult{}, mapCatalogError(err)
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
		return TableListResult{}, ErrNotFound
	}
	items, err := s.catalog.ListTables(ctx, row.Name)
	if err != nil {
		return TableListResult{}, mapCatalogError(err)
	}
	out := TableListResult{Tables: make([]TableItem, 0, len(items))}
	for _, item := range items {
		if len(out.Tables) >= listCap {
			out.Truncated = true
			break
		}
		out.Tables = append(out.Tables, item)
	}
	return out, nil
}

func (s *Service) Rows(ctx context.Context, database, schema, table, q string, offset, limit int) (RowPage, error) {
	if err := ValidateIdentifier(database); err != nil {
		return RowPage{}, err
	}
	if err := ValidateIdentifier(schema); err != nil {
		return RowPage{}, err
	}
	if err := ValidateIdentifier(table); err != nil {
		return RowPage{}, err
	}
	if catalogSchemaDenied(schema) {
		return RowPage{}, ErrNotFound
	}
	if s == nil || s.catalog == nil {
		return RowPage{}, ErrUnavailable
	}
	if s.policy.DatabaseDenied(database) {
		return RowPage{}, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, database)
	if err != nil {
		return RowPage{}, mapCatalogError(err)
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
		return RowPage{}, ErrNotFound
	}
	limit, offset = clampRowPage(limit, offset)
	page, err := s.catalog.ListRows(ctx, row.Name, schema, table, q, offset, limit)
	if err != nil {
		return RowPage{}, mapCatalogError(err)
	}
	page.Offset = offset
	page.Limit = limit
	if page.Columns == nil {
		page.Columns = []string{}
	}
	if page.Rows == nil {
		page.Rows = []map[string]any{}
	}
	return page, nil
}

func clampRowPage(limit, offset int) (int, int) {
	if limit <= 0 || limit > listCap {
		limit = defaultRowLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func catalogSchemaDenied(schema string) bool {
	return schema == "pg_catalog" || schema == "information_schema"
}

func mapCatalogError(err error) error {
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidIdentifier) || errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotConfigured) {
		return err
	}
	return ErrUnavailable
}
