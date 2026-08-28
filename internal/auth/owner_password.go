package auth

import (
	"database/sql"
	"errors"
	"time"

	"github.com/SSujitX/redgres/internal/audit"
)

// ErrSamePassword is returned when the new password equals the current password.
var ErrSamePassword = errors.New("new password must differ from the current password")

// ChangeOwnerPassword verifies the current password (rate-limited), validates
// and hashes the new password, stores it, and invalidates every owner session.
// The hash update, session deletion, and the success audit event are written in
// one transaction, so an audit failure rolls the password change back instead of
// leaving a half-applied change. It returns ErrReauthRequired on a wrong current
// password, ErrSamePassword when the new password equals the current one,
// ErrWeakPassword/ErrPasswordTooLong when the new password is rejected, and a
// RateLimitError (wrapping ErrRateLimited) when reauthentication is throttled.
// The new password is never logged or returned.
func ChangeOwnerPassword(db *sql.DB, username, currentPassword, newPassword, clientIP, requestID string, now time.Time) error {
	if err := Reauthenticate(db, username, currentPassword, clientIP, now); err != nil {
		return err
	}
	owner, err := LookupOwnerByUsername(db, username)
	if err != nil {
		return err
	}
	if currentPassword == newPassword {
		return ErrSamePassword
	}
	if err := ValidatePassword(newPassword, owner.Username); err != nil {
		return err
	}
	hash, err := Hash(newPassword)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE owners SET password_hash = ? WHERE id = ?`, []byte(hash), owner.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE owner_id = ?`, owner.ID); err != nil {
		return err
	}
	if err := audit.RecordTx(tx, owner.Username, "owner.password_change", owner.Username, "success", requestID, clientIP, map[string]any{"username": owner.Username}); err != nil {
		return err
	}
	return tx.Commit()
}
