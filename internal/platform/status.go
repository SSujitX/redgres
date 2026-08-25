package platform

import (
	"context"
	"errors"
	"time"
)

// ErrNotConfigured is returned by a PingFunc when the probe target is absent.
// Collect maps it to state not_configured rather than unavailable.
var ErrNotConfigured = errors.New("not configured")

type Component struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

type PingFunc func(ctx context.Context) error

func Collect(ctx context.Context, statePing, postgresPing, pgbouncerPing, redisPing PingFunc) []Component {
	return []Component{
		pingResult(ctx, "redgres_state", statePing),
		optionalPingResult(ctx, "postgres_direct", postgresPing),
		optionalPingResult(ctx, "pgbouncer", pgbouncerPing),
		optionalPingResult(ctx, "redis", redisPing),
		{ID: "tool_links", State: "not_configured"},
	}
}

func optionalPingResult(ctx context.Context, id string, ping PingFunc) Component {
	if ping == nil {
		return Component{ID: id, State: "not_configured"}
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := ping(pingCtx)
	if err == nil {
		return Component{ID: id, State: "ok"}
	}
	if errors.Is(err, ErrNotConfigured) {
		return Component{ID: id, State: "not_configured"}
	}
	return Component{ID: id, State: "unavailable", Reason: "unreachable"}
}

func pingResult(ctx context.Context, id string, ping PingFunc) Component {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := ping(pingCtx); err != nil {
		return Component{ID: id, State: "unavailable", Reason: "unreachable"}
	}
	return Component{ID: id, State: "ok"}
}
