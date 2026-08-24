package httpapi

import (
	"database/sql"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/SSujitX/redgres/internal/audit"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg      config.Config
	db       *sql.DB
	assets   fs.FS
	log      *slog.Logger
	audit    audit.Store
	postgres postgresadmin.Inventory
}

func New(cfg config.Config, db *sql.DB, assets fs.FS, logger *slog.Logger, postgres postgresadmin.Inventory) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if assets == nil {
		assets = nopFS{}
	}
	return &Server{cfg: cfg, db: db, assets: assets, log: logger, audit: audit.Store{DB: db}, postgres: postgres}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.CleanPath)
	r.Use(s.withRequestID)
	r.Use(s.securityHeaders)
	r.Use(s.limitBody)
	r.Use(s.recoverer)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
			return
		}
		s.serveStatic(w, r)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		s.writeError(w, r, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed")
	})

	r.Get("/api/v1/healthz", s.handleHealthz)
	r.With(s.requireSession, s.requireCapability("platform.read")).Get("/api/v1/status", s.handleStatus)
	r.Post("/api/v1/auth/login", s.handleLogin)
	r.With(s.requireSession, s.requireMutation).Post("/api/v1/auth/logout", s.handleLogout)
	r.With(s.requireSession).Get("/api/v1/session", s.handleSession)
	r.With(s.requireSession, s.requireCapability("audit.read")).Get("/api/v1/audit", s.handleAuditEvents)
	r.With(s.requireSession, s.requireCapability("postgres.read")).Get("/api/v1/postgres/databases", s.handlePostgresDatabases)
	r.With(s.requireSession, s.requireCapability("postgres.read")).Get("/api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows", s.handlePostgresRows)
	r.With(s.requireSession, s.requireCapability("postgres.read")).Get("/api/v1/postgres/databases/{db}/tables", s.handlePostgresTables)
	r.With(s.requireSession, s.requireCapability("postgres.read")).Get("/api/v1/postgres/databases/{db}", s.handlePostgresDatabase)
	return r
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeError(w, r, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed")
		return
	}
	if !assetExists(s.assets, "index.html") {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "Frontend assets are unavailable")
		return
	}

	requestPath := strings.TrimPrefix(r.URL.Path, "/")
	serveName := "index.html"
	cacheControl := "no-store"
	if allowedStaticName(requestPath) && requestPath != "index.html" {
		if !assetExists(s.assets, requestPath) {
			s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
			return
		}
		serveName = requestPath
		cacheControl = "public, max-age=31536000, immutable"
	}

	f, err := s.assets.Open(serveName)
	if err != nil {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
		return
	}
	reader, ok := f.(io.ReadSeeker)
	if !ok {
		s.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Internal server error")
		return
	}
	w.Header().Set("Cache-Control", cacheControl)
	http.ServeContent(w, r, serveName, info.ModTime(), reader)
}

func allowedStaticName(name string) bool {
	if name == "" || !fs.ValidPath(name) {
		return false
	}
	if name == "index.html" {
		return true
	}
	return strings.HasPrefix(name, "assets/") && !strings.HasSuffix(name, "/")
}

func assetExists(fsys fs.FS, name string) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

type nopFS struct{}

func (nopFS) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}
