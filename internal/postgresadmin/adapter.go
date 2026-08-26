package postgresadmin

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/secrets"
	"github.com/SSujitX/redgres/internal/securefile"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const catalogSQL = `
SELECT
	d.datname,
	pg_catalog.pg_get_userbyid(d.datdba),
	pg_catalog.pg_database_size(d.oid),
	pg_catalog.pg_size_pretty(pg_catalog.pg_database_size(d.oid)),
	d.datcollate,
	d.datctype,
	d.datlocprovider::text,
	d.datlocale,
	d.datallowconn,
	d.datistemplate,
	COALESCE(c.n, 0),
	EXISTS (
		SELECT 1
		FROM aclexplode(COALESCE(d.datacl, acldefault('d', d.datdba))) acl
		WHERE acl.grantee = 0
		  AND acl.privilege_type = 'CONNECT'
	),
	r.rolsuper,
	r.rolcanlogin,
	r.rolcreatedb,
	r.rolcreaterole,
	r.rolreplication
FROM pg_database d
JOIN pg_roles r ON r.oid = d.datdba
LEFT JOIN (
	SELECT datname, count(*)::int AS n
	FROM pg_stat_activity
	WHERE backend_type = 'client backend'
	  AND pid <> pg_backend_pid()
	GROUP BY datname
) c ON c.datname = d.datname
`

const listConnectionGroupsSQL = `
SELECT
	datname,
	usename,
	COALESCE(client_addr::text, 'local'),
	COALESCE(application_name, ''),
	state,
	count(*)::int
FROM pg_stat_activity
WHERE backend_type = 'client backend'
  AND pid <> pg_backend_pid()
GROUP BY datname, usename, client_addr, application_name, state
ORDER BY datname, usename, client_addr
`

type PoolCatalog struct {
	pool   *pgxpool.Pool
	pooled *pgxpool.Pool
}

func Open(ctx context.Context, cfg config.Config) (*Service, func(), error) {
	policy := NewPolicy(cfg)
	noop := func() {}
	if !cfg.PostgresConfigured() {
		if cfg.Production() {
			return nil, noop, errors.New("REDGRES_POSTGRES_HOST: production requires a complete administrative connection")
		}
		return NewService(nil, policy), noop, nil
	}
	password, err := readPasswordFile(cfg.PostgresPasswordFile, cfg.Production())
	if err != nil {
		return nil, noop, err
	}
	vaultKey, err := loadVaultKey(cfg)
	if err != nil {
		password = ""
		return nil, noop, err
	}
	pool, err := connectPool(ctx, cfg, password)
	if err != nil {
		password = ""
		return nil, noop, err
	}
	if err := checkServerMajor(ctx, pool, cfg.PostgresExpectedMajor); err != nil {
		password = ""
		pool.Close()
		return nil, noop, err
	}
	var pooled *pgxpool.Pool
	if cfg.PostgresPooledPort != "" {
		pooled, err = connectPooledPool(ctx, cfg, password)
		if err != nil {
			password = ""
			pool.Close()
			return nil, noop, err
		}
	}
	password = ""
	closer := func() {
		if pooled != nil {
			pooled.Close()
		}
		pool.Close()
	}
	svc := NewServiceWithVaultKey(PoolCatalog{pool: pool, pooled: pooled}, policy, vaultKey)
	return svc, closer, nil
}

func connectPool(ctx context.Context, cfg config.Config, password string) (*pgxpool.Pool, error) {
	poolCfg, err := adminPoolConfig(cfg, password)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, ErrUnavailable
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, ErrUnavailable
	}
	return pool, nil
}

func connectPooledPool(ctx context.Context, cfg config.Config, password string) (*pgxpool.Pool, error) {
	poolCfg, err := pooledPoolConfig(cfg, password)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, ErrUnavailable
	}
	return pool, nil
}

func adminPoolConfig(cfg config.Config, password string) (*pgxpool.Config, error) {
	port, err := strconv.ParseUint(cfg.PostgresPort, 10, 16)
	if err != nil {
		return nil, errors.New("REDGRES_POSTGRES_PORT: invalid value")
	}
	poolCfg, err := pgxpool.ParseConfig(keywordConnInfo(cfg))
	if err != nil {
		return nil, ErrUnavailable
	}
	poolCfg.ConnConfig.Password = password
	poolCfg.ConnConfig.Port = uint16(port)
	poolCfg.MaxConns = 4
	poolCfg.MinConns = 0
	poolCfg.MaxConnLifetime = time.Hour
	return poolCfg, nil
}

func pooledPoolConfig(cfg config.Config, password string) (*pgxpool.Config, error) {
	port, err := strconv.ParseUint(cfg.PostgresPooledPort, 10, 16)
	if err != nil {
		return nil, errors.New("REDGRES_POSTGRES_POOLED_PORT: invalid value")
	}
	poolCfg, err := pgxpool.ParseConfig(keywordPooledConnInfo(cfg))
	if err != nil {
		return nil, ErrUnavailable
	}
	poolCfg.ConnConfig.Password = password
	poolCfg.ConnConfig.Port = uint16(port)
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	poolCfg.ShouldPing = func(context.Context, pgxpool.ShouldPingParams) bool { return false }
	poolCfg.MaxConns = 4
	poolCfg.MinConns = 0
	poolCfg.MaxConnLifetime = time.Hour
	return poolCfg, nil
}

func keywordConnInfo(cfg config.Config) string {
	return keywordConnInfoForDB(cfg, cfg.PostgresDatabase)
}

func keywordPooledConnInfo(cfg config.Config) string {
	return keywordConnInfoForDB(cfg, "pgbouncer")
}

func keywordConnInfoForDB(cfg config.Config, dbname string) string {
	parts := []string{
		"host=" + keywordValue(cfg.PostgresHost),
		"user=" + keywordValue(cfg.PostgresUser),
		"dbname=" + keywordValue(dbname),
		"sslmode=" + keywordValue(cfg.PostgresSSLMode),
		"connect_timeout=10",
		"application_name=redgres",
	}
	if cfg.PostgresSSLRootCert != "" {
		parts = append(parts, "sslrootcert="+quoteKeyword(cfg.PostgresSSLRootCert))
	}
	return strings.Join(parts, " ")
}

func keywordValue(value string) string {
	if strings.ContainsAny(value, " \\'") {
		return quoteKeyword(value)
	}
	return value
}

func quoteKeyword(value string) string {
	return "'" + strings.ReplaceAll(value, `'`, `\'`) + "'"
}

func readPasswordFile(path string, production bool) (string, error) {
	raw, err := readSecretFile(path, "REDGRES_POSTGRES_PASSWORD_FILE", production)
	if err != nil {
		return "", err
	}
	password := string(raw)
	for i := range raw {
		raw[i] = 0
	}
	return password, nil
}

func readSecretFile(path, envName string, production bool) ([]byte, error) {
	raw, err := securefile.ReadRegular(path, func(mode fs.FileMode) error {
		if production && mode.Perm()&0o077 != 0 {
			return errors.New(envName + ": must not be group or world accessible")
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, securefile.ErrNotRegular) {
			return nil, errors.New(envName + ": must be a regular file")
		}
		if strings.HasPrefix(err.Error(), envName+":") {
			return nil, err
		}
		return nil, errors.New(envName + ": is unavailable")
	}
	trimmed := strings.TrimRight(string(raw), "\r\n")
	for i := range raw {
		raw[i] = 0
	}
	if trimmed == "" {
		return nil, errors.New(envName + ": is empty")
	}
	out := []byte(trimmed)
	return out, nil
}

func loadVaultKey(cfg config.Config) (string, error) {
	path := strings.TrimSpace(cfg.LegacyVaultSecretFile)
	if path == "" {
		return "", nil
	}
	raw, err := readSecretFile(path, "REDGRES_LEGACY_VAULT_SECRET_FILE", cfg.Production())
	if err != nil {
		return "", err
	}
	key := secrets.DeriveVaultKey(string(raw))
	for i := range raw {
		raw[i] = 0
	}
	return key, nil
}

func checkServerMajor(ctx context.Context, pool *pgxpool.Pool, expected int) error {
	var raw string
	if err := pool.QueryRow(ctx, "SELECT current_setting('server_version_num')").Scan(&raw); err != nil {
		return ErrUnavailable
	}
	num, err := strconv.Atoi(raw)
	if err != nil {
		return ErrUnavailable
	}
	major := num / 10000
	if major != 17 && major != 18 {
		return ErrUnavailable
	}
	if expected != 0 && expected != major {
		return ErrUnavailable
	}
	return nil
}

func (c PoolCatalog) Ping(ctx context.Context) error {
	if c.pool == nil {
		return ErrUnavailable
	}
	if err := c.pool.Ping(ctx); err != nil {
		return ErrUnavailable
	}
	return nil
}

const pooledShowVersionSQL = "SHOW VERSION"

func (c PoolCatalog) PingPooled(ctx context.Context) error {
	if c.pooled == nil {
		return ErrNotConfigured
	}
	if _, err := c.pooled.Exec(ctx, pooledShowVersionSQL); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (c PoolCatalog) List(ctx context.Context) ([]CatalogRow, error) {
	rows, err := c.pool.Query(ctx, catalogSQL+" ORDER BY d.datname")
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	var out []CatalogRow
	for rows.Next() {
		row, err := scanCatalogRow(rows)
		if err != nil {
			return nil, ErrUnavailable
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

const tableSearchPath = "pg_catalog,information_schema,pg_temp"

const listTablesSQL = `
SELECT table_schema, table_name
FROM information_schema.tables
WHERE table_type = 'BASE TABLE'
  AND table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY table_schema, table_name
LIMIT 501
`

func (c PoolCatalog) connectTarget(ctx context.Context, database string) (*pgx.Conn, context.Context, context.CancelFunc, error) {
	if c.pool == nil {
		return nil, nil, func() {}, ErrUnavailable
	}
	cfg := c.pool.Config()
	connCfg := cfg.ConnConfig.Copy()
	params := make(map[string]string, len(connCfg.RuntimeParams)+1)
	for key, value := range connCfg.RuntimeParams {
		params[key] = value
	}
	params["search_path"] = tableSearchPath
	connCfg.RuntimeParams = params
	connCfg.Database = database
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := pgx.ConnectConfig(connectCtx, connCfg)
	if err != nil {
		cancel()
		return nil, nil, func() {}, ErrUnavailable
	}
	if _, err := conn.Exec(connectCtx, "SELECT pg_catalog.set_config('search_path', $1, false)", tableSearchPath); err != nil {
		_ = conn.Close(context.Background())
		cancel()
		return nil, nil, func() {}, ErrUnavailable
	}
	return conn, connectCtx, func() {
		_ = conn.Close(context.Background())
		cancel()
	}, nil
}

func (c PoolCatalog) ListTables(ctx context.Context, database string) ([]TableItem, error) {
	if err := ValidateIdentifier(database); err != nil {
		return nil, err
	}
	conn, connectCtx, closeConn, err := c.connectTarget(ctx, database)
	if err != nil {
		return nil, err
	}
	defer closeConn()
	rows, err := conn.Query(connectCtx, listTablesSQL)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	var out []TableItem
	for rows.Next() {
		if len(out) >= listCap+1 {
			break
		}
		var item TableItem
		if err := rows.Scan(&item.Schema, &item.Name); err != nil {
			return nil, ErrUnavailable
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

func (c PoolCatalog) Lookup(ctx context.Context, name string) (CatalogRow, error) {
	row, err := scanCatalogRow(c.pool.QueryRow(ctx, catalogSQL+" WHERE d.datname = $1", name))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CatalogRow{}, ErrNotFound
		}
		return CatalogRow{}, ErrUnavailable
	}
	return row, nil
}

func (c PoolCatalog) ListConnectionGroups(ctx context.Context) ([]ConnectionGroup, error) {
	if c.pool == nil {
		return nil, ErrUnavailable
	}
	rows, err := c.pool.Query(ctx, listConnectionGroupsSQL)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := make([]ConnectionGroup, 0)
	for rows.Next() {
		var (
			database    *string
			user        *string
			client      string
			application *string
			state       *string
			count       int
		)
		if err := rows.Scan(&database, &user, &client, &application, &state, &count); err != nil {
			return nil, ErrUnavailable
		}
		group := ConnectionGroup{Client: client, Count: count}
		if database != nil {
			group.Database = *database
		}
		if user != nil {
			group.User = *user
		}
		if application != nil {
			group.Application = *application
		}
		if state != nil {
			group.State = *state
		}
		out = append(out, displayConnectionGroup(group))
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

const vaultDatabase = "database_console_vault"

// savedRoleNamesSQL is existence-only. $1 is a non-nil []string, encoded as
// PostgreSQL text[] by pgx v5.10.0 TryWrapSliceEncodePlan (pgtype/pgtype.go).
const savedRoleNamesSQL = `SELECT role_name FROM public.project_credentials WHERE role_name = ANY($1)`

func uniqueRoleNames(roles []string, max int) []string {
	seen := make(map[string]struct{}, len(roles))
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func mapVaultError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "3D000", "42P01":
			return ErrVaultUnavailable
		}
	}
	return ErrVaultUnavailable
}

func (c PoolCatalog) SavedRoleNames(ctx context.Context, roles []string) (map[string]struct{}, error) {
	if len(roles) == 0 {
		return map[string]struct{}{}, nil
	}
	unique := uniqueRoleNames(roles, listCap)
	if len(unique) == 0 {
		return map[string]struct{}{}, nil
	}
	conn, connectCtx, closeConn, err := c.connectTarget(ctx, vaultDatabase)
	if err != nil {
		return nil, mapVaultError(err)
	}
	defer closeConn()
	rows, err := conn.Query(connectCtx, savedRoleNamesSQL, unique)
	if err != nil {
		return nil, mapVaultError(err)
	}
	defer rows.Close()
	out := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, mapVaultError(err)
		}
		out[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, mapVaultError(err)
	}
	return out, nil
}

const systemIdentifierSQL = `SELECT system_identifier FROM pg_control_system()`

func formatSystemIdentifier(id int64) string {
	return strconv.FormatInt(id, 10)
}

func (c PoolCatalog) SystemIdentifier(ctx context.Context) (string, error) {
	if c.pool == nil {
		return "", ErrUnavailable
	}
	var id int64
	if err := c.pool.QueryRow(ctx, systemIdentifierSQL).Scan(&id); err != nil {
		return "", ErrUnavailable
	}
	return formatSystemIdentifier(id), nil
}

const encryptedPasswordSQL = `SELECT encrypted_password FROM public.project_credentials WHERE role_name = $1`

func (c PoolCatalog) EncryptedPassword(ctx context.Context, role string) (string, error) {
	if err := ValidateIdentifier(role); err != nil {
		return "", err
	}
	conn, connectCtx, closeConn, err := c.connectTarget(ctx, vaultDatabase)
	if err != nil {
		return "", ErrUnavailable
	}
	defer closeConn()
	var token string
	if err := conn.QueryRow(connectCtx, encryptedPasswordSQL, role).Scan(&token); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", ErrUnavailable
	}
	if token == "" {
		return "", ErrNotFound
	}
	return token, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCatalogRow(scanner rowScanner) (CatalogRow, error) {
	var row CatalogRow
	if err := scanner.Scan(
		&row.Name,
		&row.Owner,
		&row.SizeBytes,
		&row.SizePretty,
		&row.Collation,
		&row.CType,
		&row.LocaleProvider,
		&row.Locale,
		&row.AllowConn,
		&row.IsTemplate,
		&row.ConnectionCount,
		&row.PublicCanConnect,
		&row.OwnerIsSuperuser,
		&row.OwnerCanLogin,
		&row.OwnerCreatedb,
		&row.OwnerCreaterole,
		&row.OwnerReplication,
	); err != nil {
		return CatalogRow{}, err
	}
	return row, nil
}
