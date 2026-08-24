package redisadmin

import "context"

type MemoryClient struct {
	PingErr error
}

func (m *MemoryClient) Ping(context.Context) error {
	if m.PingErr != nil {
		return m.PingErr
	}
	return nil
}
