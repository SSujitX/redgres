package postgresadmin

import "context"

type MemoryCatalog struct {
	Rows         []CatalogRow
	Err          error
	Tables       map[string][]TableItem
	TablesErr    error
	LastTablesDB string
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
