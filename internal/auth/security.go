package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"math"
	"net"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	lockoutThreshold  = 5
	sprayThreshold    = 20
	lockoutWindow     = 15 * time.Minute
	maxLockout        = 15 * time.Minute
	maxStoredAttempts = 1000
	reauthIPPrefix    = "\x00reauth:"
)

var ErrRateLimited = errors.New("too many login attempts")

type RateLimitError struct {
	Remaining time.Duration
}

func (e RateLimitError) Error() string {
	return ErrRateLimited.Error()
}

func (e RateLimitError) Unwrap() error {
	return ErrRateLimited
}

func RateLimitRemaining(err error) time.Duration {
	var limited RateLimitError
	if errors.As(err, &limited) {
		return limited.Remaining
	}
	return 0
}

type AttemptStore struct {
	DB *sql.DB
}

func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func ValidateUsername(username string) error {
	normalized := NormalizeUsername(username)
	if normalized == "" {
		return ErrInvalidUsername
	}
	if utf8.RuneCountInString(normalized) > MaxUsernameRunes {
		return ErrInvalidUsername
	}
	for _, r := range normalized {
		if unicode.IsControl(r) {
			return ErrInvalidUsername
		}
	}
	return nil
}

func ClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func (s AttemptStore) Record(username, ip string, succeeded bool, now time.Time) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	cutoff := now.Add(-lockoutWindow).UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`DELETE FROM login_attempts WHERE attempted_at < ?`, cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO login_attempts (username, client_ip, succeeded, attempted_at) VALUES (?, ?, ?, ?)`,
		NormalizeUsername(username), ip, boolToInt(succeeded), now.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM login_attempts
		 WHERE id NOT IN (SELECT id FROM login_attempts ORDER BY id DESC LIMIT ?)`,
		maxStoredAttempts,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s AttemptStore) ClearFailures(username, ip string) error {
	_, err := s.DB.Exec(
		`DELETE FROM login_attempts WHERE username = ? AND client_ip = ? AND succeeded = 0`,
		NormalizeUsername(username), ip,
	)
	return err
}

func (s AttemptStore) LockoutRemaining(username, ip string, now time.Time) (time.Duration, error) {
	since := now.Add(-lockoutWindow).UTC().Format(time.RFC3339Nano)
	rows, err := s.DB.Query(
		`SELECT succeeded, attempted_at FROM login_attempts
		 WHERE username = ? AND client_ip = ? AND attempted_at >= ?
		 ORDER BY attempted_at ASC`,
		NormalizeUsername(username), ip, since,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	failures := 0
	var lastFailure time.Time
	for rows.Next() {
		var succeeded int
		var attempted string
		if err := rows.Scan(&succeeded, &attempted); err != nil {
			return 0, err
		}
		at, err := time.Parse(time.RFC3339Nano, attempted)
		if err != nil {
			return 0, err
		}
		if succeeded == 1 {
			failures = 0
			lastFailure = time.Time{}
			continue
		}
		failures++
		lastFailure = at
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if failures < lockoutThreshold || lastFailure.IsZero() {
		return 0, nil
	}
	exp := failures - lockoutThreshold
	d := time.Duration(math.Pow(2, float64(exp))) * time.Second
	if d > maxLockout {
		d = maxLockout
	}
	until := lastFailure.Add(d)
	if now.Before(until) {
		return until.Sub(now), nil
	}
	return 0, nil
}

func (s AttemptStore) IPLockoutRemaining(ip string, now time.Time) (time.Duration, error) {
	since := now.Add(-lockoutWindow).UTC().Format(time.RFC3339Nano)
	var failures int
	var last string
	err := s.DB.QueryRow(
		`SELECT COUNT(*), COALESCE(MAX(attempted_at), '')
		 FROM login_attempts
		 WHERE client_ip = ? AND succeeded = 0 AND attempted_at >= ?`,
		ip, since,
	).Scan(&failures, &last)
	if err != nil {
		return 0, err
	}
	if failures < sprayThreshold || last == "" {
		return 0, nil
	}
	lastFailure, err := time.Parse(time.RFC3339Nano, last)
	if err != nil {
		return 0, err
	}
	until := lastFailure.Add(maxLockout)
	if now.Before(until) {
		return until.Sub(now), nil
	}
	return 0, nil
}

func (s AttemptStore) RecordReauth(username, ip string, succeeded bool, now time.Time) error {
	return s.Record(username, reauthIPPrefix+ip, succeeded, now)
}

func (s AttemptStore) ClearReauthFailures(username, ip string) error {
	return s.ClearFailures(username, reauthIPPrefix+ip)
}

func (s AttemptStore) ReauthLockoutRemaining(username, ip string, now time.Time) (time.Duration, error) {
	return s.LockoutRemaining(username, reauthIPPrefix+ip, now)
}

func CSRFValid(storedHash []byte, raw string) bool {
	if raw == "" || len(storedHash) == 0 {
		return false
	}
	sum := sha256.Sum256([]byte(raw))
	return subtle.ConstantTimeCompare(storedHash, sum[:]) == 1
}

func SameOrigin(origin, referer, baseURL string) bool {
	want := normalizeOrigin(baseURL)
	if want == "" {
		return false
	}
	if origin != "" {
		return normalizeOrigin(origin) == want
	}
	if referer != "" {
		return normalizeOrigin(referer) == want
	}
	return false
}

func normalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			rest = rest[:slash]
		}
		scheme := strings.ToLower(raw[:i])
		return scheme + "://" + strings.ToLower(rest)
	}
	return strings.ToLower(raw)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
