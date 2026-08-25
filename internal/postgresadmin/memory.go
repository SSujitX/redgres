package postgresadmin

import "context"

type MemoryTable struct {
	Columns []string
	Rows    []map[string]any
}

type MemoryCatalog struct {
	Rows                          []CatalogRow
	Err                           error
	PingErr                       error
	Tables                        map[string][]TableItem
	TablesErr                     error
	LastTablesDB                  string
	TableData                     map[string]MemoryTable
	RowsErr                       error
	LastRowsKey                   string
	Connections                   []ConnectionGroup
	ConnectionsErr                error
	SavedRoles                    []string
	VaultErr                      error
	Ciphertexts                   map[string]string
	CiphertextErr                 error
	EncryptedPasswordCalls        []string
	PooledConfigured              bool
	PingPooledErr                 error
	ExistingRoles                 []string
	ExistsErr                     error
	CreateRoleErr                 error
	CreateDatabaseErr             error
	GrantSetRoleErr               error
	LockConnectErr                error
	InsertCredentialErr           error
	OwnedCount                    int
	OwnedCountErr                 error
	LastCreateRoleSQL             string
	LastGrantSQL                  string
	CreateRoleCalls               int
	CreateDatabaseCalls           int
	GrantCalls                    int
	LockConnectCalls              int
	InsertCalls                   int
	AlterRolePasswordCalls        int
	LastAlterRoleSQL              string
	AlterRolePasswordErr          error
	AlterStarted                  chan struct{}
	AlterHold                     chan struct{}
	UpsertCalls                   int
	LastUpsertRole                string
	LastUpsertToken               string
	UpsertCredentialErr           error
	UpsertFailTimes               int
	DeleteCredentialCalls         int
	DropDatabaseCalls             int
	DropRoleCalls                 int
	CreatedRoles                  []string
	CreatedDatabases              []string
	InsertedVault                 []string
	DroppedRoles                  []string
	DroppedDatabases              []string
	DeletedVault                  []string
	SnapshotSeq                   []OwnershipSnapshot
	snapshotN                     int
	SnapshotErr                   error
	SnapshotCalls                 int
	LastSnapshotName              string
	TerminateCalls                int
	LastTerminateName             string
	TerminatedDatabases           []string
	TerminateErr                  error
	CreateDatabaseTemplateCalls   int
	LastCreateDatabaseTemplateSQL string
	CreateDatabaseTemplateErr     error
	TemplateStarted               chan struct{}
	TemplateHold                  chan struct{}
	TransferCalls                 int
	LastTransferDB                string
	LastTransferOwner             string
	TransferErr                   error
	SkippedTransferOwners         []string
}

func (m *MemoryCatalog) Ping(context.Context) error {
	if m.PingErr != nil {
		return m.PingErr
	}
	return nil
}

func (m *MemoryCatalog) PingPooled(context.Context) error {
	if !m.PooledConfigured {
		return ErrNotConfigured
	}
	if m.PingPooledErr != nil {
		return m.PingPooledErr
	}
	return nil
}

func (m *MemoryCatalog) List(context.Context) ([]CatalogRow, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([]CatalogRow, len(m.Rows))
	copy(out, m.Rows)
	return out, nil
}

func (m *MemoryCatalog) Lookup(_ context.Context, name string) (CatalogRow, error) {
	if m.Err != nil {
		return CatalogRow{}, m.Err
	}
	for _, row := range m.Rows {
		if row.Name == name {
			return row, nil
		}
	}
	return CatalogRow{}, ErrNotFound
}

func (m *MemoryCatalog) ListTables(_ context.Context, database string) ([]TableItem, error) {
	if err := ValidateIdentifier(database); err != nil {
		return nil, err
	}
	m.LastTablesDB = database
	if m.TablesErr != nil {
		return nil, m.TablesErr
	}
	if m.Err != nil {
		return nil, m.Err
	}
	items := m.Tables[database]
	out := make([]TableItem, len(items))
	copy(out, items)
	return out, nil
}

func memoryTableKey(database, schema, table string) string {
	return database + "." + schema + "." + table
}

func (m *MemoryCatalog) ListRows(_ context.Context, database, schema, table, _ string, offset, limit int) (RowPage, error) {
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
	key := memoryTableKey(database, schema, table)
	m.LastRowsKey = key
	if m.RowsErr != nil {
		return RowPage{}, m.RowsErr
	}
	if m.Err != nil {
		return RowPage{}, m.Err
	}
	data, ok := m.TableData[key]
	if !ok || len(data.Columns) == 0 {
		return RowPage{}, ErrNotFound
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultRowLimit
	}
	total := int64(len(data.Rows))
	if offset > len(data.Rows) {
		return RowPage{Columns: append([]string{}, data.Columns...), Rows: []map[string]any{}, Total: total, Offset: offset, Limit: limit}, nil
	}
	end := offset + limit
	if end > len(data.Rows) {
		end = len(data.Rows)
	}
	page := data.Rows[offset:end]
	out := make([]map[string]any, len(page))
	copy(out, page)
	return RowPage{
		Columns: append([]string{}, data.Columns...),
		Rows:    out,
		Total:   total,
		Offset:  offset,
		Limit:   limit,
	}, nil
}

func (m *MemoryCatalog) ListConnectionGroups(context.Context) ([]ConnectionGroup, error) {
	if m.ConnectionsErr != nil {
		return nil, m.ConnectionsErr
	}
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([]ConnectionGroup, len(m.Connections))
	copy(out, m.Connections)
	return out, nil
}

func (m *MemoryCatalog) EncryptedPassword(_ context.Context, role string) (string, error) {
	m.EncryptedPasswordCalls = append(m.EncryptedPasswordCalls, role)
	if m.CiphertextErr != nil {
		return "", m.CiphertextErr
	}
	token, ok := m.Ciphertexts[role]
	if !ok {
		return "", ErrNotFound
	}
	return token, nil
}

func (m *MemoryCatalog) SavedRoleNames(_ context.Context, roles []string) (map[string]struct{}, error) {
	if len(roles) == 0 {
		return map[string]struct{}{}, nil
	}
	if m.VaultErr != nil {
		return nil, m.VaultErr
	}
	want := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		want[role] = struct{}{}
	}
	out := make(map[string]struct{})
	for _, role := range m.SavedRoles {
		if _, ok := want[role]; ok {
			out[role] = struct{}{}
		}
	}
	return out, nil
}

func (m *MemoryCatalog) DatabaseExists(_ context.Context, name string) (bool, error) {
	if m.ExistsErr != nil {
		return false, m.ExistsErr
	}
	if m.Err != nil {
		return false, m.Err
	}
	for _, row := range m.Rows {
		if row.Name == name {
			return true, nil
		}
	}
	for _, created := range m.CreatedDatabases {
		if created == name {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryCatalog) RoleExists(_ context.Context, name string) (bool, error) {
	if m.ExistsErr != nil {
		return false, m.ExistsErr
	}
	if m.Err != nil {
		return false, m.Err
	}
	for _, role := range m.ExistingRoles {
		if role == name {
			return true, nil
		}
	}
	for _, role := range m.CreatedRoles {
		if role == name {
			return true, nil
		}
	}
	for _, row := range m.Rows {
		if row.Owner == name {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryCatalog) CreateRole(_ context.Context, owner, password string) error {
	sql, err := formatCreateRole(owner, password)
	if err != nil {
		return err
	}
	m.LastCreateRoleSQL = sql
	m.CreateRoleCalls++
	if m.CreateRoleErr != nil {
		return m.CreateRoleErr
	}
	m.CreatedRoles = append(m.CreatedRoles, owner)
	return nil
}

func (m *MemoryCatalog) GrantSetRole(_ context.Context, owner, admin string) error {
	sql, err := formatGrantSetRole(owner, admin)
	if err != nil {
		return err
	}
	m.LastGrantSQL = sql
	m.GrantCalls++
	if m.GrantSetRoleErr != nil {
		return m.GrantSetRoleErr
	}
	return nil
}

func (m *MemoryCatalog) CreateDatabase(_ context.Context, database, owner string) error {
	m.CreateDatabaseCalls++
	if m.CreateDatabaseErr != nil {
		return m.CreateDatabaseErr
	}
	m.CreatedDatabases = append(m.CreatedDatabases, database)
	m.Rows = append(m.Rows, CatalogRow{Name: database, Owner: owner, AllowConn: true})
	return nil
}

func (m *MemoryCatalog) LockConnect(context.Context, string, string) error {
	m.LockConnectCalls++
	if m.LockConnectErr != nil {
		return m.LockConnectErr
	}
	return nil
}

func (m *MemoryCatalog) InsertCredential(_ context.Context, role, encrypted string) error {
	m.InsertCalls++
	if m.InsertCredentialErr != nil {
		return m.InsertCredentialErr
	}
	m.InsertedVault = append(m.InsertedVault, role)
	m.SavedRoles = append(m.SavedRoles, role)
	if m.Ciphertexts == nil {
		m.Ciphertexts = map[string]string{}
	}
	m.Ciphertexts[role] = encrypted
	return nil
}

func (m *MemoryCatalog) AlterRolePassword(_ context.Context, owner, password string) error {
	sql, err := formatAlterRolePassword(owner, password)
	if err != nil {
		return err
	}
	m.LastAlterRoleSQL = sql
	m.AlterRolePasswordCalls++
	if m.AlterStarted != nil {
		select {
		case m.AlterStarted <- struct{}{}:
		default:
		}
	}
	if m.AlterHold != nil {
		<-m.AlterHold
	}
	if m.AlterRolePasswordErr != nil {
		return m.AlterRolePasswordErr
	}
	return nil
}

func (m *MemoryCatalog) UpsertCredential(_ context.Context, role, encrypted string) error {
	m.UpsertCalls++
	m.LastUpsertRole = role
	m.LastUpsertToken = encrypted
	if m.UpsertCalls <= m.UpsertFailTimes {
		return ErrUnavailable
	}
	if m.UpsertCredentialErr != nil {
		return m.UpsertCredentialErr
	}
	m.InsertedVault = append(m.InsertedVault, role)
	m.SavedRoles = append(m.SavedRoles, role)
	if m.Ciphertexts == nil {
		m.Ciphertexts = map[string]string{}
	}
	m.Ciphertexts[role] = encrypted
	return nil
}

func (m *MemoryCatalog) DeleteCredential(_ context.Context, role string) error {
	m.DeleteCredentialCalls++
	m.DeletedVault = append(m.DeletedVault, role)
	return nil
}

func (m *MemoryCatalog) TerminateAndDropDatabase(_ context.Context, database string) error {
	m.DropDatabaseCalls++
	m.DroppedDatabases = append(m.DroppedDatabases, database)
	filtered := m.Rows[:0]
	for _, row := range m.Rows {
		if row.Name != database {
			filtered = append(filtered, row)
		}
	}
	m.Rows = filtered
	return nil
}

func (m *MemoryCatalog) OwnedDatabaseCount(context.Context, string) (int, error) {
	if m.OwnedCountErr != nil {
		return 0, m.OwnedCountErr
	}
	if len(m.CreatedDatabases) > 0 && m.DropDatabaseCalls == 0 {
		return len(m.CreatedDatabases), nil
	}
	return m.OwnedCount, nil
}

func (m *MemoryCatalog) DropRole(_ context.Context, owner string) error {
	m.DropRoleCalls++
	m.DroppedRoles = append(m.DroppedRoles, owner)
	return nil
}

func (m *MemoryCatalog) OwnershipSnapshot(_ context.Context, name string) (OwnershipSnapshot, error) {
	m.SnapshotCalls++
	m.LastSnapshotName = name
	if m.SnapshotErr != nil {
		return OwnershipSnapshot{}, m.SnapshotErr
	}
	if len(m.SnapshotSeq) > 0 {
		i := m.snapshotN
		if i >= len(m.SnapshotSeq) {
			i = len(m.SnapshotSeq) - 1
		}
		m.snapshotN++
		return m.SnapshotSeq[i], nil
	}
	row, err := m.Lookup(context.Background(), name)
	if err != nil {
		return OwnershipSnapshot{}, err
	}
	return OwnershipSnapshot{Owner: row.Owner}, nil
}

func (m *MemoryCatalog) TerminateSessions(_ context.Context, database string) error {
	m.TerminateCalls++
	m.LastTerminateName = database
	m.TerminatedDatabases = append(m.TerminatedDatabases, database)
	if m.TerminateErr != nil {
		return m.TerminateErr
	}
	return nil
}

func (m *MemoryCatalog) CreateDatabaseTemplate(_ context.Context, database, source, owner string) error {
	sql, err := formatCreateDatabaseTemplate(database, source, owner)
	if err != nil {
		return err
	}
	m.LastCreateDatabaseTemplateSQL = sql
	m.CreateDatabaseTemplateCalls++
	if m.TemplateStarted != nil {
		select {
		case m.TemplateStarted <- struct{}{}:
		default:
		}
	}
	if m.TemplateHold != nil {
		<-m.TemplateHold
	}
	if m.CreateDatabaseTemplateErr != nil {
		return m.CreateDatabaseTemplateErr
	}
	m.CreatedDatabases = append(m.CreatedDatabases, database)
	m.Rows = append(m.Rows, CatalogRow{Name: database, Owner: owner, AllowConn: true, OwnerCanLogin: true})
	return nil
}

func (m *MemoryCatalog) TransferCloneOwnership(_ context.Context, database, newOwner, _ string, skipOwner func(string) bool) error {
	m.TransferCalls++
	m.LastTransferDB = database
	m.LastTransferOwner = newOwner
	if skipOwner != nil {
		for _, row := range m.Rows {
			if row.Name == database {
				continue
			}
			if skipOwner(row.Owner) {
				m.SkippedTransferOwners = append(m.SkippedTransferOwners, row.Owner)
			}
		}
	}
	if m.TransferErr != nil {
		return m.TransferErr
	}
	return nil
}
