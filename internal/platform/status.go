package platform

import (
	"context"
	"errors"
	"time"

	"github.com/SSujitX/redgres/internal/postgresadmin"
)

type Component struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

type PingFunc func(ctx context.Context) error

func Collect(ctx context.Context, statePing, postgresPing PingFunc) []Component {
	return []Component{
		pingResult(ctx, "redgres_state", statePing),
		postgresResult(ctx, postgresPing),
		{ID: "pgbouncer", State: "not_implemented"},
		{ID: "redis", State: "not_implemented"},
		{ID: "tool_links", State: "not_configured"},
	}
}

func postgresResult(ctx context.Context, ping PingFunc) Component {
	if ping == nil {
		return Component{ID: "postgres_direct", State: "not_configured"}
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := ping(pingCtx)
	if err == nil {
		return Component{ID: "postgres_direct", State: "ok"}
	}
	if errors.Is(err, postgresadmin.ErrNotConfigured) {
		return Component{ID: "postgres_direct", State: "not_configured"}
	}
	return Component{ID: "postgres_direct", State: "unavailable", Reason: "unreachable"}
}

func pingResult(ctx context.Context, id string, ping PingFunc) Component {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := ping(pingCtx); err != nil {
		return Component{ID: id, State: "unavailable", Reason: "unreachable"}
	}
	return Component{ID: id, State: "ok"}
}
