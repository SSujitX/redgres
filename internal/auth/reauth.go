package auth

import (
	"database/sql"
	"errors"
)

var (
	ErrReauthRequired = errors.New("owner password is incorrect")
	ErrUnauthorized   = errors.New("unauthorized")
)

var verifyUnknown = VerifyUnknown

// Reauthenticate looks up the owner by session username and verifies password.
// Returns a distinct mismatch error mapped to 403 reauth_required. Never logs password.
func Reauthenticate(db *sql.DB, username, password string) error {
	owner, err := LookupOwnerByUsername(db, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			verifyUnknown(password)
			return ErrUnauthorized
		}
		return err
	}
	if err := Verify(owner.PasswordHash, password); err != nil {
		if errors.Is(err, ErrMismatchedHash) {
			return ErrReauthRequired
		}
		return err
	}
	return nil
}
