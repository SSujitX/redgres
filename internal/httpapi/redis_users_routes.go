package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/redisadmin"
	"github.com/go-chi/chi/v5"
)

const redisUnavailableMessage = "Redis is unavailable"

const redisUsersTimeout = 2 * time.Second

type redisUserSummary struct {
	Username     string `json:"username"`
	Enabled      bool   `json:"enabled"`
	KeyPattern   string `json:"key_pattern"`
	Preset       string `json:"preset"`
	QueueKind    string `json:"queue_kind,omitempty"`
	Protected    bool   `json:"protected"`
	RuleFidelity string `json:"rule_fidelity"`
}

type redisUserDetail struct {
	Username     string   `json:"username"`
	Enabled      bool     `json:"enabled"`
	KeyPattern   string   `json:"key_pattern"`
	Preset       string   `json:"preset"`
	QueueKind    string   `json:"queue_kind,omitempty"`
	Protected    bool     `json:"protected"`
	RuleFidelity string   `json:"rule_fidelity"`
	Commands     []string `json:"commands"`
	Categories   []string `json:"categories"`
}

type redisUsersListBody struct {
	State     string              `json:"state"`
	Users     *[]redisUserSummary `json:"users,omitempty"`
	Truncated *bool               `json:"truncated,omitempty"`
	Reason    string              `json:"reason,omitempty"`
	RequestID string              `json:"request_id"`
}

type redisUserBody struct {
	State     string           `json:"state"`
	User      *redisUserDetail `json:"user,omitempty"`
	Reason    string           `json:"reason,omitempty"`
	RequestID string           `json:"request_id"`
}

type redisCreateRequest struct {
	Username   string `json:"username"`
	KeyPattern string `json:"key_pattern"`
}

type redisCreateResource struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type redisCreateURLs struct {
	Primary string `json:"primary"`
}

type redisCreateCredential struct {
	Username string           `json:"username"`
	Password string           `json:"password"`
	URLs     *redisCreateURLs `json:"urls,omitempty"`
	OneTime  bool             `json:"one_time"`
}

type redisUserCreateResponse struct {
	Resource   redisCreateResource   `json:"resource"`
	User       redisUserSummary      `json:"user"`
	Credential redisCreateCredential `json:"credential"`
	RequestID  string                `json:"request_id"`
}

func (s *Server) handleRedisUsers(w http.ResponseWriter, r *http.Request) {
	if s.redis == nil {
		empty := []redisUserSummary{}
		s.writeJSON(w, r, http.StatusOK, redisUsersListBody{
			State:     "not_configured",
			Users:     &empty,
			RequestID: requestID(r),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), redisUsersTimeout)
	defer cancel()
	list, err := s.redis.ListUsers(ctx)
	if errors.Is(err, redisadmin.ErrNotConfigured) {
		empty := []redisUserSummary{}
		s.writeJSON(w, r, http.StatusOK, redisUsersListBody{
			State:     "not_configured",
			Users:     &empty,
			RequestID: requestID(r),
		})
		return
	}
	if err != nil {
		s.writeJSON(w, r, http.StatusOK, redisUsersListBody{
			State:     "unavailable",
			Reason:    redisFailureReason(err),
			RequestID: requestID(r),
		})
		return
	}
	summaries := make([]redisUserSummary, 0, len(list.Users))
	for _, u := range list.Users {
		summaries = append(summaries, toRedisUserSummary(u))
	}
	truncated := list.Truncated
	s.writeJSON(w, r, http.StatusOK, redisUsersListBody{
		State:     "ok",
		Users:     &summaries,
		Truncated: &truncated,
		RequestID: requestID(r),
	})
}

func (s *Server) handleRedisUsersCreate(w http.ResponseWriter, r *http.Request) {
	var body redisCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), redisUsersTimeout)
	defer cancel()
	sess := sessionFrom(r)
	meta := map[string]any{
		"username":    body.Username,
		"preset":      redisadmin.PresetCacheReadWrite,
		"key_pattern": body.KeyPattern,
	}
	if s.redis == nil {
		_ = s.audit.Record(sess.Username, "redis.user.create", body.Username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
		return
	}
	created, err := s.redis.CreateUser(ctx, body.Username, body.KeyPattern)
	if err != nil {
		_ = s.audit.Record(sess.Username, "redis.user.create", body.Username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeRedisCreateError(w, r, err)
		return
	}
	meta["username"] = created.User.Username
	meta["key_pattern"] = created.User.KeyPattern
	cred := redisCreateCredential{
		Username: created.User.Username,
		Password: created.Password,
		OneTime:  true,
	}
	if s.cfg.RedisPublicHost != "" && s.cfg.RedisPublicPort != "" {
		primary, urlErr := redisadmin.ProjectConnectionURL(s.cfg.RedisPublicHost, s.cfg.RedisPublicPort, created.User.Username, created.Password)
		if urlErr != nil {
			_ = s.audit.Record(sess.Username, "redis.user.create", created.User.Username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
			return
		}
		cred.URLs = &redisCreateURLs{Primary: primary}
	}
	if err := s.audit.Record(sess.Username, "redis.user.create", created.User.Username, "success", requestID(r), auth.ClientIP(r.RemoteAddr), meta); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusCreated, redisUserCreateResponse{
		Resource:   redisCreateResource{Type: "redis_user", Name: created.User.Username},
		User:       toRedisUserSummary(created.User),
		Credential: cred,
		RequestID:  requestID(r),
	})
}

func (s *Server) writeRedisCreateError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, redisadmin.ErrInvalidUsername):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid username", map[string]string{"username": "invalid"})
	case errors.Is(err, redisadmin.ErrInvalidPrefix):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid key pattern", map[string]string{"key_pattern": "invalid"})
	case errors.Is(err, redisadmin.ErrProtectedUser):
		s.writeError(w, r, http.StatusForbidden, CodeProtectedResource, "This Redis user is protected")
	case errors.Is(err, redisadmin.ErrConflict):
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Redis user already exists")
	default:
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
	}
}

func (s *Server) handleRedisUser(w http.ResponseWriter, r *http.Request) {
	username, err := parseRedisUsernameParam(chi.URLParam(r, "username"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid username")
		return
	}
	if s.redis == nil {
		s.writeJSON(w, r, http.StatusOK, redisUserBody{State: "not_configured", RequestID: requestID(r)})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), redisUsersTimeout)
	defer cancel()
	user, err := s.redis.GetUser(ctx, username)
	if errors.Is(err, redisadmin.ErrNotConfigured) {
		s.writeJSON(w, r, http.StatusOK, redisUserBody{State: "not_configured", RequestID: requestID(r)})
		return
	}
	if errors.Is(err, redisadmin.ErrNotFound) {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
		return
	}
	if err != nil {
		s.writeJSON(w, r, http.StatusOK, redisUserBody{
			State:     "unavailable",
			Reason:    redisFailureReason(err),
			RequestID: requestID(r),
		})
		return
	}
	detail := toRedisUserDetail(user)
	s.writeJSON(w, r, http.StatusOK, redisUserBody{
		State:     "ok",
		User:      &detail,
		RequestID: requestID(r),
	})
}

func redisFailureReason(err error) string {
	if errors.Is(err, redisadmin.ErrAuthFailed) {
		return "auth_failed"
	}
	if errors.Is(err, redisadmin.ErrPermissionDenied) {
		return "permission_denied"
	}
	return "unreachable"
}

func toRedisUserSummary(u redisadmin.User) redisUserSummary {
	out := redisUserSummary{
		Username:     u.Username,
		Enabled:      u.Enabled,
		KeyPattern:   u.KeyPattern,
		Preset:       u.Preset,
		Protected:    u.Protected,
		RuleFidelity: u.RuleFidelity,
	}
	if u.Preset == redisadmin.PresetQueueWorker {
		out.QueueKind = u.QueueKind
	}
	return out
}

func toRedisUserDetail(u redisadmin.User) redisUserDetail {
	commands := u.Commands
	if commands == nil {
		commands = []string{}
	}
	categories := u.Categories
	if categories == nil {
		categories = []string{}
	}
	out := redisUserDetail{
		Username:     u.Username,
		Enabled:      u.Enabled,
		KeyPattern:   u.KeyPattern,
		Preset:       u.Preset,
		Protected:    u.Protected,
		RuleFidelity: u.RuleFidelity,
		Commands:     commands,
		Categories:   categories,
	}
	if u.Preset == redisadmin.PresetQueueWorker {
		out.QueueKind = u.QueueKind
	}
	return out
}

func parseRedisUsernameParam(raw string) (string, error) {
	name, err := url.PathUnescape(raw)
	if err != nil {
		return "", err
	}
	if name == "" || name == "/" || name == ".." || strings.Contains(name, "/") {
		return "", errors.New("invalid username")
	}
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 64 {
		return "", errors.New("invalid username")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", errors.New("invalid username")
		}
		if !isRedisUsernameRune(r) {
			return "", errors.New("invalid username")
		}
	}
	return name, nil
}

func isRedisUsernameRune(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
}
