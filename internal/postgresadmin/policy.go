package postgresadmin

import (
	"strings"

	"github.com/SSujitX/redgres/internal/config"
)

var hardcodedProtectedDatabases = []string{
	"postgres",
	"template0",
	"template1",
	"database_console_vault",
}

var hardcodedProtectedRoles = []string{
	"postgres",
	"database_console",
	"onelife_pg_admin",
	"pg_database_owner",
	"pg_read_all_data",
	"pg_write_all_data",
}

type Policy struct {
	databases map[string]struct{}
	roles     map[string]struct{}
	adminUser string
}

func NewPolicy(cfg config.Config) Policy {
	databases := setOf(hardcodedProtectedDatabases)
	addAll(databases, cfg.PostgresProtectedDatabases)
	if cfg.PostgresDatabase != "" {
		databases[cfg.PostgresDatabase] = struct{}{}
	}
	roles := setOf(hardcodedProtectedRoles)
	addAll(roles, cfg.PostgresProtectedRoles)
	if cfg.PostgresUser != "" {
		roles[cfg.PostgresUser] = struct{}{}
	}
	return Policy{databases: databases, roles: roles, adminUser: cfg.PostgresUser}
}

func (p Policy) AdminUser() string {
	return p.adminUser
}

func (p Policy) DatabaseDenied(name string) bool {
	_, ok := p.databases[name]
	return ok
}

func (p Policy) OwnerDenied(owner string) bool {
	if strings.HasPrefix(owner, "pg_") {
		return true
	}
	_, ok := p.roles[owner]
	return ok
}

func (p Policy) Manageable(name, owner string, allowConn, isTemplate bool) bool {
	if !allowConn || isTemplate {
		return false
	}
	if p.DatabaseDenied(name) || p.OwnerDenied(owner) {
		return false
	}
	return true
}

func setOf(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	addAll(out, values)
	return out
}

func addAll(dst map[string]struct{}, values []string) {
	for _, value := range values {
		if value != "" {
			dst[value] = struct{}{}
		}
	}
}
