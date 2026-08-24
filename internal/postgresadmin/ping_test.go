package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
)

func TestServicePingNilCatalogIsNotConfigured(t *testing.T) {
	svc := NewService(nil, NewPolicy(config.Config{}))
	if err := svc.Ping(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	var nilSvc *Service
	if err := nilSvc.Ping(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil service err = %v", err)
	}
}

func TestServicePingMapsMemoryCatalogPingErr(t *testing.T) {
	svc := NewService(&MemoryCatalog{PingErr: errors.New("password=canary-secret host=10.0.0.1")}, NewPolicy(config.Config{}))
	err := svc.Ping(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), "10.0.0.1") {
		t.Fatalf("leaked canary: %v", err)
	}
}
