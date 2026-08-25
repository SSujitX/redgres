package postgresadmin

import "context"

type MemoryTable struct {
	Columns []string
	Rows    []map[string]any
}

type MemoryCatalog struct {
	Rows           []CatalogRow
	Err            error
	PingErr        error
	Tables         map[string][]TableItem
	TablesErr      error
	LastTablesDB   string
	TableData      map[string]MemoryTable
	RowsErr        error
	LastRowsKey    string
	Connections    []ConnectionGroup
	ConnectionsErr error
}

func (m *MemoryCatalog) Ping(context.Context) error {
	if m.PingErr != nil {
		return m.PingErr
	}
	return nil
}

func (m *MemoryCatalog) List(context.Context) ([]CatalogRow, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([]CatalogRow, len(m.Rows))
	copy(out, m.Rows)
	return out, nil
}

func (m *MemoryCatalog) Lookup(_ context.Context, name string) (CatalogRow, error) {
	if m.Err != nil {
		return CatalogRow{}, m.Err
	}
	for _, row := range m.Rows {
		if row.Name == name {
			return row, nil
		}
	}
	return CatalogRow{}, ErrNotFound
}

func (m *MemoryCatalog) ListTables(_ context.Context, database string) ([]TableItem, error) {
	if err := ValidateIdentifier(database); err != nil {
		return nil, err
	}
	m.LastTablesDB = database
	if m.TablesErr != nil {
		return nil, m.TablesErr
	}
	if m.Err != nil {
		return nil, m.Err
	}
	items := m.Tables[database]
	out := make([]TableItem, len(items))
	copy(out, items)
	return out, nil
}

func memoryTableKey(database, schema, table string) string {
	return database + "." + schema + "." + table
}

func (m *MemoryCatalog) ListRows(_ context.Context, database, schema, table, _ string, offset, limit int) (RowPage, error) {
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
	key := memoryTableKey(database, schema, table)
	m.LastRowsKey = key
	if m.RowsErr != nil {
		return RowPage{}, m.RowsErr
	}
	if m.Err != nil {
		return RowPage{}, m.Err
	}
	data, ok := m.TableData[key]
	if !ok || len(data.Columns) == 0 {
		return RowPage{}, ErrNotFound
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultRowLimit
	}
	total := int64(len(data.Rows))
	if offset > len(data.Rows) {
		return RowPage{Columns: append([]string{}, data.Columns...), Rows: []map[string]any{}, Total: total, Offset: offset, Limit: limit}, nil
	}
	end := offset + limit
	if end > len(data.Rows) {
		end = len(data.Rows)
	}
	page := data.Rows[offset:end]
	out := make([]map[string]any, len(page))
	copy(out, page)
	return RowPage{
		Columns: append([]string{}, data.Columns...),
		Rows:    out,
		Total:   total,
		Offset:  offset,
		Limit:   limit,
	}, nil
}

func (m *MemoryCatalog) ListConnectionGroups(context.Context) ([]ConnectionGroup, error) {
	if m.ConnectionsErr != nil {
		return nil, m.ConnectionsErr
	}
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([]ConnectionGroup, len(m.Connections))
	copy(out, m.Connections)
	return out, nil
}
