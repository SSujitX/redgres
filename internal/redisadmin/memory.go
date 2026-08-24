package redisadmin

import "context"

type MemoryClient struct {
	PingErr    error
	InfoErr    error
	InfoText   string
	DBSizeErr  error
	Size       int64
	ACLListErr error
	ACLLines   []string
}

func (m *MemoryClient) Ping(context.Context) error {
	if m.PingErr != nil {
		return m.PingErr
	}
	return nil
}

func (m *MemoryClient) Info(context.Context) (string, error) {
	if m.InfoErr != nil {
		return "", m.InfoErr
	}
	return m.InfoText, nil
}

func (m *MemoryClient) DBSize(context.Context) (int64, error) {
	if m.DBSizeErr != nil {
		return 0, m.DBSizeErr
	}
	return m.Size, nil
}

func (m *MemoryClient) ACLList(context.Context) ([]string, error) {
	if m.ACLListErr != nil {
		return nil, m.ACLListErr
	}
	if m.ACLLines == nil {
		return []string{}, nil
	}
	return m.ACLLines, nil
}
