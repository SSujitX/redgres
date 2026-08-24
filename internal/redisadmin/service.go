package redisadmin

import "context"

type Client interface {
	Ping(ctx context.Context) error
}

type Service struct {
	client Client
}

func NewService(client Client) *Service {
	return &Service{client: client}
}

func (s *Service) Ping(ctx context.Context) error {
	if s == nil || s.client == nil {
		return ErrNotConfigured
	}
	if err := s.client.Ping(ctx); err != nil {
		return ErrUnavailable
	}
	return nil
}
