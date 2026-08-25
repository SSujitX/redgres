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
	remaining, err := store.ReauthLockoutRemaining(username, clientIP, now)
	if err != nil {
		return err
	}
	if remaining > 0 {
		return RateLimitError{Remaining: remaining}
	}

	owner, err := LookupOwnerByUsername(db, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			verifyUnknown(password)
			if recordErr := store.RecordReauth(username, clientIP, false, now); recordErr != nil {
				return recordErr
			}
			return ErrReauthRequired
		}
		return err
	}
	if err := Verify(owner.PasswordHash, password); err != nil {
		if errors.Is(err, ErrMismatchedHash) {
			if recordErr := store.RecordReauth(username, clientIP, false, now); recordErr != nil {
				return recordErr
			}
			return ErrReauthRequired
		}
		return err
	}
	if err := store.ClearReauthFailures(username, clientIP); err != nil {
		return err
	}
	if err := store.RecordReauth(username, clientIP, true, now); err != nil {
		return err
	}
	return nil
}
