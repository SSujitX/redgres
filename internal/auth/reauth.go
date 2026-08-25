package auth

import (
	"database/sql"
	"errors"
	"time"
)

var ErrReauthRequired = errors.New("owner password is incorrect")

var verifyUnknown = VerifyUnknown

// Reauthenticate looks up the owner by session username and verifies password.
// Returns a distinct mismatch error mapped to 403 reauth_required. Never logs password.
func Reauthenticate(db *sql.DB, username, password, clientIP string, now time.Time) error {
	store := AttemptStore{DB: db}
	owner, err := LookupOwnerByUsername(db, username)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := store.ReserveReauthFailure(username, clientIP, now); err != nil {
			return err
		}
		verifyUnknown(password)
		return ErrReauthRequired
	}
	if err := store.ReserveReauthFailure(username, clientIP, now); err != nil {
		return err
	}
	if err := Verify(owner.PasswordHash, password); err != nil {
		if errors.Is(err, ErrMismatchedHash) {
			return ErrReauthRequired
		}
		return err
	}
	if err := store.RecordReauthSuccess(username, clientIP, now); err != nil {
		return err
	}
	return nil
}
