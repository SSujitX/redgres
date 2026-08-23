package postgresadmin

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/jackc/pgx/v5"
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

type PoolCatalog struct {
	pool *pgxpool.Pool
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
	pool, err := connectPool(ctx, cfg, password)
	password = ""
	if err != nil {
		return nil, noop, err
	}
	if err := checkServerMajor(ctx, pool, cfg.PostgresExpectedMajor); err != nil {
		pool.Close()
		return nil, noop, err
	}
	return NewService(PoolCatalog{pool: pool}, policy), pool.Close, nil
}

func connectPool(ctx context.Context, cfg config.Config, password string) (*pgxpool.Pool, error) {
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

func keywordConnInfo(cfg config.Config) string {
	parts := []string{
		"host=" + keywordValue(cfg.PostgresHost),
		"user=" + keywordValue(cfg.PostgresUser),
		"dbname=" + keywordValue(cfg.PostgresDatabase),
		"sslmode=" + keywordValue(cfg.PostgresSSLMode),
		"connect_timeout=10",
		"application_name=redgres",
	}
	if cfg.PostgresSSLRootCert != "" {
		parts = append(parts, "sslrootcert="+keywordValue(cfg.PostgresSSLRootCert))
	}
	return strings.Join(parts, " ")
}

func keywordValue(value string) string {
	if strings.ContainsAny(value, " \\'") {
		return "'" + strings.ReplaceAll(value, `'`, `\'`) + "'"
	}
	return value
}

func readPasswordFile(path string, production bool) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", errors.New("REDGRES_POSTGRES_PASSWORD_FILE: is unavailable")
	}
	if production && info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("REDGRES_POSTGRES_PASSWORD_FILE: must not be group or world accessible")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("REDGRES_POSTGRES_PASSWORD_FILE: is unavailable")
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if password == "" {
		return "", errors.New("REDGRES_POSTGRES_PASSWORD_FILE: is empty")
	}
	return password, nil
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
