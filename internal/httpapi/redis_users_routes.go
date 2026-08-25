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
	Preset     string `json:"preset"`
	QueueKind  string `json:"queue_kind"`
}

type redisPatchRequest struct {
	KeyPattern string   `json:"key_pattern"`
	Preset     string   `json:"preset"`
	QueueKind  string   `json:"queue_kind"`
	Commands   []string `json:"commands"`
}

type redisPresetsBody struct {
	Presets   []redisadmin.NamedPreset `json:"presets"`
	RequestID string                   `json:"request_id"`
}

type redisCommandsBody struct {
	Commands  []string `json:"commands"`
	RequestID string   `json:"request_id"`
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

type redisUserEnableResponse struct {
	User      redisUserDetail `json:"user"`
	RequestID string          `json:"request_id"`
}

type redisUserRotateResponse struct {
	Resource   redisCreateResource   `json:"resource"`
	User       redisUserDetail       `json:"user"`
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

func (s *Server) handleRedisPresets(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, http.StatusOK, redisPresetsBody{
		Presets:   redisadmin.NamedPresets(),
		RequestID: requestID(r),
	})
}

func (s *Server) handleRedisCommands(w http.ResponseWriter, r *http.Request) {
	commands := redisadmin.AllowedCommands()
	if commands == nil {
		commands = []string{}
	}
	s.writeJSON(w, r, http.StatusOK, redisCommandsBody{
		Commands:  commands,
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
	meta := redisCreateAuditMeta(body.Username, body.KeyPattern, body.Preset, body.QueueKind)
	if s.redis == nil {
		_ = s.audit.Record(sess.Username, "redis.user.create", body.Username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
		return
	}
	created, err := s.redis.CreateUser(ctx, body.Username, body.KeyPattern, body.Preset, body.QueueKind)
	if err != nil {
		_ = s.audit.Record(sess.Username, "redis.user.create", body.Username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeRedisCreateError(w, r, err)
		return
	}
	meta["username"] = created.User.Username
	meta["preset"] = created.User.Preset
	meta["key_pattern"] = created.User.KeyPattern
	delete(meta, "queue_kind")
	if created.User.Preset == redisadmin.PresetQueueWorker {
		meta["queue_kind"] = created.User.QueueKind
	}
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

func (s *Server) handleRedisUserPatch(w http.ResponseWriter, r *http.Request) {
	username, err := parseRedisUsernameParam(chi.URLParam(r, "username"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid username")
		return
	}
	var body redisPatchRequest
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), redisUsersTimeout)
	defer cancel()
	sess := sessionFrom(r)
	meta := redisUpdateAuditMeta(username, body.KeyPattern, body.Preset, body.QueueKind)
	if s.redis == nil {
		_ = s.audit.Record(sess.Username, "redis.user.update", username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
		return
	}
	user, err := s.redis.UpdatePermissions(ctx, username, body.KeyPattern, body.Preset, body.QueueKind, body.Commands)
	if err != nil {
		_ = s.audit.Record(sess.Username, "redis.user.update", username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeRedisUpdateError(w, r, err)
		return
	}
	meta["username"] = user.Username
	meta["preset"] = user.Preset
	meta["key_pattern"] = user.KeyPattern
	delete(meta, "queue_kind")
	if user.Preset == redisadmin.PresetQueueWorker {
		meta["queue_kind"] = user.QueueKind
	}
	if err := s.audit.Record(sess.Username, "redis.user.update", user.Username, "success", requestID(r), auth.ClientIP(r.RemoteAddr), meta); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, redisUserEnableResponse{
		User:      toRedisUserDetail(user),
		RequestID: requestID(r),
	})
}

func (s *Server) handleRedisUserRotate(w http.ResponseWriter, r *http.Request) {
	username, err := parseRedisUsernameParam(chi.URLParam(r, "username"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid username")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), redisUsersTimeout)
	defer cancel()
	sess := sessionFrom(r)
	meta := map[string]any{"username": username}
	if s.redis == nil {
		_ = s.audit.Record(sess.Username, "redis.user.rotate", username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
		return
	}
	rotated, err := s.redis.RotateUser(ctx, username)
	if err != nil {
		_ = s.audit.Record(sess.Username, "redis.user.rotate", username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeRedisEnableError(w, r, err)
		return
	}
	cred := redisCreateCredential{
		Username: rotated.User.Username,
		Password: rotated.Password,
		OneTime:  true,
	}
	if s.cfg.RedisPublicHost != "" && s.cfg.RedisPublicPort != "" {
		primary, urlErr := redisadmin.ProjectConnectionURL(s.cfg.RedisPublicHost, s.cfg.RedisPublicPort, rotated.User.Username, rotated.Password)
		if urlErr != nil {
			_ = s.audit.Record(sess.Username, "redis.user.rotate", rotated.User.Username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
			return
		}
		cred.URLs = &redisCreateURLs{Primary: primary}
	}
	if err := s.audit.Record(sess.Username, "redis.user.rotate", rotated.User.Username, "success", requestID(r), auth.ClientIP(r.RemoteAddr), meta); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, redisUserRotateResponse{
		Resource:   redisCreateResource{Type: "redis_user", Name: rotated.User.Username},
		User:       toRedisUserDetail(rotated.User),
		Credential: cred,
		RequestID:  requestID(r),
	})
}

func (s *Server) handleRedisUserEnable(w http.ResponseWriter, r *http.Request) {
	s.setRedisUserEnabled(w, r, true)
}

func (s *Server) handleRedisUserDisable(w http.ResponseWriter, r *http.Request) {
	s.setRedisUserEnabled(w, r, false)
}

func (s *Server) setRedisUserEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	username, err := parseRedisUsernameParam(chi.URLParam(r, "username"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid username")
		return
	}
	action := "redis.user.disable"
	if enabled {
		action = "redis.user.enable"
	}
	ctx, cancel := context.WithTimeout(r.Context(), redisUsersTimeout)
	defer cancel()
	sess := sessionFrom(r)
	meta := map[string]any{"username": username}
	if s.redis == nil {
		_ = s.audit.Record(sess.Username, action, username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
		return
	}
	user, err := s.redis.SetEnabled(ctx, username, enabled)
	if err != nil {
		_ = s.audit.Record(sess.Username, action, username, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeRedisEnableError(w, r, err)
		return
	}
	if err := s.audit.Record(sess.Username, action, username, "success", requestID(r), auth.ClientIP(r.RemoteAddr), meta); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, redisUserEnableResponse{
		User:      toRedisUserDetail(user),
		RequestID: requestID(r),
	})
}

func (s *Server) writeRedisEnableError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, redisadmin.ErrProtectedUser):
		s.writeError(w, r, http.StatusForbidden, CodeProtectedResource, "This Redis user is protected")
	case errors.Is(err, redisadmin.ErrNotFound):
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
	case errors.Is(err, redisadmin.ErrInvalidUsername):
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid username")
	default:
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
	}
}

func (s *Server) writeRedisUpdateError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, redisadmin.ErrInvalidPrefix):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid key pattern", map[string]string{"key_pattern": "invalid"})
	case errors.Is(err, redisadmin.ErrInvalidPreset):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid preset", map[string]string{"preset": "invalid"})
	case errors.Is(err, redisadmin.ErrInvalidQueueKind):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid queue kind", map[string]string{"queue_kind": "invalid"})
	case errors.Is(err, redisadmin.ErrInvalidCommands):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid commands", map[string]string{"commands": "invalid"})
	case errors.Is(err, redisadmin.ErrProtectedUser):
		s.writeError(w, r, http.StatusForbidden, CodeProtectedResource, "This Redis user is protected")
	case errors.Is(err, redisadmin.ErrNotFound):
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
	case errors.Is(err, redisadmin.ErrInvalidUsername):
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid username")
	default:
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
	}
}

func redisUpdateAuditMeta(username, keyPattern, preset, queueKind string) map[string]any {
	meta := map[string]any{"username": username}
	if keyPattern != "" {
		meta["key_pattern"] = keyPattern
	}
	switch preset {
	case redisadmin.PresetCacheReadWrite, redisadmin.PresetReadOnly, redisadmin.PresetCustom:
		if queueKind == "" {
			meta["preset"] = preset
		}
	case redisadmin.PresetQueueWorker:
		meta["preset"] = preset
		switch queueKind {
		case redisadmin.QueueLists, redisadmin.QueueStreams, redisadmin.QueueSortedSets:
			meta["queue_kind"] = queueKind
		}
	}
	return meta
}

func (s *Server) writeRedisCreateError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, redisadmin.ErrInvalidUsername):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid username", map[string]string{"username": "invalid"})
	case errors.Is(err, redisadmin.ErrInvalidPrefix):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid key pattern", map[string]string{"key_pattern": "invalid"})
	case errors.Is(err, redisadmin.ErrInvalidPreset):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid preset", map[string]string{"preset": "invalid"})
	case errors.Is(err, redisadmin.ErrInvalidQueueKind):
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid queue kind", map[string]string{"queue_kind": "invalid"})
	case errors.Is(err, redisadmin.ErrProtectedUser):
		s.writeError(w, r, http.StatusForbidden, CodeProtectedResource, "This Redis user is protected")
	case errors.Is(err, redisadmin.ErrConflict):
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Redis user already exists")
	default:
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, redisUnavailableMessage)
	}
}

func redisCreateAuditMeta(username, keyPattern, preset, queueKind string) map[string]any {
	meta := map[string]any{
		"username":    username,
		"key_pattern": keyPattern,
	}
	if preset == "" {
		preset = redisadmin.PresetCacheReadWrite
	}
	switch preset {
	case redisadmin.PresetCacheReadWrite, redisadmin.PresetReadOnly:
		if queueKind == "" {
			meta["preset"] = preset
		}
	case redisadmin.PresetQueueWorker:
		switch queueKind {
		case redisadmin.QueueLists, redisadmin.QueueStreams, redisadmin.QueueSortedSets:
			meta["preset"] = preset
			meta["queue_kind"] = queueKind
		}
	}
	return meta
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
