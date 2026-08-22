package auth

import (
	"database/sql"
	"errors"
	"time"
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

	var existingID int64
	err = db.QueryRow(`SELECT id FROM owners LIMIT 1`).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Owner{}, err
	}
	if err == nil {
		if !replace {
			return Owner{}, ErrOwnerExists
		}
		if _, err := db.Exec(`UPDATE owners SET username = ?, password_hash = ? WHERE id = ?`, username, []byte(hash), existingID); err != nil {
			return Owner{}, err
		}
		if err := DeleteOwnerSessions(db, existingID); err != nil {
			return Owner{}, err
		}
		return GetOwner(db)
	}

	res, err := db.Exec(
		`INSERT INTO owners (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, []byte(hash), now,
	)
	if err != nil {
		return Owner{}, err
	}
	id, _ := res.LastInsertId()
	return Owner{ID: id, Username: username, PasswordHash: hash, CreatedAt: time.Now().UTC()}, nil
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
