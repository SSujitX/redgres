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

type RedisSearch struct {
	Status    string
	Truncated bool
	Names     []string
}

func ResourceGroups(pg PostgresSearch, redis RedisSearch) []SearchGroup {
	pgHits := make([]SearchHit, 0, len(pg.Names))
	pgStatus := pg.Status
	pgTruncated := false
	if pgStatus == "ok" {
		pgTruncated = pg.Truncated
		for _, name := range pg.Names {
			pgHits = append(pgHits, SearchHit{
				ID:    "postgres_database:" + name,
				Type:  "postgres_database",
				Label: name,
			})
		}
	}
	if pgStatus == "" {
		pgStatus = "unavailable"
	}

	redisHits := make([]SearchHit, 0, len(redis.Names))
	redisStatus := redis.Status
	redisTruncated := false
	if redisStatus == "ok" {
		redisTruncated = redis.Truncated
		for _, name := range redis.Names {
			redisHits = append(redisHits, SearchHit{
				ID:    "redis_acl_user:" + name,
				Type:  "redis_acl_user",
				Label: name,
			})
		}
	}
	if redisStatus == "" {
		redisStatus = "unavailable"
	}

	return []SearchGroup{
		{
			ID:        "postgres_databases",
			Label:     "PostgreSQL databases",
			Service:   "postgres",
			Status:    pgStatus,
			Truncated: pgTruncated,
			Hits:      pgHits,
		},
		{
			ID:        "redis_acl_users",
			Label:     "Redis ACL users",
			Service:   "redis",
			Status:    redisStatus,
			Truncated: redisTruncated,
			Hits:      redisHits,
		},
	}
}
