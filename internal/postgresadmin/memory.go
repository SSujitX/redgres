package postgresadmin

import "context"

type MemoryCatalog struct {
	Rows []CatalogRow
	Err  error
}

func (m MemoryCatalog) List(context.Context) ([]CatalogRow, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([]CatalogRow, len(m.Rows))
	copy(out, m.Rows)
	return out, nil
}

func (m MemoryCatalog) Lookup(_ context.Context, name string) (CatalogRow, error) {
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
