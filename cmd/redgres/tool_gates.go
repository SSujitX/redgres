package main

import (
	"log/slog"
	"strings"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/toolgate"
)

func startToolGates(cfg config.Config, store *toolgate.Memory, origin *toolgate.Origin, log *slog.Logger) {
	if store == nil {
		return
	}
	startOne := func(tool, listen, upstream, remoteUser string) {
		if listen == "" || upstream == "" {
			return
		}
		gate, err := toolgate.NewGate(tool, upstream, store, cfg.CookieSecure, "")
		if err != nil {
			log.Error("tool gate skipped", slog.String("tool", tool))
			return
		}
		gate.UseOrigin(origin)
		gate.RemoteUser = strings.TrimSpace(strings.ToLower(remoteUser))
		go func() {
			log.Info("tool gate listening", slog.String("tool", tool), slog.String("address", listen))
			if err := toolgate.ListenAndServe(listen, gate); err != nil {
				log.Error("tool gate stopped", slog.String("tool", tool))
			}
		}()
	}
	startOne(toolgate.ToolPgAdmin, cfg.ToolGatePgAdminListen, cfg.ToolGatePgAdminUpstream, cfg.PgAdminEmail)
	startOne(toolgate.ToolRedisInsight, cfg.ToolGateRedisListen, cfg.ToolGateRedisUpstream, "")
}
