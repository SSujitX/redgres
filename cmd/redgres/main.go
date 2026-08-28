package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SSujitX/redgres/internal/audit"
	"github.com/SSujitX/redgres/internal/bootstrap"
	"github.com/SSujitX/redgres/internal/buildinfo"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/internal/httpapi"
	"github.com/SSujitX/redgres/internal/operations"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/redisadmin"
	"github.com/SSujitX/redgres/internal/web"
	"github.com/SSujitX/redgres/migrations"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "create-owner" {
		if err := createOwner(args[1:]); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		return
	}
	if len(args) > 0 && args[0] == "backup" {
		if err := runBackup(args[1:]); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		return
	}
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version" || args[0] == "-V") {
		os.Stdout.WriteString(buildinfo.Version + "\n")
		return
	}
	if len(args) > 0 && (args[0] == "serve" || args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		if args[0] != "serve" {
			os.Stderr.WriteString("usage: redgres [serve | create-owner | backup | version] [flags]\n")
			os.Exit(2)
		}
		args = args[1:]
	}
	if err := run(args); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}))
	db, err := database.Open(cfg.SQLitePath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := database.Migrate(db, migrations.FS); err != nil {
		return err
	}
	assets, closeAssets, err := web.Open(cfg.DevAssetDir)
	if err != nil {
		return err
	}
	defer closeAssets()

	pg, closePG, err := postgresadmin.Open(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer closePG()

	ops := operations.NewStore(db)
	if err := ops.Reconcile(context.Background(), postgresadmin.NewDuplicateProbe(pg), postgresadmin.NewDuplicateCompensator(pg), time.Now().UTC()); err != nil {
		return err
	}

	rd, closeRD, err := redisadmin.Open(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer closeRD()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go pollQueuedDuplicates(ctx, ops, pg, audit.Store{DB: db}, log)

	api := httpapi.New(cfg, db, assets, log, pg, rd)
	handler := api.Handler()

	srv := &http.Server{
		Addr:              cfg.Address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("address", cfg.Address))
		errCh <- srv.ListenAndServe()
	}()

	if cfg.BootstrapAddress != "" && !bootstrap.MarkerPresent(cfg.SQLitePath) {
		bootstrapLn := bootstrap.New(handler, cfg.BootstrapAddress, cfg.BootstrapTTL)
		bootstrapLn.SetSQLitePath(cfg.SQLitePath)
		defer bootstrapLn.Close()
		if err := bootstrapLn.Start(); err != nil {
			_ = srv.Close()
			return err
		}
		api.SetBootstrapCloser(bootstrapLn)
		log.Info("bootstrap listening", slog.String("address", bootstrapLn.Addr()), slog.Duration("ttl", cfg.BootstrapTTL))
	} else if cfg.BootstrapAddress != "" {
		log.Info("bootstrap skipped (already closed)")
	}

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		log.Info("shutdown complete")
		return nil
	}
}

func pollQueuedDuplicates(ctx context.Context, store operations.Store, pg *postgresadmin.Service, auditor postgresadmin.DuplicateAuditor, log *slog.Logger) {
	run := func() {
		if err := postgresadmin.RunQueuedDuplicates(ctx, store, pg, auditor); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			log.Error("duplicate worker did not finish")
		}
	}
	run()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
