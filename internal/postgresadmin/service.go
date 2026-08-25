package postgresadmin

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/SSujitX/redgres/internal/secrets"
)

type Service struct {
	catalog     Catalog
	policy      Policy
	vaultKey    string
	rotateMu    sync.Mutex
	rotating    map[string]struct{}
	duplicateMu sync.Mutex
	duplicating map[string]struct{}
	truncateMu  sync.Mutex
	truncating  map[string]struct{}
}

func NewService(catalog Catalog, policy Policy) *Service {
	return NewServiceWithVaultKey(catalog, policy, "")
}

func NewServiceWithVaultKey(catalog Catalog, policy Policy, vaultKey string) *Service {
	return &Service{catalog: catalog, policy: policy, vaultKey: vaultKey}
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

func (s *Service) PingPooled(ctx context.Context) error {
	if s == nil || s.catalog == nil {
		return ErrNotConfigured
	}
	if err := s.catalog.PingPooled(ctx); err != nil {
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
		SavedCredential: s.savedCredential(ctx, row.Owner),
	}, nil
}

func (s *Service) Connection(ctx context.Context, name string) (Connection, error) {
	if err := ValidateIdentifier(name); err != nil {
		return Connection{}, err
	}
	if s == nil || s.catalog == nil {
		return Connection{}, ErrUnavailable
	}
	if s.policy.DatabaseDenied(name) {
		return Connection{}, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, name)
	if err != nil {
		return Connection{}, mapCatalogError(err)
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
		return Connection{}, ErrNotFound
	}
	return Connection{
		Database:        row.Name,
		Owner:           row.Owner,
		SavedCredential: s.savedCredential(ctx, row.Owner),
	}, nil
}

func (s *Service) Reveal(ctx context.Context, name string) (RevealedConnection, error) {
	if err := ValidateIdentifier(name); err != nil {
		return RevealedConnection{}, err
	}
	if s == nil || s.catalog == nil {
		return RevealedConnection{}, ErrUnavailable
	}
	if s.policy.DatabaseDenied(name) {
		return RevealedConnection{}, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, name)
	if err != nil {
		return RevealedConnection{}, mapCatalogError(err)
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
		return RevealedConnection{}, ErrNotFound
	}
	if row.Owner == "" {
		return RevealedConnection{}, ErrNotFound
	}
	token, err := s.catalog.EncryptedPassword(ctx, row.Owner)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RevealedConnection{}, ErrNotFound
		}
		return RevealedConnection{}, ErrUnavailable
	}
	if s.vaultKey == "" {
		return RevealedConnection{}, ErrUnavailable
	}
	plain, err := secrets.Decrypt(s.vaultKey, token)
	if err != nil || len(plain) == 0 {
		return RevealedConnection{}, ErrUnavailable
	}
	return RevealedConnection{Database: row.Name, Owner: row.Owner, Password: string(plain)}, nil
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

func (s *Service) PrimaryKey(ctx context.Context, database, schema, table string) ([]string, error) {
	if err := ValidateIdentifier(database); err != nil {
		return nil, err
	}
	if err := ValidateIdentifier(schema); err != nil {
		return nil, err
	}
	if err := ValidateIdentifier(table); err != nil {
		return nil, err
	}
	if catalogSchemaDenied(schema) {
		return nil, ErrNotFound
	}
	if s == nil || s.catalog == nil {
		return nil, ErrUnavailable
	}
	if s.policy.DatabaseDenied(database) {
		return nil, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, database)
	if err != nil {
		return nil, mapCatalogError(err)
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
		return nil, ErrNotFound
	}
	cols, err := s.catalog.PrimaryKey(ctx, row.Name, schema, table)
	if err != nil {
		return nil, mapCatalogError(err)
	}
	if cols == nil {
		cols = []string{}
	}
	return cols, nil
}

func (s *Service) DeleteRows(ctx context.Context, database, schema, table string, values []any) (int64, error) {
	if err := ValidateIdentifier(database); err != nil {
		return 0, err
	}
	if err := ValidateIdentifier(schema); err != nil {
		return 0, err
	}
	if err := ValidateIdentifier(table); err != nil {
		return 0, err
	}
	if catalogSchemaDenied(schema) {
		return 0, ErrNotFound
	}
	if s == nil || s.catalog == nil {
		return 0, ErrUnavailable
	}
	if s.policy.DatabaseDenied(database) {
		return 0, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, database)
	if err != nil {
		return 0, mapCatalogError(err)
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) {
		return 0, ErrNotFound
	}
	cols, err := s.catalog.PrimaryKey(ctx, row.Name, schema, table)
	if err != nil {
		return 0, mapCatalogError(err)
	}
	if len(cols) != 1 {
		return 0, FieldError{Field: "primary_key", Message: missingSingleColumnPKMessage}
	}
	deleted, err := s.catalog.DeleteRows(ctx, row.Name, schema, table, cols[0], values)
	if err != nil {
		return 0, mapCatalogError(err)
	}
	return deleted, nil
}

func (s *Service) SecurityOverview(ctx context.Context) (SecurityOverview, error) {
	if s == nil || s.catalog == nil {
		return SecurityOverview{}, ErrUnavailable
	}
	rows, err := s.catalog.List(ctx)
	if err != nil {
		return SecurityOverview{}, mapCatalogError(err)
	}
	groups, err := s.catalog.ListConnectionGroups(ctx)
	if err != nil {
		return SecurityOverview{}, mapCatalogError(err)
	}
	databases := make([]SecurityDatabase, 0, len(rows))
	publicConnect := 0
	for _, row := range rows {
		if row.IsTemplate {
			continue
		}
		if row.PublicCanConnect {
			publicConnect++
		}
		protected := !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate)
		databases = append(databases, SecurityDatabase{
			Name:              row.Name,
			Owner:             row.Owner,
			Protected:         protected,
			PublicCanConnect:  row.PublicCanConnect,
			OwnerIsSuperuser:  row.OwnerIsSuperuser,
			OwnerCanLogin:     row.OwnerCanLogin,
			OwnerCreatedb:     row.OwnerCreatedb,
			OwnerCreaterole:   row.OwnerCreaterole,
			OwnerReplication:  row.OwnerReplication,
			ActiveConnections: row.ConnectionCount,
			RotationEligible:  rotationEligible(row.Owner, protected, row.OwnerCanLogin, row.OwnerIsSuperuser),
		})
	}
	sort.Slice(databases, func(i, j int) bool { return databases[i].Name < databases[j].Name })

	connections := make([]ConnectionGroup, 0, len(groups))
	active := 0
	for _, group := range groups {
		group = displayConnectionGroup(group)
		active += group.Count
		connections = append(connections, group)
	}
	sort.Slice(connections, func(i, j int) bool {
		a, b := connections[i], connections[j]
		if a.Database != b.Database {
			return a.Database < b.Database
		}
		if a.User != b.User {
			return a.User < b.User
		}
		if a.Client != b.Client {
			return a.Client < b.Client
		}
		if a.Application != b.Application {
			return a.Application < b.Application
		}
		return a.State < b.State
	})

	out := SecurityOverview{
		Summary: SecuritySummary{
			DatabaseCount:         len(databases),
			PublicConnectCount:    publicConnect,
			ActiveConnectionCount: active,
			ConnectionGroupCount:  len(connections),
		},
		Databases:   databases,
		Connections: connections,
		Truncated:   len(databases) > listCap || len(connections) > listCap,
	}
	s.applyVaultExistence(ctx, &out)
	if len(out.Databases) > listCap {
		out.Databases = out.Databases[:listCap]
	}
	if len(out.Connections) > listCap {
		out.Connections = out.Connections[:listCap]
	}
	return out, nil
}

func (s *Service) savedCredential(ctx context.Context, owner string) SavedCredential {
	names, err := s.catalog.SavedRoleNames(ctx, []string{owner})
	if err != nil {
		return vaultUnavailable()
	}
	if _, ok := names[owner]; ok {
		return SavedCredential{Status: "present", Reason: ""}
	}
	return SavedCredential{Status: "missing", Reason: ""}
}

func (s *Service) applyVaultExistence(ctx context.Context, out *SecurityOverview) {
	owners := make([]string, 0, len(out.Databases))
	for _, db := range out.Databases {
		owners = append(owners, db.Owner)
	}
	names, err := s.catalog.SavedRoleNames(ctx, uniqueRoleNames(owners, listCap))
	if err != nil {
		out.SavedCredential = vaultUnavailable()
		return
	}
	out.SavedCredential = SavedCredential{Status: "ok", Reason: ""}
	missing := 0
	for _, db := range out.Databases {
		if _, ok := names[db.Owner]; !ok {
			missing++
		}
	}
	out.Summary.MissingPasswordCount = &missing
}

func displayConnectionGroup(group ConnectionGroup) ConnectionGroup {
	if group.Database == "" {
		group.Database = "(none)"
	}
	if group.User == "" {
		group.User = "(unknown)"
	}
	if group.Client == "" {
		group.Client = "local"
	}
	if group.Application == "" {
		group.Application = "—"
	}
	if group.State == "" {
		group.State = "unknown"
	}
	return group
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
	var field FieldError
	if errors.As(err, &field) {
		return err
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidIdentifier) || errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotConfigured) || errors.Is(err, ErrVaultUnavailable) || errors.Is(err, ErrProtected) || errors.Is(err, ErrConflict) || errors.Is(err, ErrOperationInProgress) || errors.Is(err, ErrVaultUnsynced) {
		return err
	}
	return ErrUnavailable
}
