package postgresadmin

import (
	"context"
	"errors"
)

type Service struct {
	catalog Catalog
	policy  Policy
}

func NewService(catalog Catalog, policy Policy) *Service {
	return &Service{catalog: catalog, policy: policy}
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

func mapCatalogError(err error) error {
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidIdentifier) || errors.Is(err, ErrUnavailable) {
		return err
	}
	return ErrUnavailable
}
