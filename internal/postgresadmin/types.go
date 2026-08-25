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
	ListConnectionGroups(ctx context.Context) ([]ConnectionGroup, error)
	Ping(ctx context.Context) error
	PingPooled(ctx context.Context) error
}

type Inventory interface {
	List(ctx context.Context) (ListResult, error)
	Search(ctx context.Context, q string, limit int) (SearchResult, error)
	Details(ctx context.Context, name string) (DatabaseDetails, error)
	Tables(ctx context.Context, name string) (TableListResult, error)
	Rows(ctx context.Context, database, schema, table, q string, offset, limit int) (RowPage, error)
	SecurityOverview(ctx context.Context) (SecurityOverview, error)
	Ping(ctx context.Context) error
	PingPooled(ctx context.Context) error
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
	defaultRowLimit  = 50
	MaxRowQueryRunes = 128
)

type RowPage struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Total   int64            `json:"total"`
	Offset  int              `json:"offset"`
	Limit   int              `json:"limit"`
}

func vaultNotAvailable() SavedCredential {
	return SavedCredential{Status: "not_available", Reason: "vault_not_implemented"}
}

type SecuritySummary struct {
	DatabaseCount         int `json:"database_count"`
	PublicConnectCount    int `json:"public_connect_count"`
	ActiveConnectionCount int `json:"active_connection_count"`
	ConnectionGroupCount  int `json:"connection_group_count"`
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
