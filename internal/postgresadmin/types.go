package postgresadmin

import "context"

const listCap = 500

type CatalogRow struct {
	Name             string
	Owner            string
	SizeBytes        int64
	SizePretty       string
	Collation        string
	CType            string
	LocaleProvider   string
	Locale           *string
	AllowConn        bool
	IsTemplate       bool
	ConnectionCount  int
	PublicCanConnect bool
	OwnerIsSuperuser bool
	OwnerCanLogin    bool
	OwnerCreatedb    bool
	OwnerCreaterole  bool
	OwnerReplication bool
}

type Catalog interface {
	List(ctx context.Context) ([]CatalogRow, error)
	Lookup(ctx context.Context, name string) (CatalogRow, error)
	ListTables(ctx context.Context, database string) ([]TableItem, error)
	ListRows(ctx context.Context, database, schema, table, q string, offset, limit int) (RowPage, error)
	PrimaryKey(ctx context.Context, database, schema, table string) ([]string, error)
	DeleteRows(ctx context.Context, database, schema, table, pkColumn string, values []any) (int64, error)
	ListConnectionGroups(ctx context.Context) ([]ConnectionGroup, error)
	SavedRoleNames(ctx context.Context, roles []string) (map[string]struct{}, error)
	EncryptedPassword(ctx context.Context, role string) (string, error)
	DatabaseExists(ctx context.Context, name string) (bool, error)
	RoleExists(ctx context.Context, name string) (bool, error)
	CreateRole(ctx context.Context, owner, password string) error
	GrantSetRole(ctx context.Context, owner, admin string) error
	CreateDatabase(ctx context.Context, database, owner string) error
	LockConnect(ctx context.Context, database, owner string) error
	InsertCredential(ctx context.Context, role, encrypted string) error
	AlterRolePassword(ctx context.Context, owner, password string) error
	UpsertCredential(ctx context.Context, role, encrypted string) error
	DeleteCredential(ctx context.Context, role string) error
	TerminateAndDropDatabase(ctx context.Context, database string) error
	OwnedDatabaseCount(ctx context.Context, owner string) (int, error)
	DropRole(ctx context.Context, owner string) error
	OwnershipSnapshot(ctx context.Context, database string) (OwnershipSnapshot, error)
	TerminateSessions(ctx context.Context, database string) error
	CreateDatabaseTemplate(ctx context.Context, database, source, owner string) error
	TransferCloneOwnership(ctx context.Context, database, newOwner, admin string, skipOwner func(string) bool) error
	Ping(ctx context.Context) error
	PingPooled(ctx context.Context) error
}

type OwnershipSnapshot struct {
	Owner  string
	Datacl string
}

type Inventory interface {
	List(ctx context.Context) (ListResult, error)
	Search(ctx context.Context, q string, limit int) (SearchResult, error)
	Details(ctx context.Context, name string) (DatabaseDetails, error)
	Tables(ctx context.Context, name string) (TableListResult, error)
	Rows(ctx context.Context, database, schema, table, q string, offset, limit int) (RowPage, error)
	PrimaryKey(ctx context.Context, database, schema, table string) ([]string, error)
	DeleteRows(ctx context.Context, database, schema, table string, values []any) (int64, error)
	SecurityOverview(ctx context.Context) (SecurityOverview, error)
	Connection(ctx context.Context, name string) (Connection, error)
	Reveal(ctx context.Context, name string) (RevealedConnection, error)
	Create(ctx context.Context, database, owner string) (CreatedDatabase, error)
	Rotate(ctx context.Context, name string) (CreatedDatabase, error)
	Duplicate(ctx context.Context, source, database, owner string) (CreatedDatabase, error)
	Ping(ctx context.Context) error
	PingPooled(ctx context.Context) error
}

type CreatedDatabase struct {
	Database string
	Owner    string
	Password string
}

type SearchHit struct {
	Name string
}

type SearchResult struct {
	Hits      []SearchHit
	Truncated bool
}

type ListItem struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

type ListResult struct {
	Databases []ListItem `json:"databases"`
	Truncated bool       `json:"truncated"`
}

type SecurityStatus struct {
	PublicCanConnect bool `json:"public_can_connect"`
	OwnerIsSuperuser bool `json:"owner_is_superuser"`
	OwnerCanLogin    bool `json:"owner_can_login"`
	OwnerCreatedb    bool `json:"owner_createdb"`
	OwnerCreaterole  bool `json:"owner_createrole"`
	OwnerReplication bool `json:"owner_replication"`
}

type SavedCredential struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type Connection struct {
	Database        string          `json:"database"`
	Owner           string          `json:"owner"`
	SavedCredential SavedCredential `json:"saved_credential"`
}

type RevealedConnection struct {
	Database string
	Owner    string
	Password string
}

type DatabaseDetails struct {
	Name            string          `json:"name"`
	Owner           string          `json:"owner"`
	Size            string          `json:"size"`
	SizeBytes       int64           `json:"size_bytes"`
	Collation       string          `json:"collation"`
	CType           string          `json:"ctype"`
	LocaleProvider  string          `json:"locale_provider"`
	Locale          *string         `json:"locale"`
	ConnectionCount int             `json:"connection_count"`
	Security        SecurityStatus  `json:"security"`
	SavedCredential SavedCredential `json:"saved_credential"`
}

type TableItem struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

type TableListResult struct {
	Tables    []TableItem `json:"tables"`
	Truncated bool        `json:"truncated"`
}

const (
	defaultRowLimit    = 50
	MaxRowQueryRunes   = 128
	MaxRowDeleteValues = 500
)

type RowPage struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Total   int64            `json:"total"`
	Offset  int              `json:"offset"`
	Limit   int              `json:"limit"`
}

func vaultUnavailable() SavedCredential {
	return SavedCredential{Status: "not_available", Reason: "vault_unavailable"}
}

type SecuritySummary struct {
	DatabaseCount         int  `json:"database_count"`
	PublicConnectCount    int  `json:"public_connect_count"`
	ActiveConnectionCount int  `json:"active_connection_count"`
	ConnectionGroupCount  int  `json:"connection_group_count"`
	MissingPasswordCount  *int `json:"missing_password_count,omitempty"`
}

type SecurityDatabase struct {
	Name              string `json:"name"`
	Owner             string `json:"owner"`
	Protected         bool   `json:"protected"`
	PublicCanConnect  bool   `json:"public_can_connect"`
	OwnerIsSuperuser  bool   `json:"owner_is_superuser"`
	OwnerCanLogin     bool   `json:"owner_can_login"`
	OwnerCreatedb     bool   `json:"owner_createdb"`
	OwnerCreaterole   bool   `json:"owner_createrole"`
	OwnerReplication  bool   `json:"owner_replication"`
	ActiveConnections int    `json:"active_connections"`
	RotationEligible  bool   `json:"rotation_eligible"`
}

type ConnectionGroup struct {
	Database    string `json:"database"`
	User        string `json:"user"`
	Client      string `json:"client"`
	Application string `json:"application"`
	State       string `json:"state"`
	Count       int    `json:"count"`
}

type SecurityOverview struct {
	Summary         SecuritySummary    `json:"summary"`
	SavedCredential SavedCredential    `json:"saved_credential"`
	Databases       []SecurityDatabase `json:"databases"`
	Connections     []ConnectionGroup  `json:"connections"`
	Truncated       bool               `json:"truncated"`
}
