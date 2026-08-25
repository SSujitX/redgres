package postgresadmin

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/SSujitX/redgres/internal/secrets"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	projectRoleConnectionLimit = 20
	createCompensateTimeout    = 30 * time.Second
)

const insertCredentialSQL = `INSERT INTO public.project_credentials (role_name, encrypted_password, updated_at) VALUES ($1, $2, now())`

const deleteCredentialSQL = `DELETE FROM public.project_credentials WHERE role_name = $1`

const databaseExistsSQL = `SELECT 1 FROM pg_database WHERE datname = $1`

const roleExistsSQL = `SELECT 1 FROM pg_roles WHERE rolname = $1`

const terminateDatabaseSQL = `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`

const ownedDatabaseCountSQL = `SELECT COUNT(*)::int FROM pg_database WHERE pg_catalog.pg_get_userbyid(datdba) = $1`

func quoteStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func formatCreateRole(owner, password string) (string, error) {
	quotedOwner, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	return "CREATE ROLE " + quotedOwner + " LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION CONNECTION LIMIT " + strconv.Itoa(projectRoleConnectionLimit) + " PASSWORD " + quoteStringLiteral(password), nil
}

func formatGrantSetRole(owner, admin string) (string, error) {
	quotedOwner, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	quotedAdmin, err := QuoteIdentifier(admin)
	if err != nil {
		return "", err
	}
	return "GRANT " + quotedOwner + " TO " + quotedAdmin + " WITH INHERIT TRUE, SET TRUE", nil
}

func formatCreateDatabase(database, owner string) (string, error) {
	quotedDB, err := QuoteIdentifier(database)
	if err != nil {
		return "", err
	}
	quotedOwner, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	return "CREATE DATABASE " + quotedDB + " OWNER " + quotedOwner, nil
}

func formatRevokePublicConnect(database string) (string, error) {
	quotedDB, err := QuoteIdentifier(database)
	if err != nil {
		return "", err
	}
	return "REVOKE CONNECT ON DATABASE " + quotedDB + " FROM PUBLIC", nil
}

func formatGrantConnect(database, owner string) (string, error) {
	quotedDB, err := QuoteIdentifier(database)
	if err != nil {
		return "", err
	}
	quotedOwner, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	return "GRANT CONNECT ON DATABASE " + quotedDB + " TO " + quotedOwner, nil
}

func formatDropDatabase(database string) (string, error) {
	quotedDB, err := QuoteIdentifier(database)
	if err != nil {
		return "", err
	}
	return "DROP DATABASE IF EXISTS " + quotedDB, nil
}

func formatDropRole(owner string) (string, error) {
	quotedOwner, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	return "DROP ROLE IF EXISTS " + quotedOwner, nil
}

func skipGrantSetRole(admin, owner string) bool {
	return admin == "" || admin == "postgres" || admin == owner
}

func (s *Service) Create(ctx context.Context, database, owner string) (CreatedDatabase, error) {
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

	password, err := GeneratePassword()
	if err != nil {
		return CreatedDatabase{}, ErrUnavailable
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

	if err := s.catalog.CreateDatabase(ctx, database, owner); err != nil {
		compensate()
		return CreatedDatabase{}, mapCatalogError(err)
	}
	createdDB = true

	if err := s.catalog.LockConnect(ctx, database, owner); err != nil {
		compensate()
		return CreatedDatabase{}, mapCatalogError(err)
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

	return CreatedDatabase{Database: database, Owner: owner, Password: password}, nil
}

func (s *Service) compensateCreate(ctx context.Context, database, owner string, createdRole, createdDB, insertedVault bool) {
	cctx, cancel := compensateContext(ctx)
	defer cancel()
	if insertedVault {
		_ = s.catalog.DeleteCredential(cctx, owner)
	}
	if createdDB && !s.policy.DatabaseDenied(database) {
		_ = s.catalog.TerminateAndDropDatabase(cctx, database)
	}
	if createdRole && !s.policy.OwnerDenied(owner) {
		n, err := s.catalog.OwnedDatabaseCount(cctx, owner)
		if err == nil && n == 0 {
			_ = s.catalog.DropRole(cctx, owner)
		}
	}
}

func compensateContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	return context.WithTimeout(context.WithoutCancel(ctx), createCompensateTimeout)
}

func (c PoolCatalog) execSimple(ctx context.Context, sql string) error {
	if c.pool == nil {
		return ErrUnavailable
	}
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer conn.Release()
	results := conn.Conn().PgConn().Exec(ctx, sql)
	_, err = results.ReadAll()
	if err != nil {
		return mapCreateSQLError(err)
	}
	return nil
}

func mapCreateSQLError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "42710":
			return Conflict{Field: conflictFieldOwner}
		case "42P04":
			return Conflict{Field: conflictFieldDatabase}
		}
	}
	return ErrUnavailable
}

func (c PoolCatalog) existsQuery(ctx context.Context, sql, name string) (bool, error) {
	if c.pool == nil {
		return false, ErrUnavailable
	}
	var n int
	err := c.pool.QueryRow(ctx, sql, name).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, ErrUnavailable
	}
	return true, nil
}

func (c PoolCatalog) DatabaseExists(ctx context.Context, name string) (bool, error) {
	if err := ValidateIdentifier(name); err != nil {
		return false, err
	}
	return c.existsQuery(ctx, databaseExistsSQL, name)
}

func (c PoolCatalog) RoleExists(ctx context.Context, name string) (bool, error) {
	if err := ValidateIdentifier(name); err != nil {
		return false, err
	}
	return c.existsQuery(ctx, roleExistsSQL, name)
}

func (c PoolCatalog) CreateRole(ctx context.Context, owner, password string) error {
	sql, err := formatCreateRole(owner, password)
	if err != nil {
		return err
	}
	return c.execSimple(ctx, sql)
}

func (c PoolCatalog) GrantSetRole(ctx context.Context, owner, admin string) error {
	sql, err := formatGrantSetRole(owner, admin)
	if err != nil {
		return err
	}
	return c.execSimple(ctx, sql)
}

func (c PoolCatalog) CreateDatabase(ctx context.Context, database, owner string) error {
	sql, err := formatCreateDatabase(database, owner)
	if err != nil {
		return err
	}
	return c.execSimple(ctx, sql)
}

func (c PoolCatalog) LockConnect(ctx context.Context, database, owner string) error {
	revoke, err := formatRevokePublicConnect(database)
	if err != nil {
		return err
	}
	if err := c.execSimple(ctx, revoke); err != nil {
		return err
	}
	grant, err := formatGrantConnect(database, owner)
	if err != nil {
		return err
	}
	return c.execSimple(ctx, grant)
}

func (c PoolCatalog) InsertCredential(ctx context.Context, role, encrypted string) error {
	if err := ValidateIdentifier(role); err != nil {
		return err
	}
	conn, connectCtx, closeConn, err := c.connectTarget(ctx, vaultDatabase)
	if err != nil {
		return ErrUnavailable
	}
	defer closeConn()
	if _, err := conn.Exec(connectCtx, insertCredentialSQL, role, encrypted); err != nil {
		return mapCreateSQLError(err)
	}
	return nil
}

func (c PoolCatalog) DeleteCredential(ctx context.Context, role string) error {
	if err := ValidateIdentifier(role); err != nil {
		return err
	}
	conn, connectCtx, closeConn, err := c.connectTarget(ctx, vaultDatabase)
	if err != nil {
		return ErrUnavailable
	}
	defer closeConn()
	if _, err := conn.Exec(connectCtx, deleteCredentialSQL, role); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (c PoolCatalog) TerminateAndDropDatabase(ctx context.Context, database string) error {
	if err := ValidateIdentifier(database); err != nil {
		return err
	}
	if c.pool == nil {
		return ErrUnavailable
	}
	_, _ = c.pool.Exec(ctx, terminateDatabaseSQL, database)
	sql, err := formatDropDatabase(database)
	if err != nil {
		return err
	}
	return c.execSimple(ctx, sql)
}

func (c PoolCatalog) OwnedDatabaseCount(ctx context.Context, owner string) (int, error) {
	if err := ValidateIdentifier(owner); err != nil {
		return 0, err
	}
	if c.pool == nil {
		return 0, ErrUnavailable
	}
	var n int
	if err := c.pool.QueryRow(ctx, ownedDatabaseCountSQL, owner).Scan(&n); err != nil {
		return 0, ErrUnavailable
	}
	return n, nil
}

func (c PoolCatalog) DropRole(ctx context.Context, owner string) error {
	sql, err := formatDropRole(owner)
	if err != nil {
		return err
	}
	return c.execSimple(ctx, sql)
}
