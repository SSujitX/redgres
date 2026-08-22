package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

const (
	SessionTokenBytes = 32
	CSRFTokenBytes    = 32
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

type Session struct {
	ID                int64
	OwnerID           int64
	Username          string
	TokenHash         []byte
	CSRFHash          []byte
	CreatedAt         time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
}

type IssuedSession struct {
	Session
	RawToken string
	RawCSRF  string
}

func randomToken(n int) (string, []byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, err
	}
	raw := hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}

func HashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func CreateSession(db *sql.DB, ownerID int64, idle, absolute time.Duration, now time.Time) (IssuedSession, error) {
	if err := DeleteOwnerSessions(db, ownerID); err != nil {
		return IssuedSession{}, err
	}
	rawToken, tokenHash, err := randomToken(SessionTokenBytes)
	if err != nil {
		return IssuedSession{}, err
	}
	rawCSRF, csrfHash, err := randomToken(CSRFTokenBytes)
	if err != nil {
		return IssuedSession{}, err
	}
	created := now.UTC().Format(time.RFC3339Nano)
	idleAt := now.Add(idle).UTC().Format(time.RFC3339Nano)
	abs := now.Add(absolute).UTC().Format(time.RFC3339Nano)
	res, err := db.Exec(
		`INSERT INTO sessions (owner_id, token_hash, csrf_hash, idle_expires_at, absolute_expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ownerID, tokenHash, csrfHash, idleAt, abs, created,
	)
	if err != nil {
		return IssuedSession{}, err
	}
	id, _ := res.LastInsertId()
	return IssuedSession{
		Session: Session{
			ID:                id,
			OwnerID:           ownerID,
			TokenHash:         tokenHash,
			CSRFHash:          csrfHash,
			CreatedAt:         now.UTC(),
			IdleExpiresAt:     now.Add(idle).UTC(),
			AbsoluteExpiresAt: now.Add(absolute).UTC(),
		},
		RawToken: rawToken,
		RawCSRF:  rawCSRF,
	}, nil
}

func LookupSession(db *sql.DB, rawToken string, now time.Time) (Session, error) {
	if rawToken == "" {
		return Session{}, ErrSessionNotFound
	}
	hash := HashToken(rawToken)
	row := db.QueryRow(
		`SELECT s.id, s.owner_id, o.username, s.token_hash, s.csrf_hash, s.created_at, s.idle_expires_at, s.absolute_expires_at
		 FROM sessions s JOIN owners o ON o.id = s.owner_id
		 WHERE s.token_hash = ?`,
		hash,
	)
	var s Session
	var created, idleAt, abs string
	if err := row.Scan(&s.ID, &s.OwnerID, &s.Username, &s.TokenHash, &s.CSRFHash, &created, &idleAt, &abs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, err
	}
	var err error
	if s.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Session{}, err
	}
	if s.IdleExpiresAt, err = time.Parse(time.RFC3339Nano, idleAt); err != nil {
		return Session{}, err
	}
	if s.AbsoluteExpiresAt, err = time.Parse(time.RFC3339Nano, abs); err != nil {
		return Session{}, err
	}
	now = now.UTC()
	if now.After(s.IdleExpiresAt) || now.After(s.AbsoluteExpiresAt) {
		_, _ = db.Exec(`DELETE FROM sessions WHERE id = ?`, s.ID)
		return Session{}, ErrSessionExpired
	}
	return s, nil
}

func TouchSession(db *sql.DB, id int64, idle time.Duration, now time.Time) error {
	expires := now.Add(idle).UTC().Format(time.RFC3339Nano)
	_, err := db.Exec(`UPDATE sessions SET idle_expires_at = ? WHERE id = ?`, expires, id)
	return err
}

func RotateCSRF(db *sql.DB, id int64) (string, error) {
	raw, hash, err := randomToken(CSRFTokenBytes)
	if err != nil {
		return "", err
	}
	if _, err := db.Exec(`UPDATE sessions SET csrf_hash = ? WHERE id = ?`, hash, id); err != nil {
		return "", err
	}
	return raw, nil
}

func DeleteSession(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func DeleteOwnerSessions(db *sql.DB, ownerID int64) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE owner_id = ?`, ownerID)
	return err
}

func RawTokenStored(db *sql.DB, raw string) (bool, error) {
	rows, err := db.Query(`SELECT token_hash, csrf_hash FROM sessions`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var tokenHash, csrfHash []byte
		if err := rows.Scan(&tokenHash, &csrfHash); err != nil {
			return false, err
		}
		if string(tokenHash) == raw || string(csrfHash) == raw {
			return true, nil
		}
	}
	return false, rows.Err()
}
