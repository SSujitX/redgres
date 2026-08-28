package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/version"
)

const (
	sessionCookieName = "redgres_session"
	csrfHeader        = "X-CSRF-Token"

	loginGenericMessage  = "Invalid username or password."
	authRequiredMessage  = "Authentication required"
	originFailedMessage  = "Origin check failed"
	csrfInvalidMessage   = "CSRF token is invalid"
	storageUnavailable   = "Control-plane storage is unavailable"
	rateLimitedMessage   = "Too many login attempts. Try again later."
	reauthLimitedMessage = "Too many reauthentication attempts. Try again later."
)

type ctxSessionKey struct{}

var defaultCapabilities = []string{
	"platform.read", "platform.network", "audit.read",
	"postgres.read", "postgres.provision", "postgres.credentials", "postgres.destructive",
	"redis.read", "redis.provision", "redis.credentials", "redis.destructive",
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil || c.Value == "" {
			s.writeError(w, r, http.StatusUnauthorized, CodeUnauthorized, authRequiredMessage)
			return
		}
		sess, err := auth.LookupSession(s.db, c.Value, time.Now().UTC())
		if err != nil {
			if errors.Is(err, auth.ErrSessionNotFound) || errors.Is(err, auth.ErrSessionExpired) {
				s.writeError(w, r, http.StatusUnauthorized, CodeUnauthorized, authRequiredMessage)
				return
			}
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
			return
		}
		if err := auth.TouchSession(s.db, sess.ID, s.cfg.SessionTTL, time.Now().UTC()); err != nil {
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxSessionKey{}, sess)))
	})
}

func hasCapability(name string) bool {
	for _, item := range defaultCapabilities {
		if item == name {
			return true
		}
	}
	return false
}

func (s *Server) requireCapability(name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hasCapability(name) {
				s.writeError(w, r, http.StatusForbidden, CodeForbidden, "Forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) requireMutation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.SameOrigin(r.Header.Get("Origin"), r.Header.Get("Referer"), s.cfg.BaseURL) {
			s.writeError(w, r, http.StatusForbidden, CodeCSRFInvalid, originFailedMessage)
			return
		}
		sess := sessionFrom(r)
		if !auth.CSRFValid(sess.CSRFHash, r.Header.Get(csrfHeader)) {
			s.writeError(w, r, http.StatusForbidden, CodeCSRFInvalid, csrfInvalidMessage)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sessionFrom(r *http.Request) auth.Session {
	sess, _ := r.Context().Value(ctxSessionKey{}).(auth.Session)
	return sess
}

func requestClientIP(r *http.Request) string {
	cf := ""
	if vals := r.Header.Values("CF-Connecting-IP"); len(vals) == 1 {
		cf = vals[0]
	}
	return auth.EffectiveClientIP(r.RemoteAddr, cf)
}

func (s *Server) writeLoginRateLimited(w http.ResponseWriter, r *http.Request, remaining time.Duration, username, ip string) {
	w.Header().Set("Retry-After", strconv.Itoa(int(remaining.Seconds())+1))
	_ = s.audit.Record(username, "owner.login", username, "rate_limited", requestID(r), ip, map[string]any{"username": username})
	s.writeError(w, r, http.StatusTooManyRequests, CodeRateLimited, rateLimitedMessage)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !auth.SameOrigin(r.Header.Get("Origin"), r.Header.Get("Referer"), s.cfg.BaseURL) {
		s.writeError(w, r, http.StatusForbidden, CodeCSRFInvalid, originFailedMessage)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	username := auth.NormalizeUsername(body.Username)
	ip := requestClientIP(r)
	store := auth.AttemptStore{DB: s.db}
	now := time.Now().UTC()

	ipRemaining, err := store.IPLockoutRemaining(ip, now)
	if err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if ipRemaining > 0 {
		auditUsername := username
		if auth.ValidateUsername(username) != nil {
			auditUsername = "invalid_username"
		}
		s.writeLoginRateLimited(w, r, ipRemaining, auditUsername, ip)
		return
	}
	if err := auth.ValidateUsername(username); err != nil {
		if err := store.ReserveFailure("invalid_username", ip, now); err != nil {
			if errors.Is(err, auth.ErrRateLimited) {
				s.writeLoginRateLimited(w, r, auth.RateLimitRemaining(err), "invalid_username", ip)
				return
			}
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
			return
		}
		auth.VerifyUnknown(body.Password)
		_ = s.audit.Record("invalid_username", "owner.login", "invalid_username", "failure", requestID(r), ip, map[string]any{"username": "invalid_username"})
		s.writeError(w, r, http.StatusUnauthorized, CodeUnauthorized, loginGenericMessage)
		return
	}

	owner, lookupErr := auth.LookupOwnerByUsername(s.db, username)
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if err := store.ReserveFailure(username, ip, now); err != nil {
		if errors.Is(err, auth.ErrRateLimited) {
			s.writeLoginRateLimited(w, r, auth.RateLimitRemaining(err), username, ip)
			return
		}
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if errors.Is(lookupErr, sql.ErrNoRows) {
		auth.VerifyUnknown(body.Password)
		_ = s.audit.Record(username, "owner.login", username, "failure", requestID(r), ip, map[string]any{"username": username})
		s.writeError(w, r, http.StatusUnauthorized, CodeUnauthorized, loginGenericMessage)
		return
	}
	if err := auth.Verify(owner.PasswordHash, body.Password); err != nil {
		_ = s.audit.Record(owner.Username, "owner.login", username, "failure", requestID(r), ip, map[string]any{"username": username})
		s.writeError(w, r, http.StatusUnauthorized, CodeUnauthorized, loginGenericMessage)
		return
	}

	if err := store.RecordSuccess(username, ip, now); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	issued, err := auth.CreateSession(s.db, owner.ID, s.cfg.SessionTTL, s.cfg.AbsoluteSessionTTL, now)
	if err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if err := s.audit.Record(owner.Username, "owner.login", username, "success", requestID(r), ip, map[string]any{"username": username}); err != nil {
		_ = auth.DeleteSession(s.db, issued.ID)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.setSessionCookie(w, issued.RawToken, issued.AbsoluteExpiresAt)
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"owner":      map[string]any{"username": owner.Username},
		"csrf_token": issued.RawCSRF,
		"request_id": requestID(r),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "owner.logout", sess.Username, "success", requestID(r), requestClientIP(r), map[string]any{"username": sess.Username}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if err := auth.DeleteSession(s.db, sess.ID); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.clearSessionCookie(w)
	s.writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "request_id": requestID(r)})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	raw, err := auth.RotateCSRF(s.db, sess.ID)
	if err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"owner":        map[string]any{"username": sess.Username},
		"csrf_token":   raw,
		"capabilities": defaultCapabilities,
		"tool_links":   s.sessionToolLinks(),
		"version":      version.Version,
		"request_id":   requestID(r),
	})
}

func (s *Server) handleOwnerChangePassword(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}

	ip := requestClientIP(r)
	now := time.Now().UTC()
	reqID := requestID(r)

	err := auth.ChangeOwnerPassword(s.db, sess.Username, body.CurrentPassword, body.NewPassword, ip, reqID, now)
	if err != nil {
		if remaining := auth.RateLimitRemaining(err); remaining > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(remaining.Seconds())+1))
			_ = s.audit.Record(sess.Username, "owner.password_change", sess.Username, "rate_limited", reqID, ip, map[string]any{"username": sess.Username})
			s.writeError(w, r, http.StatusTooManyRequests, CodeRateLimited, reauthLimitedMessage)
			return
		}
		switch {
		case errors.Is(err, auth.ErrReauthRequired):
			_ = s.audit.Record(sess.Username, "owner.password_change", sess.Username, "failure", reqID, ip, map[string]any{"username": sess.Username})
			s.writeError(w, r, http.StatusForbidden, CodeReauthRequired, "Current password is incorrect")
		case errors.Is(err, auth.ErrSamePassword):
			_ = s.audit.Record(sess.Username, "owner.password_change", sess.Username, "failure", reqID, ip, map[string]any{"username": sess.Username})
			s.writeErrorFields(w, r, http.StatusUnprocessableEntity, CodeValidationError, "New password must differ from the current password", map[string]string{"new_password": "same_as_current"})
		case errors.Is(err, auth.ErrWeakPassword):
			_ = s.audit.Record(sess.Username, "owner.password_change", sess.Username, "failure", reqID, ip, map[string]any{"username": sess.Username})
			s.writeErrorFields(w, r, http.StatusUnprocessableEntity, CodeValidationError, "New password does not meet the strength policy", map[string]string{"new_password": "too_weak"})
		case errors.Is(err, auth.ErrPasswordTooLong):
			_ = s.audit.Record(sess.Username, "owner.password_change", sess.Username, "failure", reqID, ip, map[string]any{"username": sess.Username})
			s.writeErrorFields(w, r, http.StatusUnprocessableEntity, CodeValidationError, "New password is too long", map[string]string{"new_password": "too_long"})
		default:
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		}
		return
	}

	// Success: the audit was recorded in the same transaction as the hash update
	// and session deletion, so clear the cookie only after that commit succeeded.
	s.clearSessionCookie(w)
	s.writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "request_id": reqID})
}

func (s *Server) sessionToolLinks() map[string]string {
	links := map[string]string{}
	if s.cfg.PgAdminURL != "" {
		links["pgadmin"] = s.cfg.PgAdminURL
	}
	if s.cfg.RedisInsightURL != "" {
		links["redisinsight"] = s.cfg.RedisInsightURL
	}
	return links
}

func (s *Server) setSessionCookie(w http.ResponseWriter, raw string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  expires,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}
