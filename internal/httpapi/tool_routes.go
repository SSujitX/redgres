package httpapi

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/SSujitX/redgres/internal/securefile"
	"github.com/SSujitX/redgres/internal/toolgate"
	"github.com/go-chi/chi/v5"
)

const maxPgAdminPasswordBytes = 4096

func (s *Server) handleToolLaunch(w http.ResponseWriter, r *http.Request) {
	tool := chi.URLParam(r, "tool")
	if !toolgate.ValidTool(tool) {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
		return
	}
	public := s.toolPublicURL(tool)
	if public == "" || s.tools == nil {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Expert tool is not configured")
		return
	}
	ticket, err := s.tools.Issue(tool)
	if err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "tools.launch", tool, "success", requestID(r), requestClientIP(r), map[string]any{"tool": tool}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	launch := strings.TrimRight(public, "/") + toolgate.LaunchPath + "?ticket=" + url.QueryEscape(ticket)
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"launch_url": launch,
		"request_id": requestID(r),
	})
}

func (s *Server) handlePgAdminReveal(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(s.cfg.PgAdminEmail)
	path := strings.TrimSpace(s.cfg.PgAdminPasswordFile)
	if email == "" || path == "" {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "pgAdmin login is not configured")
		return
	}
	raw, err := readPgAdminPasswordFile(path)
	if err != nil || len(raw) == 0 {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "pgAdmin login is not configured")
		return
	}
	password := strings.TrimSpace(string(raw))
	if password == "" {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "pgAdmin login is not configured")
		return
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "tools.pgadmin.reveal", "pgadmin", "success", requestID(r), requestClientIP(r), map[string]any{"tool": "pgadmin"}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	body := map[string]any{
		"email":      email,
		"password":   password,
		"request_id": requestID(r),
	}
	if master := readPgAdminMasterPassword(s.cfg.PgAdminMasterPasswordFile); master != "" {
		body["master_password"] = master
	}
	s.writeJSON(w, r, http.StatusOK, body)
}

func readPgAdminMasterPassword(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	raw, err := readPgAdminPasswordFile(path)
	if err != nil || len(raw) == 0 {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func readPgAdminPasswordFile(path string) ([]byte, error) {
	file, err := securefile.OpenRegular(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxPgAdminPasswordBytes {
		return nil, os.ErrNotExist
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxPgAdminPasswordBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxPgAdminPasswordBytes {
		return nil, os.ErrNotExist
	}
	return raw, nil
}

func (s *Server) toolPublicURL(tool string) string {
	switch tool {
	case toolgate.ToolPgAdmin:
		return s.cfg.PgAdminURL
	case toolgate.ToolRedisInsight:
		return s.cfg.RedisInsightURL
	default:
		return ""
	}
}
