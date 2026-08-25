package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/SSujitX/redgres/internal/audit"
)

type Owner struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

func CreateOrReplaceOwner(db *sql.DB, username, password string, replace bool) (Owner, error) {
	if err := ValidateUsername(username); err != nil {
		return Owner{}, err
	}
	username = NormalizeUsername(username)
	if err := ValidatePassword(password, username); err != nil {
		return Owner{}, err
	}
	hash, err := Hash(password)
	if err != nil {
		return Owner{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := db.Begin()
	if err != nil {
		return Owner{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var existing Owner
	var existingCreated string
	err = tx.QueryRow(`SELECT id, username, created_at FROM owners LIMIT 1`).
		Scan(&existing.ID, &existing.Username, &existingCreated)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Owner{}, err
	}
	if err == nil {
		if !replace {
			return Owner{}, ErrOwnerExists
		}
		if _, err := tx.Exec(`UPDATE owners SET username = ?, password_hash = ? WHERE id = ?`, username, []byte(hash), existing.ID); err != nil {
			return Owner{}, err
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE owner_id = ?`, existing.ID); err != nil {
			return Owner{}, err
		}
		requestID, err := ownerAuditRequestID()
		if err != nil {
			return Owner{}, err
		}
		if err := audit.RecordTx(
			tx,
			existing.Username,
			"owner.replace",
			username,
			"success",
			requestID,
			"",
			map[string]any{"previous_username": existing.Username, "username": username},
		); err != nil {
			return Owner{}, err
		}
		if err := tx.Commit(); err != nil {
			return Owner{}, err
		}
		created, _ := time.Parse(time.RFC3339Nano, existingCreated)
		return Owner{ID: existing.ID, Username: username, PasswordHash: hash, CreatedAt: created}, nil
	}

	res, err := tx.Exec(
		`INSERT INTO owners (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, []byte(hash), now,
	)
	if err != nil {
		return Owner{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return Owner{}, err
	}
	return Owner{ID: id, Username: username, PasswordHash: hash, CreatedAt: time.Now().UTC()}, nil
}

func ownerAuditRequestID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func GetOwner(db *sql.DB) (Owner, error) {
	var o Owner
	var hash []byte
	var created string
	err := db.QueryRow(`SELECT id, username, password_hash, created_at FROM owners LIMIT 1`).
		Scan(&o.ID, &o.Username, &hash, &created)
	if err != nil {
		return Owner{}, err
	}
	o.PasswordHash = string(hash)
	o.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return o, nil
}

func LookupOwnerByUsername(db *sql.DB, username string) (Owner, error) {
	var o Owner
	var hash []byte
	var created string
	err := db.QueryRow(
		`SELECT id, username, password_hash, created_at FROM owners WHERE username = ?`,
		NormalizeUsername(username),
	).Scan(&o.ID, &o.Username, &hash, &created)
	if err != nil {
		return Owner{}, err
	}
	o.PasswordHash = string(hash)
	o.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return o, nil
}
