package platform

type SearchHit struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

type SearchGroup struct {
	ID        string      `json:"id"`
	Label     string      `json:"label"`
	Service   string      `json:"service"`
	Status    string      `json:"status"`
	Truncated bool        `json:"truncated"`
	Hits      []SearchHit `json:"hits"`
}

type PostgresSearch struct {
	Status    string
	Truncated bool
	Names     []string
}

func ResourceGroups(pg PostgresSearch) []SearchGroup {
	hits := make([]SearchHit, 0, len(pg.Names))
	status := pg.Status
	truncated := false
	if status == "ok" {
		truncated = pg.Truncated
		for _, name := range pg.Names {
			hits = append(hits, SearchHit{
				ID:    "postgres_database:" + name,
				Type:  "postgres_database",
				Label: name,
			})
		}
	}
	if status == "" {
		status = "unavailable"
	}
	return []SearchGroup{
		{
			ID:        "postgres_databases",
			Label:     "PostgreSQL databases",
			Service:   "postgres",
			Status:    status,
			Truncated: truncated,
			Hits:      hits,
		},
		{
			ID:        "redis_acl_users",
			Label:     "Redis ACL users",
			Service:   "redis",
			Status:    "not_implemented",
			Truncated: false,
			Hits:      []SearchHit{},
		},
	}
}
