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
	lockoutThreshold = 5
	lockoutWindow    = 15 * time.Minute
	maxLockout       = 15 * time.Minute
)

var ErrRateLimited = errors.New("too many login attempts")

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
	_, err := s.DB.Exec(
		`INSERT INTO login_attempts (username, client_ip, succeeded, attempted_at) VALUES (?, ?, ?, ?)`,
		NormalizeUsername(username), ip, boolToInt(succeeded), now.UTC().Format(time.RFC3339Nano),
	)
	return err
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
