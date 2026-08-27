package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SSujitX/redgres/internal/securefile"
	"github.com/SSujitX/redgres/migrations"
	"modernc.org/sqlite"
)

type SQLiteRestoreCheck struct {
	SchemaVersion      int
	OwnerCount         int64
	SessionCount       int64
	LoginAttemptCount  int64
	AuditEventCount    int64
	OperationCount     int64
	OperationLockCount int64
}

const maxSQLiteRestoreSnapshotBytes int64 = 512 << 20

// VerifySQLiteSnapshotRestore restores a captured snapshot into a fresh,
// isolated in-memory database and validates its migration ledger, integrity,
// foreign keys, and bounded control-table counts.
// It never writes to the snapshot.
func VerifySQLiteSnapshotRestore(ctx context.Context, snapshot SQLiteSnapshot) (_ SQLiteRestoreCheck, finalErr error) {
	if err := ctx.Err(); err != nil {
		return SQLiteRestoreCheck{}, err
	}
	if err := validateSQLiteSnapshotMetadata(snapshot); err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "snapshot metadata")
	}

	pinned, err := pinSQLiteRestoreSnapshot(snapshot)
	if err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "pin snapshot")
	}
	defer func() {
		if err := pinned.close(); err != nil && finalErr == nil {
			finalErr = sqliteRestoreStageError(ctx, err, "close snapshot")
		}
	}()

	restoreSource, err := prepareSQLiteRestoreSource(ctx, snapshot.Path, pinned.file)
	if err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "prepare restore source")
	}
	defer func() {
		if err := restoreSource.close(); err != nil && finalErr == nil {
			finalErr = sqliteRestoreStageError(ctx, err, "close restore source")
		}
	}()
	if restoreSource.size != snapshot.SizeBytes || restoreSource.digest != snapshot.SHA256 {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, errors.New("snapshot metadata mismatch"), "snapshot metadata")
	}

	destination, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "open destination")
	}
	destination.SetMaxOpenConns(1)
	defer func() {
		if err := destination.Close(); err != nil && finalErr == nil {
			finalErr = sqliteRestoreStageError(ctx, err, "close destination")
		}
	}()

	conn, err := destination.Conn(ctx)
	if err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "open destination")
	}
	defer func() {
		if err := conn.Close(); err != nil && finalErr == nil {
			finalErr = sqliteRestoreStageError(ctx, err, "close destination")
		}
	}()

	verifySourceIdentity := func() error {
		return pinned.verifyIdentity()
	}
	err = conn.Raw(func(raw any) error {
		provider, ok := raw.(interface {
			NewRestore(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("sqlite driver does not support online restore")
		}
		return runSQLiteRestore(ctx, func() (sqliteRestoreBackup, error) {
			backup, err := provider.NewRestore(restoreSource.uri)
			if err != nil {
				return nil, err
			}
			return backup, nil
		}, verifySourceIdentity)
	})
	if err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "restore snapshot")
	}

	if err := verifySQLiteRestoreIntegrity(ctx, conn); err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "integrity check")
	}
	if err := verifySQLiteRestoreForeignKeys(ctx, conn); err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "foreign key check")
	}

	check, err := verifySQLiteRestoreMigrations(ctx, conn)
	if err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "migration check")
	}
	if err := querySQLiteRestoreCounts(ctx, conn, &check); err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "control count check")
	}

	afterSize, afterDigest, err := streamBoundedSQLiteRestoreSnapshot(ctx, snapshot.Path, pinned.file)
	if err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "hash snapshot after restore")
	}
	if afterSize != restoreSource.size || afterDigest != restoreSource.digest ||
		afterSize != snapshot.SizeBytes || afterDigest != snapshot.SHA256 {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, errors.New("snapshot changed"), "snapshot stability check")
	}
	if err := verifySourceIdentity(); err != nil {
		return SQLiteRestoreCheck{}, sqliteRestoreStageError(ctx, err, "snapshot stability check")
	}
	if err := ctx.Err(); err != nil {
		return SQLiteRestoreCheck{}, err
	}
	return check, nil
}

func streamBoundedSQLiteRestoreSnapshot(ctx context.Context, path string, file *os.File) (int64, string, error) {
	before, err := file.Stat()
	if err != nil {
		return 0, "", err
	}
	if !before.Mode().IsRegular() || before.Size() > maxSQLiteRestoreSnapshotBytes {
		return 0, "", errors.New("invalid sqlite restore source size")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, "", err
	}

	hash := sha256.New()
	buffer := make([]byte, 128*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, "", err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			size += int64(n)
			if size > maxSQLiteRestoreSnapshotBytes {
				return 0, "", errors.New("sqlite restore source exceeds size limit")
			}
			_, _ = hash.Write(buffer[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, "", readErr
		}
	}

	after, err := file.Stat()
	if err != nil {
		return 0, "", err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) ||
		size != before.Size() || size != after.Size() {
		return 0, "", errors.New("sqlite restore source changed while hashing")
	}
	if err := securefile.VerifyRegularPath(path, file); err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

type sqliteRestoreSource struct {
	uri       string
	size      int64
	digest    string
	closeFunc func() error
}

func (s *sqliteRestoreSource) close() error {
	if s == nil || s.closeFunc == nil {
		return nil
	}
	return s.closeFunc()
}

func validateSQLiteSnapshotMetadata(snapshot SQLiteSnapshot) error {
	if snapshot.Path == "" || !filepath.IsAbs(snapshot.Path) || filepath.Clean(snapshot.Path) != snapshot.Path {
		return errors.New("invalid snapshot path")
	}
	if strings.ContainsAny(snapshot.Path, "?#%") || strings.ContainsRune(snapshot.Path, 0) {
		return errors.New("invalid snapshot path")
	}
	if filepath.Base(snapshot.Path) != "redgres-sqlite-snapshot.db" ||
		!validSQLiteSnapshotOperationName(filepath.Base(filepath.Dir(snapshot.Path))) {
		return errors.New("invalid snapshot producer namespace")
	}
	if snapshot.SizeBytes <= 0 || snapshot.SizeBytes > maxSQLiteRestoreSnapshotBytes ||
		len(snapshot.SHA256) != sha256HexLength ||
		snapshot.SHA256 != strings.ToLower(snapshot.SHA256) {
		return errors.New("invalid snapshot digest metadata")
	}
	if _, err := hex.DecodeString(snapshot.SHA256); err != nil {
		return errors.New("invalid snapshot digest metadata")
	}
	return nil
}

const sha256HexLength = 64

type pinnedSQLiteRestoreSnapshot struct {
	root      *snapshotRoot
	operation *snapshotOperation
	target    *snapshotTarget
	file      *os.File
}

func pinSQLiteRestoreSnapshot(snapshot SQLiteSnapshot) (*pinnedSQLiteRestoreSnapshot, error) {
	rootPath := filepath.Dir(filepath.Dir(snapshot.Path))
	root, err := openSnapshotRoot(rootPath)
	if err != nil {
		return nil, err
	}
	closeRoot := true
	defer func() {
		if closeRoot {
			_ = root.handle.Close()
		}
	}()

	operationName := filepath.Base(filepath.Dir(snapshot.Path))
	operationInfo, err := root.handle.Lstat(operationName)
	if err != nil || !operationInfo.IsDir() || operationInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("invalid snapshot operation directory")
	}
	operationHandle, err := root.handle.OpenRoot(operationName)
	if err != nil {
		return nil, err
	}
	operation := &snapshotOperation{
		name: operationName, path: filepath.Dir(snapshot.Path), handle: operationHandle,
		identity: operationInfo, parent: root,
	}
	closeOperation := true
	defer func() {
		if closeOperation {
			_ = operationHandle.Close()
		}
	}()
	if err := operation.verifyIdentity(); err != nil {
		return nil, err
	}

	targetName := filepath.Base(snapshot.Path)
	file, err := operation.handle.OpenFile(targetName, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	closeFile := true
	defer func() {
		if closeFile {
			_ = file.Close()
		}
	}()
	targetInfo, err := file.Stat()
	if err != nil || !targetInfo.Mode().IsRegular() {
		return nil, errors.New("invalid snapshot file")
	}
	target := &snapshotTarget{
		name: targetName, path: snapshot.Path, file: file, identity: targetInfo, operation: operation,
	}
	pinned := &pinnedSQLiteRestoreSnapshot{root: root, operation: operation, target: target, file: file}
	if err := pinned.verifyIdentity(); err != nil {
		return nil, err
	}

	closeRoot = false
	closeOperation = false
	closeFile = false
	return pinned, nil
}

func (p *pinnedSQLiteRestoreSnapshot) verifyIdentity() error {
	if err := p.root.verifyIdentity(); err != nil {
		return err
	}
	if err := p.operation.verifyIdentity(); err != nil {
		return err
	}
	current, err := p.operation.handle.Lstat(p.target.name)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(p.target.identity, current) {
		return errors.New("snapshot identity changed")
	}
	if err := securefile.VerifyRegularPath(p.target.path, p.file); err != nil {
		return err
	}
	if err := requirePrivateSnapshotFile(p.file); err != nil {
		return err
	}
	return p.target.requireNoSidecars()
}

func (p *pinnedSQLiteRestoreSnapshot) close() error {
	return errors.Join(p.file.Close(), p.operation.handle.Close(), p.root.handle.Close())
}

func validSQLiteSnapshotOperationName(name string) bool {
	const prefix = "redgres-sqlite-"
	if !strings.HasPrefix(name, prefix) || len(name) != len(prefix)+48 {
		return false
	}
	for _, character := range name[len(prefix):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

type sqliteRestoreBackup interface {
	Step(int32) (bool, error)
	Finish() error
}

func runSQLiteRestore(
	ctx context.Context,
	start func() (sqliteRestoreBackup, error),
	verifySource func() error,
) error {
	backup, err := start()
	if err != nil {
		return err
	}

	var restoreErr error
	if err := verifySource(); err != nil {
		restoreErr = err
	}
	for restoreErr == nil {
		if err := ctx.Err(); err != nil {
			restoreErr = err
			break
		}
		if err := verifySource(); err != nil {
			restoreErr = err
			break
		}
		more, err := backup.Step(snapshotStepPages)
		if err != nil {
			restoreErr = err
			break
		}
		if !more {
			break
		}
	}
	return errors.Join(restoreErr, backup.Finish())
}

func verifySQLiteRestoreIntegrity(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	valid := true
	for rows.Next() {
		count++
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		if result != "ok" {
			valid = false
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 1 || !valid {
		return errors.New("invalid integrity result")
	}
	return rows.Close()
}

func verifySQLiteRestoreForeignKeys(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("foreign key violation")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return rows.Close()
}

func verifySQLiteRestoreMigrations(ctx context.Context, conn *sql.Conn) (SQLiteRestoreCheck, error) {
	embedded, err := loadMigrations(migrations.FS)
	if err != nil || len(embedded) == 0 {
		return SQLiteRestoreCheck{}, errors.New("embedded migrations unavailable")
	}
	rows, err := conn.QueryContext(
		ctx,
		`SELECT version, checksum FROM schema_migrations LIMIT ?`,
		len(embedded)+1,
	)
	if err != nil {
		return SQLiteRestoreCheck{}, err
	}
	defer rows.Close()

	applied := make(map[int]string, len(embedded))
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return SQLiteRestoreCheck{}, err
		}
		if _, exists := applied[version]; exists {
			return SQLiteRestoreCheck{}, errors.New("duplicate migration version")
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return SQLiteRestoreCheck{}, err
	}
	if err := rows.Close(); err != nil {
		return SQLiteRestoreCheck{}, err
	}
	if len(applied) != len(embedded) {
		return SQLiteRestoreCheck{}, errors.New("migration set mismatch")
	}
	for _, migration := range embedded {
		if applied[migration.Version] != migration.Checksum {
			return SQLiteRestoreCheck{}, errors.New("migration checksum mismatch")
		}
	}
	return SQLiteRestoreCheck{SchemaVersion: embedded[len(embedded)-1].Version}, nil
}

func querySQLiteRestoreCounts(ctx context.Context, conn *sql.Conn, check *SQLiteRestoreCheck) error {
	queries := [...]struct {
		query string
		value *int64
	}{
		{`SELECT COUNT(*) FROM owners`, &check.OwnerCount},
		{`SELECT COUNT(*) FROM sessions`, &check.SessionCount},
		{`SELECT COUNT(*) FROM login_attempts`, &check.LoginAttemptCount},
		{`SELECT COUNT(*) FROM audit_events`, &check.AuditEventCount},
		{`SELECT COUNT(*) FROM operations`, &check.OperationCount},
		{`SELECT COUNT(*) FROM operation_locks`, &check.OperationLockCount},
	}
	for _, query := range queries {
		if err := conn.QueryRowContext(ctx, query.query).Scan(query.value); err != nil {
			return err
		}
	}
	return nil
}

func sqliteRestoreStageError(ctx context.Context, err error, stage string) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return fmt.Errorf("verify sqlite snapshot restore: %s", stage)
}
