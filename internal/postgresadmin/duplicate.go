package postgresadmin

import (
	"context"
	"errors"

	"github.com/SSujitX/redgres/internal/secrets"
	"github.com/jackc/pgx/v5"
)

const ownershipSnapshotSQL = `
SELECT
	pg_catalog.pg_get_userbyid(d.datdba),
	COALESCE(d.datacl::text, '')
FROM pg_database d
WHERE d.datname = $1
`

const cloneNamespaceSQL = `
SELECT n.nspname, pg_catalog.pg_get_userbyid(n.nspowner), COALESCE(r.rolsuper, false)
FROM pg_catalog.pg_namespace n
LEFT JOIN pg_catalog.pg_roles r ON r.oid = n.nspowner
WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
  AND n.nspname !~ '^pg_(toast|temp)'
  AND pg_catalog.pg_get_userbyid(n.nspowner) <> $1
  AND NOT EXISTS (
    SELECT 1
    FROM pg_catalog.pg_depend d
    WHERE d.classid = 'pg_catalog.pg_namespace'::pg_catalog.regclass
      AND d.objid = n.oid
      AND d.deptype = 'e'
  )
`

const cloneRelationSQL = `
SELECT
	n.nspname,
	c.relname,
	c.relkind,
	pg_catalog.pg_get_userbyid(c.relowner),
	COALESCE(r.rolsuper, false)
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_catalog.pg_roles r ON r.oid = c.relowner
WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
  AND n.nspname !~ '^pg_(toast|temp)'
  AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f', 'i', 'I')
  AND pg_catalog.pg_get_userbyid(c.relowner) <> $1
  AND NOT EXISTS (
    SELECT 1
    FROM pg_catalog.pg_depend d
    WHERE d.classid = 'pg_catalog.pg_class'::pg_catalog.regclass
      AND d.objid = c.oid
      AND d.deptype = 'e'
  )
ORDER BY
  CASE c.relkind
    WHEN 'S' THEN 1
    WHEN 'r' THEN 2
    WHEN 'p' THEN 2
    WHEN 'f' THEN 2
    WHEN 'v' THEN 3
    WHEN 'm' THEN 3
    ELSE 4
  END,
  n.nspname,
  c.relname
`

const cloneTypeSQL = `
SELECT
	n.nspname,
	t.typname,
	pg_catalog.pg_get_userbyid(t.typowner),
	COALESCE(r.rolsuper, false)
FROM pg_catalog.pg_type t
JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace
LEFT JOIN pg_catalog.pg_roles r ON r.oid = t.typowner
WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
  AND t.typtype IN ('b', 'c', 'd', 'e', 'r')
  AND NOT (t.typlen = -1 AND t.typelem <> 0)
  AND (
    t.typrelid = 0
    OR EXISTS (
      SELECT 1 FROM pg_catalog.pg_class c
      WHERE c.oid = t.typrelid AND c.relkind = 'c'
    )
  )
  AND pg_catalog.pg_get_userbyid(t.typowner) <> $1
  AND NOT EXISTS (
    SELECT 1
    FROM pg_catalog.pg_depend d
    WHERE d.classid = 'pg_catalog.pg_type'::pg_catalog.regclass
      AND d.objid = t.oid
      AND d.deptype = 'e'
  )
`

const cloneProcSQL = `
SELECT
	n.nspname,
	p.proname,
	pg_catalog.pg_get_function_identity_arguments(p.oid),
	p.prokind,
	pg_catalog.pg_get_userbyid(p.proowner),
	COALESCE(r.rolsuper, false)
FROM pg_catalog.pg_proc p
JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
LEFT JOIN pg_catalog.pg_roles r ON r.oid = p.proowner
WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND pg_catalog.pg_get_userbyid(p.proowner) <> $1
  AND NOT EXISTS (
    SELECT 1
    FROM pg_catalog.pg_depend d
    WHERE d.classid = 'pg_catalog.pg_proc'::pg_catalog.regclass
      AND d.objid = p.oid
      AND d.deptype = 'e'
  )
`

func formatCreateDatabaseTemplate(database, source, owner string) (string, error) {
	quotedDB, err := QuoteIdentifier(database)
	if err != nil {
		return "", err
	}
	quotedSource, err := QuoteIdentifier(source)
	if err != nil {
		return "", err
	}
	quotedOwner, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	return "CREATE DATABASE " + quotedDB + " TEMPLATE " + quotedSource + " OWNER " + quotedOwner, nil
}

func formatAlterSchemaOwner(schema, newOwner string) (string, error) {
	quotedSchema, err := QuoteCatalogIdentifier(schema)
	if err != nil {
		return "", err
	}
	quotedOwner, err := QuoteIdentifier(newOwner)
	if err != nil {
		return "", err
	}
	return "ALTER SCHEMA " + quotedSchema + " OWNER TO " + quotedOwner, nil
}

func formatAlterRelationOwner(relkind, schema, name, newOwner string) (string, error) {
	kind, ok := cloneRelationKindSQL[relkind]
	if !ok {
		return "", ErrUnavailable
	}
	quotedSchema, err := QuoteCatalogIdentifier(schema)
	if err != nil {
		return "", err
	}
	quotedName, err := QuoteCatalogIdentifier(name)
	if err != nil {
		return "", err
	}
	quotedOwner, err := QuoteIdentifier(newOwner)
	if err != nil {
		return "", err
	}
	return kind + " " + quotedSchema + "." + quotedName + " OWNER TO " + quotedOwner, nil
}

func formatAlterTypeOwner(schema, name, newOwner string) (string, error) {
	quotedSchema, err := QuoteCatalogIdentifier(schema)
	if err != nil {
		return "", err
	}
	quotedName, err := QuoteCatalogIdentifier(name)
	if err != nil {
		return "", err
	}
	quotedOwner, err := QuoteIdentifier(newOwner)
	if err != nil {
		return "", err
	}
	return "ALTER TYPE " + quotedSchema + "." + quotedName + " OWNER TO " + quotedOwner, nil
}

func formatAlterRoutineOwner(prokind, schema, name, identityArgs, newOwner string) (string, error) {
	kind := cloneProcKindSQL[prokind]
	if kind == "" {
		kind = "ALTER FUNCTION"
	}
	quotedSchema, err := QuoteCatalogIdentifier(schema)
	if err != nil {
		return "", err
	}
	quotedName, err := QuoteCatalogIdentifier(name)
	if err != nil {
		return "", err
	}
	quotedOwner, err := QuoteIdentifier(newOwner)
	if err != nil {
		return "", err
	}
	return kind + " " + quotedSchema + "." + quotedName + "(" + identityArgs + ") OWNER TO " + quotedOwner, nil
}

func formatGrantAllOnPublic(newOwner string) ([]string, error) {
	quotedOwner, err := QuoteIdentifier(newOwner)
	if err != nil {
		return nil, err
	}
	return []string{
		"GRANT ALL ON SCHEMA public TO " + quotedOwner,
		"GRANT ALL ON ALL TABLES IN SCHEMA public TO " + quotedOwner,
		"GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO " + quotedOwner,
		"GRANT ALL ON ALL FUNCTIONS IN SCHEMA public TO " + quotedOwner,
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO " + quotedOwner,
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO " + quotedOwner,
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON FUNCTIONS TO " + quotedOwner,
	}, nil
}

var cloneRelationKindSQL = map[string]string{
	"r": "ALTER TABLE",
	"p": "ALTER TABLE",
	"v": "ALTER VIEW",
	"m": "ALTER MATERIALIZED VIEW",
	"S": "ALTER SEQUENCE",
	"f": "ALTER FOREIGN TABLE",
	"i": "ALTER INDEX",
	"I": "ALTER INDEX",
}

var cloneProcKindSQL = map[string]string{
	"f": "ALTER FUNCTION",
	"w": "ALTER FUNCTION",
	"p": "ALTER PROCEDURE",
	"a": "ALTER AGGREGATE",
}

func skipCloneObjectOwner(policy Policy, owner string) bool {
	return policy.OwnerDenied(owner)
}

func skipCloneTransferOwner(skipOwner func(string) bool, owner string, ownerIsSuperuser bool) bool {
	if ownerIsSuperuser {
		return true
	}
	return skipOwner != nil && skipOwner(owner)
}

func formatGrantCatalogRole(owner, admin string) (string, error) {
	quotedOwner, err := QuoteCatalogIdentifier(owner)
	if err != nil {
		return "", err
	}
	quotedAdmin, err := QuoteIdentifier(admin)
	if err != nil {
		return "", err
	}
	return "GRANT " + quotedOwner + " TO " + quotedAdmin + " WITH INHERIT TRUE, SET TRUE", nil
}

func (s *Service) Duplicate(ctx context.Context, source, database, owner string) (CreatedDatabase, error) {
	if err := ValidateIdentifier(source); err != nil {
		return CreatedDatabase{}, err
	}
	if err := ValidateIdentifier(database); err != nil {
		return CreatedDatabase{}, err
	}
	if err := ValidateIdentifier(owner); err != nil {
		return CreatedDatabase{}, err
	}
	if s == nil {
		return CreatedDatabase{}, ErrUnavailable
	}
	if s.policy.DatabaseDenied(database) || s.policy.OwnerDenied(owner) {
		return CreatedDatabase{}, ErrProtected
	}
	if s.catalog == nil {
		return CreatedDatabase{}, ErrUnavailable
	}
	if s.policy.DatabaseDenied(source) {
		return CreatedDatabase{}, ErrNotFound
	}

	row, err := s.catalog.Lookup(ctx, source)
	if err != nil {
		return CreatedDatabase{}, mapCatalogError(err)
	}
	if row.Owner == "" || !row.AllowConn || row.IsTemplate {
		return CreatedDatabase{}, ErrNotFound
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) && s.policy.DatabaseDenied(row.Name) {
		return CreatedDatabase{}, ErrNotFound
	}
	if row.OwnerIsSuperuser || !row.OwnerCanLogin || s.policy.OwnerDenied(row.Owner) {
		return CreatedDatabase{}, ErrProtected
	}
	if owner == row.Owner {
		return CreatedDatabase{}, FieldError{Field: conflictFieldOwner, Message: duplicateSameOwnerMessage}
	}
	if s.vaultKey == "" {
		return CreatedDatabase{}, ErrUnavailable
	}

	existsDB, err := s.catalog.DatabaseExists(ctx, database)
	if err != nil {
		return CreatedDatabase{}, mapCatalogError(err)
	}
	if existsDB {
		return CreatedDatabase{}, Conflict{Field: conflictFieldDatabase}
	}
	existsRole, err := s.catalog.RoleExists(ctx, owner)
	if err != nil {
		return CreatedDatabase{}, mapCatalogError(err)
	}
	if existsRole {
		return CreatedDatabase{}, Conflict{Field: conflictFieldOwner}
	}
	if _, err := s.catalog.SavedRoleNames(ctx, []string{owner}); err != nil {
		return CreatedDatabase{}, ErrUnavailable
	}

	password, err := GeneratePassword()
	if err != nil {
		return CreatedDatabase{}, ErrUnavailable
	}
	if !s.tryLockDuplicate(source, database, owner) {
		return CreatedDatabase{}, DuplicateInProgress{}
	}
	defer s.unlockDuplicate(source, database, owner)

	snapshot, err := s.catalog.OwnershipSnapshot(ctx, source)
	if err != nil {
		return CreatedDatabase{}, mapCatalogError(err)
	}

	var createdRole, createdDB, insertedVault bool
	compensate := func() {
		s.compensateCreate(ctx, database, owner, createdRole, createdDB, insertedVault)
	}

	if err := s.catalog.CreateRole(ctx, owner, password); err != nil {
		return CreatedDatabase{}, mapCatalogError(err)
	}
	createdRole = true

	admin := s.policy.AdminUser()
	if !skipGrantSetRole(admin, owner) {
		if err := s.catalog.GrantSetRole(ctx, owner, admin); err != nil {
			compensate()
			return CreatedDatabase{}, mapCatalogError(err)
		}
	}

	if err := s.catalog.TerminateSessions(ctx, source); err != nil {
		compensate()
		return CreatedDatabase{}, mapCatalogError(err)
	}

	if err := s.catalog.CreateDatabaseTemplate(ctx, database, source, owner); err != nil {
		compensate()
		return CreatedDatabase{}, mapCatalogError(err)
	}
	createdDB = true

	if err := s.catalog.LockConnect(ctx, database, owner); err != nil {
		compensate()
		return CreatedDatabase{}, mapCatalogError(err)
	}

	if err := s.assertSourceUnchanged(ctx, source, snapshot); err != nil {
		compensate()
		return CreatedDatabase{}, err
	}

	token, err := secrets.Encrypt(s.vaultKey, []byte(password))
	if err != nil {
		compensate()
		return CreatedDatabase{}, ErrUnavailable
	}
	if err := s.catalog.InsertCredential(ctx, owner, token); err != nil {
		compensate()
		return CreatedDatabase{}, mapCatalogError(err)
	}
	insertedVault = true

	if err := s.catalog.TransferCloneOwnership(ctx, database, owner, admin, func(current string) bool {
		return skipCloneObjectOwner(s.policy, current)
	}); err != nil {
		compensate()
		return CreatedDatabase{}, mapCatalogError(err)
	}

	if err := s.assertSourceUnchanged(ctx, source, snapshot); err != nil {
		compensate()
		return CreatedDatabase{}, err
	}

	return CreatedDatabase{Database: database, Owner: owner, Password: password}, nil
}

func (s *Service) assertSourceUnchanged(ctx context.Context, source string, snapshot OwnershipSnapshot) error {
	current, err := s.catalog.OwnershipSnapshot(ctx, source)
	if err != nil {
		return mapCatalogError(err)
	}
	if current.Owner != snapshot.Owner || current.Datacl != snapshot.Datacl {
		return IsolationChanged{}
	}
	return nil
}

func duplicateLockKeys(source, database, owner string) []string {
	return []string{"db:" + source, "db:" + database, "role:" + owner}
}

func (s *Service) tryLockDuplicate(source, database, owner string) bool {
	s.duplicateMu.Lock()
	defer s.duplicateMu.Unlock()
	if s.duplicating == nil {
		s.duplicating = map[string]struct{}{}
	}
	keys := duplicateLockKeys(source, database, owner)
	for _, key := range keys {
		if _, held := s.duplicating[key]; held {
			return false
		}
	}
	for _, key := range keys {
		s.duplicating[key] = struct{}{}
	}
	return true
}

func (s *Service) unlockDuplicate(source, database, owner string) {
	s.duplicateMu.Lock()
	defer s.duplicateMu.Unlock()
	for _, key := range duplicateLockKeys(source, database, owner) {
		delete(s.duplicating, key)
	}
}

func (c PoolCatalog) OwnershipSnapshot(ctx context.Context, database string) (OwnershipSnapshot, error) {
	if err := ValidateIdentifier(database); err != nil {
		return OwnershipSnapshot{}, err
	}
	if c.pool == nil {
		return OwnershipSnapshot{}, ErrUnavailable
	}
	var snap OwnershipSnapshot
	err := c.pool.QueryRow(ctx, ownershipSnapshotSQL, database).Scan(&snap.Owner, &snap.Datacl)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OwnershipSnapshot{}, ErrNotFound
		}
		return OwnershipSnapshot{}, ErrUnavailable
	}
	return snap, nil
}

func (c PoolCatalog) TerminateSessions(ctx context.Context, database string) error {
	if err := ValidateIdentifier(database); err != nil {
		return err
	}
	if c.pool == nil {
		return ErrUnavailable
	}
	if _, err := c.pool.Exec(ctx, terminateDatabaseSQL, database); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (c PoolCatalog) CreateDatabaseTemplate(ctx context.Context, database, source, owner string) error {
	sql, err := formatCreateDatabaseTemplate(database, source, owner)
	if err != nil {
		return err
	}
	return c.execSimple(ctx, sql)
}

func (c PoolCatalog) TransferCloneOwnership(ctx context.Context, database, newOwner, admin string, skipOwner func(string) bool) error {
	if err := ValidateIdentifier(database); err != nil {
		return err
	}
	if err := ValidateIdentifier(newOwner); err != nil {
		return err
	}
	if skipOwner == nil {
		skipOwner = func(string) bool { return false }
	}
	conn, queryCtx, closeConn, err := c.connectClone(ctx, database)
	if err != nil {
		return ErrUnavailable
	}
	defer closeConn()

	if !skipGrantSetRole(admin, newOwner) {
		grant, grantErr := formatGrantSetRole(newOwner, admin)
		if grantErr != nil {
			return grantErr
		}
		if _, err := conn.Exec(queryCtx, grant); err != nil {
			return ErrUnavailable
		}
	}

	if err := c.transferCloneNamespaces(queryCtx, conn, newOwner, admin, skipOwner); err != nil {
		return err
	}
	if err := c.transferCloneRelations(queryCtx, conn, newOwner, admin, skipOwner); err != nil {
		return err
	}
	if err := c.transferCloneTypes(queryCtx, conn, newOwner, admin, skipOwner); err != nil {
		return err
	}
	if err := c.transferCloneRoutines(queryCtx, conn, newOwner, admin, skipOwner); err != nil {
		return err
	}

	grants, err := formatGrantAllOnPublic(newOwner)
	if err != nil {
		return err
	}
	for _, sql := range grants {
		if _, err := conn.Exec(queryCtx, sql); err != nil {
			return ErrUnavailable
		}
	}
	return nil
}

func (c PoolCatalog) connectClone(ctx context.Context, database string) (*pgx.Conn, context.Context, context.CancelFunc, error) {
	return c.connectTarget(ctx, database)
}

func (c PoolCatalog) alterCloneOwner(ctx context.Context, conn *pgx.Conn, currentOwner, admin, statement string, skipOwner func(string) bool) error {
	if skipOwner(currentOwner) {
		return nil
	}
	if !skipGrantSetRole(admin, currentOwner) {
		grant, err := formatGrantCatalogRole(currentOwner, admin)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, grant); err != nil {
			return ErrUnavailable
		}
	}
	if _, err := conn.Exec(ctx, statement); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (c PoolCatalog) transferCloneNamespaces(ctx context.Context, conn *pgx.Conn, newOwner, admin string, skipOwner func(string) bool) error {
	rows, err := conn.Query(ctx, cloneNamespaceSQL, newOwner)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var schema, oldOwner string
		var ownerIsSuperuser bool
		if err := rows.Scan(&schema, &oldOwner, &ownerIsSuperuser); err != nil {
			return ErrUnavailable
		}
		if skipCloneTransferOwner(skipOwner, oldOwner, ownerIsSuperuser) {
			continue
		}
		sql, err := formatAlterSchemaOwner(schema, newOwner)
		if err != nil {
			return err
		}
		if err := c.alterCloneOwner(ctx, conn, oldOwner, admin, sql, skipOwner); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (c PoolCatalog) transferCloneRelations(ctx context.Context, conn *pgx.Conn, newOwner, admin string, skipOwner func(string) bool) error {
	rows, err := conn.Query(ctx, cloneRelationSQL, newOwner)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name, relkind, oldOwner string
		var ownerIsSuperuser bool
		if err := rows.Scan(&schema, &name, &relkind, &oldOwner, &ownerIsSuperuser); err != nil {
			return ErrUnavailable
		}
		if skipCloneTransferOwner(skipOwner, oldOwner, ownerIsSuperuser) {
			continue
		}
		if len(relkind) != 1 {
			return ErrUnavailable
		}
		sql, err := formatAlterRelationOwner(relkind, schema, name, newOwner)
		if err != nil {
			return err
		}
		if err := c.alterCloneOwner(ctx, conn, oldOwner, admin, sql, skipOwner); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (c PoolCatalog) transferCloneTypes(ctx context.Context, conn *pgx.Conn, newOwner, admin string, skipOwner func(string) bool) error {
	rows, err := conn.Query(ctx, cloneTypeSQL, newOwner)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name, oldOwner string
		var ownerIsSuperuser bool
		if err := rows.Scan(&schema, &name, &oldOwner, &ownerIsSuperuser); err != nil {
			return ErrUnavailable
		}
		if skipCloneTransferOwner(skipOwner, oldOwner, ownerIsSuperuser) {
			continue
		}
		sql, err := formatAlterTypeOwner(schema, name, newOwner)
		if err != nil {
			return err
		}
		if err := c.alterCloneOwner(ctx, conn, oldOwner, admin, sql, skipOwner); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (c PoolCatalog) transferCloneRoutines(ctx context.Context, conn *pgx.Conn, newOwner, admin string, skipOwner func(string) bool) error {
	rows, err := conn.Query(ctx, cloneProcSQL, newOwner)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name, identityArgs, prokind, oldOwner string
		var ownerIsSuperuser bool
		if err := rows.Scan(&schema, &name, &identityArgs, &prokind, &oldOwner, &ownerIsSuperuser); err != nil {
			return ErrUnavailable
		}
		if skipCloneTransferOwner(skipOwner, oldOwner, ownerIsSuperuser) {
			continue
		}
		if len(prokind) != 1 {
			return ErrUnavailable
		}
		sql, err := formatAlterRoutineOwner(prokind, schema, name, identityArgs, newOwner)
		if err != nil {
			return err
		}
		if err := c.alterCloneOwner(ctx, conn, oldOwner, admin, sql, skipOwner); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnavailable
	}
	return nil
}
