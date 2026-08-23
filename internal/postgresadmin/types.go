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
}

type Inventory interface {
	List(ctx context.Context) (ListResult, error)
	Details(ctx context.Context, name string) (DatabaseDetails, error)
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

func vaultNotAvailable() SavedCredential {
	return SavedCredential{Status: "not_available", Reason: "vault_not_implemented"}
}
