package postgresadmin

import (
	"context"
	"errors"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
)

func TestSystemIdentifierSQLMatchesControlSystem(t *testing.T) {
	if systemIdentifierSQL != `SELECT system_identifier FROM pg_control_system()` {
		t.Fatalf("sql = %s", systemIdentifierSQL)
	}
}

func TestFormatSystemIdentifierDecimalString(t *testing.T) {
	cases := []struct {
		id   int64
		want string
	}{
		{id: 0, want: "0"},
		{id: 7439123456789012345, want: "7439123456789012345"},
		{id: 9223372036854775807, want: "9223372036854775807"},
	}
	for _, tc := range cases {
		if got := formatSystemIdentifier(tc.id); got != tc.want {
			t.Fatalf("formatSystemIdentifier(%d) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestPoolCatalogSystemIdentifierNilPoolUnavailable(t *testing.T) {
	id, err := (PoolCatalog{}).SystemIdentifier(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if id != "" {
		t.Fatalf("id = %q", id)
	}
}

func TestMemoryCatalogSystemIdentifierValueAndErr(t *testing.T) {
	cat := &MemoryCatalog{SystemIdentifierValue: "7439123456789012345"}
	got, err := cat.SystemIdentifier(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "7439123456789012345" {
		t.Fatalf("got %q", got)
	}
	cat.SystemIdentifierErr = ErrUnavailable
	if _, err := cat.SystemIdentifier(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestServiceSystemIdentifierDelegates(t *testing.T) {
	cat := &MemoryCatalog{SystemIdentifierValue: "7439123456789012345"}
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.SystemIdentifier(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "7439123456789012345" {
		t.Fatalf("got %q", got)
	}
	if _, err := (*Service)(nil).SystemIdentifier(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil service err = %v", err)
	}
	empty := NewService(nil, NewPolicy(config.Config{}))
	if _, err := empty.SystemIdentifier(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil catalog err = %v", err)
	}
}
