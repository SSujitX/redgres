package postgresadmin

import (
	"context"
	"strconv"

	"github.com/SSujitX/redgres/internal/secrets"
)

const vaultUpsertAttempts = 3

const upsertCredentialSQL = `INSERT INTO public.project_credentials (role_name, encrypted_password, updated_at) VALUES ($1, $2, now()) ON CONFLICT (role_name) DO UPDATE SET encrypted_password = EXCLUDED.encrypted_password, updated_at = now()`

func formatAlterRolePassword(owner, password string) (string, error) {
	quotedOwner, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	return "ALTER ROLE " + quotedOwner + " WITH PASSWORD " + quoteStringLiteral(password) + " CONNECTION LIMIT " + strconv.Itoa(projectRoleConnectionLimit), nil
}

func (s *Service) Rotate(ctx context.Context, name string) (CreatedDatabase, error) {
	if err := ValidateIdentifier(name); err != nil {
		return CreatedDatabase{}, err
	}
	if s == nil || s.catalog == nil {
		return CreatedDatabase{}, ErrUnavailable
	}
	if s.policy.DatabaseDenied(name) {
		return CreatedDatabase{}, ErrNotFound
	}
	row, err := s.catalog.Lookup(ctx, name)
	if err != nil {
		return CreatedDatabase{}, mapCatalogError(err)
	}
	if row.Owner == "" || !row.AllowConn || row.IsTemplate {
		return CreatedDatabase{}, ErrNotFound
	}
	if !s.policy.Manageable(row.Name, row.Owner, row.AllowConn, row.IsTemplate) || s.policy.OwnerDenied(row.Owner) {
		return CreatedDatabase{}, ErrProtected
	}
	if !row.OwnerCanLogin || row.OwnerIsSuperuser {
		return CreatedDatabase{}, ErrProtected
	}
	if s.vaultKey == "" {
		return CreatedDatabase{}, ErrUnavailable
	}
	if _, err := s.catalog.SavedRoleNames(ctx, []string{row.Owner}); err != nil {
		return CreatedDatabase{}, ErrUnavailable
	}

	password, err := GeneratePassword()
	if err != nil {
		return CreatedDatabase{}, ErrUnavailable
	}
	token, err := secrets.Encrypt(s.vaultKey, []byte(password))
	if err != nil {
		return CreatedDatabase{}, ErrUnavailable
	}
	if !s.tryLockOwner(row.Owner) {
		return CreatedDatabase{}, ErrOperationInProgress
	}
	defer s.unlockOwner(row.Owner)

	if err := s.catalog.AlterRolePassword(ctx, row.Owner, password); err != nil {
		return CreatedDatabase{}, mapCatalogError(err)
	}

	var upsertErr error
	for i := 0; i < vaultUpsertAttempts; i++ {
		upsertErr = s.catalog.UpsertCredential(ctx, row.Owner, token)
		if upsertErr == nil {
			return CreatedDatabase{Database: row.Name, Owner: row.Owner, Password: password}, nil
		}
	}
	return CreatedDatabase{}, VaultUnsynced{Database: row.Name, Owner: row.Owner}
}

func (s *Service) tryLockOwner(owner string) bool {
	s.rotateMu.Lock()
	defer s.rotateMu.Unlock()
	if s.rotating == nil {
		s.rotating = map[string]struct{}{}
	}
	if _, held := s.rotating[owner]; held {
		return false
	}
	s.rotating[owner] = struct{}{}
	return true
}

func (s *Service) unlockOwner(owner string) {
	s.rotateMu.Lock()
	defer s.rotateMu.Unlock()
	delete(s.rotating, owner)
}

func (c PoolCatalog) AlterRolePassword(ctx context.Context, owner, password string) error {
	sql, err := formatAlterRolePassword(owner, password)
	if err != nil {
		return err
	}
	return c.execSimple(ctx, sql)
}

func (c PoolCatalog) UpsertCredential(ctx context.Context, role, encrypted string) error {
	if err := ValidateIdentifier(role); err != nil {
		return err
	}
	conn, connectCtx, closeConn, err := c.connectTarget(ctx, vaultDatabase)
	if err != nil {
		return ErrUnavailable
	}
	defer closeConn()
	if _, err := conn.Exec(connectCtx, upsertCredentialSQL, role, encrypted); err != nil {
		return ErrUnavailable
	}
	return nil
}
