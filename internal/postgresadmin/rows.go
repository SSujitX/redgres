package postgresadmin

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const confirmBaseTableSQL = `
SELECT 1
FROM information_schema.tables
WHERE table_type = 'BASE TABLE'
  AND table_schema = $1
  AND table_name = $2
  AND table_schema NOT IN ('pg_catalog', 'information_schema')
`

const listColumnsSQL = `
SELECT column_name, data_type
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2
ORDER BY ordinal_position
`

type columnMeta struct {
	Name     string
	DataType string
}

func (c PoolCatalog) ListRows(ctx context.Context, database, schema, table, q string, offset, limit int) (RowPage, error) {
	if err := ValidateIdentifier(database); err != nil {
		return RowPage{}, err
	}
	if err := ValidateIdentifier(schema); err != nil {
		return RowPage{}, err
	}
	if err := ValidateIdentifier(table); err != nil {
		return RowPage{}, err
	}
	if catalogSchemaDenied(schema) {
		return RowPage{}, ErrNotFound
	}
	conn, connectCtx, closeConn, err := c.connectTarget(ctx, database)
	if err != nil {
		return RowPage{}, err
	}
	defer closeConn()
	var exists int
	if err := conn.QueryRow(connectCtx, confirmBaseTableSQL, schema, table).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RowPage{}, ErrNotFound
		}
		return RowPage{}, ErrUnavailable
	}
	cols, err := conn.Query(connectCtx, listColumnsSQL, schema, table)
	if err != nil {
		return RowPage{}, ErrUnavailable
	}
	var columns []columnMeta
	for cols.Next() {
		var col columnMeta
		if err := cols.Scan(&col.Name, &col.DataType); err != nil {
			cols.Close()
			return RowPage{}, ErrUnavailable
		}
		columns = append(columns, col)
	}
	if err := cols.Err(); err != nil {
		cols.Close()
		return RowPage{}, ErrUnavailable
	}
	cols.Close()
	if len(columns) == 0 {
		return RowPage{}, ErrNotFound
	}
	relation := pgx.Identifier{schema, table}.Sanitize()
	quoted := make([]string, len(columns))
	names := make([]string, len(columns))
	for i, col := range columns {
		quoted[i] = pgx.Identifier{col.Name}.Sanitize()
		names[i] = col.Name
	}
	whereSQL, args := rowSearchClause(columns, q)
	countSQL := "SELECT count(*) FROM " + relation + whereSQL
	var total int64
	if err := conn.QueryRow(connectCtx, countSQL, args...).Scan(&total); err != nil {
		return RowPage{}, ErrUnavailable
	}
	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	dataSQL := "SELECT " + strings.Join(quoted, ", ") + " FROM " + relation + whereSQL +
		" ORDER BY " + quoted[0] + " LIMIT $" + strconv.Itoa(limitArg) + " OFFSET $" + strconv.Itoa(offsetArg)
	dataArgs := append(append([]any{}, args...), limit, offset)
	data, err := conn.Query(connectCtx, dataSQL, dataArgs...)
	if err != nil {
		return RowPage{}, ErrUnavailable
	}
	defer data.Close()
	outRows := make([]map[string]any, 0)
	for data.Next() {
		vals := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := data.Scan(ptrs...); err != nil {
			return RowPage{}, ErrUnavailable
		}
		row := make(map[string]any, len(columns))
		for i, col := range columns {
			cell, err := encodeCell(col.DataType, vals[i])
			if err != nil {
				return RowPage{}, ErrUnavailable
			}
			row[col.Name] = cell
		}
		outRows = append(outRows, row)
	}
	if data.Err() != nil {
		return RowPage{}, ErrUnavailable
	}
	return RowPage{Columns: names, Rows: outRows, Total: total, Offset: offset, Limit: limit}, nil
}

func rowSearchClause(columns []columnMeta, q string) (string, []any) {
	if q == "" {
		return "", nil
	}
	var ors []string
	var args []any
	for _, col := range columns {
		kind := strings.ToLower(col.DataType)
		if !strings.Contains(kind, "text") && !strings.Contains(kind, "character") && !strings.Contains(kind, "citext") {
			continue
		}
		args = append(args, "%"+q+"%")
		ors = append(ors, pgx.Identifier{col.Name}.Sanitize()+" ILIKE $"+strconv.Itoa(len(args)))
	}
	if len(ors) == 0 {
		return "", nil
	}
	return " WHERE (" + strings.Join(ors, " OR ") + ")", args
}
