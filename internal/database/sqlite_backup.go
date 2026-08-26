package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SSujitX/redgres/internal/securefile"
	"modernc.org/sqlite"
)

const (
	snapshotStepPages   = int32(64)
	snapshotNameRetries = 8
)

var snapshotSidecarSuffixes = [...]string{"-journal", "-wal", "-shm"}

type SQLiteSnapshot struct {
	Path      string
	SizeBytes int64
	SHA256    string
}

// CaptureSQLiteSnapshot creates a standalone, integrity-checked online backup
// in an existing private staging directory. The caller chooses only the root;
// the snapshot name is generated and reserved exclusively by this package.
func CaptureSQLiteSnapshot(ctx context.Context, source *sql.DB, stagingRoot string) (_ SQLiteSnapshot, finalErr error) {
	if source == nil {
		return SQLiteSnapshot{}, fmt.Errorf("capture sqlite snapshot: source database is required")
	}
	if err := ctx.Err(); err != nil {
		return SQLiteSnapshot{}, err
	}

	root, err := openSnapshotRoot(stagingRoot)
	if err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("capture sqlite snapshot: unsafe staging root: %w", err)
	}
	defer root.handle.Close()

	sourceConn, err := source.Conn(ctx)
	if err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("capture sqlite snapshot: acquire source connection: %w", err)
	}
	defer sourceConn.Close()

	var target *snapshotTarget
	err = sourceConn.Raw(func(raw any) error {
		provider, ok := raw.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("sqlite driver does not support online backup")
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		var reserveErr error
		target, reserveErr = root.reserveTarget()
		if reserveErr != nil {
			return reserveErr
		}
		if err := root.verifyIdentity(); err != nil {
			return err
		}
		if err := securefile.VerifyRegularPath(target.path, target.file); err != nil {
			return fmt.Errorf("verify reserved snapshot: %w", err)
		}
		return runSQLiteBackup(ctx, provider, target.path, func() error {
			if err := root.verifyIdentity(); err != nil {
				return err
			}
			if err := target.operation.verifyIdentity(); err != nil {
				return err
			}
			return securefile.VerifyRegularPath(target.path, target.file)
		})
	})
	if err != nil {
		if target != nil {
			finalErr = errors.Join(err, root.cleanupTarget(target))
			return SQLiteSnapshot{}, finalErr
		}
		return SQLiteSnapshot{}, fmt.Errorf("capture sqlite snapshot: %w", err)
	}
	defer func() {
		if target != nil && target.operation != nil && target.operation.handle != nil {
			finalErr = errors.Join(finalErr, target.operation.handle.Close())
			target.operation.handle = nil
		}
	}()

	cleanup := true
	defer func() {
		if cleanup {
			finalErr = errors.Join(finalErr, root.cleanupTarget(target))
		}
	}()

	if err := target.file.Chmod(0o600); err != nil && !ignorePermission(err) {
		return SQLiteSnapshot{}, fmt.Errorf("restrict sqlite snapshot: %w", err)
	}
	if err := root.verifyIdentity(); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot root: %w", err)
	}
	if err := target.operation.verifyIdentity(); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot operation: %w", err)
	}
	if err := securefile.VerifyRegularPath(target.path, target.file); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot: %w", err)
	}
	if err := target.file.Close(); err != nil {
		target.file = nil
		return SQLiteSnapshot{}, fmt.Errorf("close sqlite snapshot: %w", err)
	}
	target.file = nil

	pinned, err := securefile.OpenRegularUnder(root.path, target.path, os.O_RDONLY, 0)
	if err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("pin sqlite snapshot: %w", err)
	}
	defer pinned.Close()
	if err := root.verifyIdentity(); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot root: %w", err)
	}
	if err := target.operation.verifyIdentity(); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot operation: %w", err)
	}

	size, digest, err := verifyStableSQLiteSnapshot(ctx, target.path, pinned, func() error {
		return checkSQLiteSnapshotIntegrity(ctx, target.path)
	})
	if err != nil {
		return SQLiteSnapshot{}, err
	}
	if err := root.verifyIdentity(); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot root: %w", err)
	}
	if err := securefile.VerifyRegularPath(target.path, pinned); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot after integrity check: %w", err)
	}
	if err := target.operation.verifyIdentity(); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot operation: %w", err)
	}
	if err := target.requireNoSidecars(); err != nil {
		return SQLiteSnapshot{}, err
	}

	if err := root.verifyIdentity(); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot root: %w", err)
	}
	if err := securefile.VerifyRegularPath(target.path, pinned); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot after hashing: %w", err)
	}
	if err := target.operation.verifyIdentity(); err != nil {
		return SQLiteSnapshot{}, fmt.Errorf("verify sqlite snapshot operation: %w", err)
	}
	if err := requirePrivateSnapshotFile(pinned); err != nil {
		return SQLiteSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return SQLiteSnapshot{}, err
	}

	cleanup = false
	return SQLiteSnapshot{Path: target.path, SizeBytes: size, SHA256: digest}, nil
}

func runSQLiteBackup(ctx context.Context, provider interface {
	NewBackup(string) (*sqlite.Backup, error)
}, targetPath string, verifyOpened func() error) error {
	backup, err := provider.NewBackup(targetPath)
	if err != nil {
		return fmt.Errorf("start sqlite online backup: %w", err)
	}

	var copyErr error
	if err := verifyOpened(); err != nil {
		copyErr = fmt.Errorf("verify opened sqlite backup target: %w", err)
	}
	for {
		if copyErr != nil {
			break
		}
		if err := ctx.Err(); err != nil {
			copyErr = err
			break
		}
		more, err := backup.Step(snapshotStepPages)
		if err != nil {
			copyErr = fmt.Errorf("step sqlite online backup: %w", err)
			break
		}
		if !more {
			break
		}
	}
	finishErr := backup.Finish()
	if finishErr != nil {
		finishErr = fmt.Errorf("finish sqlite online backup: %w", finishErr)
	}
	return errors.Join(copyErr, finishErr)
}

type snapshotRoot struct {
	path     string
	handle   *os.Root
	identity fs.FileInfo
}

func openSnapshotRoot(path string) (*snapshotRoot, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, fmt.Errorf("staging root must be absolute")
	}
	if strings.ContainsAny(path, "?#%") || strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("staging root contains reserved pathname characters")
	}
	path = filepath.Clean(path)
	if err := verifySnapshotRootAncestors(path); err != nil {
		return nil, err
	}

	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, fmt.Errorf("staging root must be a real directory")
	}
	if runtime.GOOS != "windows" && before.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("staging root must not grant group or other permissions")
	}

	handle, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	root := &snapshotRoot{path: path, handle: handle, identity: before}
	if err := root.verifyIdentity(); err != nil {
		_ = handle.Close()
		return nil, err
	}
	return root, nil
}

func verifySnapshotRootAncestors(path string) error {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("staging root ancestry must contain only real directories")
		}
		if err := verifySnapshotAncestorSecurity(info, current == filepath.Clean(path)); err != nil {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}

func (r *snapshotRoot) verifyIdentity() error {
	if err := verifySnapshotRootAncestors(r.path); err != nil {
		return err
	}
	current, err := os.Lstat(r.path)
	if err != nil {
		return err
	}
	opened, err := r.handle.Stat(".")
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !opened.IsDir() ||
		!os.SameFile(r.identity, current) || !os.SameFile(r.identity, opened) {
		return fmt.Errorf("staging root changed during capture")
	}
	if runtime.GOOS != "windows" && (current.Mode().Perm()&0o077 != 0 || opened.Mode().Perm()&0o077 != 0) {
		return fmt.Errorf("staging root permissions changed during capture")
	}
	return nil
}

type snapshotTarget struct {
	name      string
	path      string
	file      *os.File
	identity  fs.FileInfo
	operation *snapshotOperation
}

func (r *snapshotRoot) reserveTarget() (*snapshotTarget, error) {
	operation, err := r.reserveOperation()
	if err != nil {
		return nil, err
	}
	target, err := operation.reserveTarget()
	if err != nil {
		_ = operation.closeAndRemoveIfEmpty()
		return nil, err
	}
	return target, nil
}

type snapshotOperation struct {
	name     string
	path     string
	handle   *os.Root
	identity fs.FileInfo
	parent   *snapshotRoot
}

func (r *snapshotRoot) reserveOperation() (*snapshotOperation, error) {
	for range snapshotNameRetries {
		var randomBytes [24]byte
		if _, err := rand.Read(randomBytes[:]); err != nil {
			return nil, fmt.Errorf("generate sqlite snapshot operation name: %w", err)
		}
		name := "redgres-sqlite-" + hex.EncodeToString(randomBytes[:])
		err := r.handle.Mkdir(name, 0o700)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reserve sqlite snapshot operation: %w", err)
		}
		identity, err := r.handle.Lstat(name)
		if err != nil || !identity.IsDir() || identity.Mode()&os.ModeSymlink != 0 {
			_ = r.handle.Remove(name)
			return nil, fmt.Errorf("verify sqlite snapshot operation directory")
		}
		handle, err := r.handle.OpenRoot(name)
		if err != nil {
			_ = r.handle.Remove(name)
			return nil, fmt.Errorf("open sqlite snapshot operation: %w", err)
		}
		operation := &snapshotOperation{name: name, path: filepath.Join(r.path, name), handle: handle, identity: identity, parent: r}
		if err := operation.verifyIdentity(); err != nil {
			_ = handle.Close()
			_ = r.handle.Remove(name)
			return nil, fmt.Errorf("verify sqlite snapshot operation: %w", err)
		}
		return operation, nil
	}
	return nil, fmt.Errorf("reserve sqlite snapshot operation: generated names collided")
}

func (o *snapshotOperation) reserveTarget() (*snapshotTarget, error) {
	const name = "redgres-sqlite-snapshot.db"
	file, err := o.handle.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("reserve sqlite snapshot: %w", err)
	}
	identity, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat reserved sqlite snapshot: %w", err)
	}
	return &snapshotTarget{
		name: name, path: filepath.Join(o.path, name), file: file, identity: identity, operation: o,
	}, nil
}

func (o *snapshotOperation) verifyIdentity() error {
	current, err := os.Lstat(o.path)
	if err != nil {
		return err
	}
	opened, err := o.handle.Stat(".")
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !opened.IsDir() ||
		!os.SameFile(o.identity, current) || !os.SameFile(o.identity, opened) {
		return fmt.Errorf("sqlite snapshot operation directory changed")
	}
	if runtime.GOOS != "windows" && (current.Mode().Perm() != 0o700 || opened.Mode().Perm() != 0o700) {
		return fmt.Errorf("sqlite snapshot operation permissions changed")
	}
	return nil
}

func (r *snapshotRoot) cleanupTarget(target *snapshotTarget) (finalErr error) {
	if target == nil {
		return nil
	}
	var cleanupErr error
	defer func() {
		if target.operation != nil && target.operation.handle != nil {
			cleanupErr = errors.Join(cleanupErr, target.operation.handle.Close())
			target.operation.handle = nil
		}
		if cleanupErr != nil {
			finalErr = fmt.Errorf("clean generated sqlite snapshot: %w", cleanupErr)
		}
	}()
	if target.file != nil {
		cleanupErr = errors.Join(cleanupErr, target.file.Close())
		target.file = nil
	}
	if err := r.verifyIdentity(); err != nil {
		cleanupErr = errors.Join(cleanupErr, err)
		return nil
	}
	if err := target.operation.verifyIdentity(); err != nil {
		cleanupErr = errors.Join(cleanupErr, err)
		return nil
	}
	current, err := target.operation.handle.Lstat(target.name)
	if err == nil {
		if !current.Mode().IsRegular() || !os.SameFile(target.identity, current) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("generated sqlite snapshot identity changed"))
		} else {
			cleanupErr = errors.Join(cleanupErr, target.operation.handle.Remove(target.name))
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		cleanupErr = errors.Join(cleanupErr, err)
	}
	for _, suffix := range snapshotSidecarSuffixes {
		name := target.name + suffix
		info, statErr := target.operation.handle.Lstat(name)
		if errors.Is(statErr, fs.ErrNotExist) {
			continue
		}
		if statErr != nil || !info.Mode().IsRegular() {
			cleanupErr = errors.Join(cleanupErr, statErr, fmt.Errorf("generated sqlite snapshot sidecar is not regular"))
			continue
		}
		cleanupErr = errors.Join(cleanupErr, target.operation.handle.Remove(name))
	}
	cleanupErr = errors.Join(cleanupErr, target.operation.closeAndRemoveIfEmpty())
	return nil
}

func (o *snapshotOperation) closeAndRemoveIfEmpty() error {
	if o == nil || o.handle == nil {
		return nil
	}
	if err := o.verifyIdentity(); err != nil {
		return err
	}
	directory, err := o.handle.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil || closeErr != nil {
		return errors.Join(readErr, closeErr)
	}
	if len(entries) != 0 {
		return fmt.Errorf("sqlite snapshot operation directory is not empty")
	}
	if err := o.parent.verifyIdentity(); err != nil {
		return err
	}
	current, err := o.parent.handle.Lstat(o.name)
	if err != nil || !current.IsDir() || !os.SameFile(o.identity, current) {
		return fmt.Errorf("sqlite snapshot operation directory changed before cleanup")
	}
	if err := o.handle.Close(); err != nil {
		return err
	}
	o.handle = nil
	return o.parent.handle.Remove(o.name)
}

func (t *snapshotTarget) requireNoSidecars() error {
	for _, suffix := range snapshotSidecarSuffixes {
		_, err := t.operation.handle.Lstat(t.name + suffix)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return fmt.Errorf("inspect sqlite snapshot sidecar: %w", err)
		default:
			return fmt.Errorf("sqlite snapshot is not standalone")
		}
	}
	return nil
}

func checkSQLiteSnapshotIntegrity(ctx context.Context, path string) (finalErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&immutable=1"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open sqlite snapshot for integrity check: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer func() { finalErr = errors.Join(finalErr, db.Close()) }()

	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("check sqlite snapshot integrity: %w", err)
	}
	defer rows.Close()

	count := 0
	valid := true
	for rows.Next() {
		count++
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("read sqlite integrity result: %w", err)
		}
		if result != "ok" {
			valid = false
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read sqlite integrity results: %w", err)
	}
	if count != 1 || !valid {
		return fmt.Errorf("sqlite snapshot failed integrity check")
	}
	return nil
}

func streamSQLiteSnapshot(ctx context.Context, path string, file *os.File) (int64, string, error) {
	before, err := file.Stat()
	if err != nil {
		return 0, "", fmt.Errorf("stat sqlite snapshot: %w", err)
	}
	if !before.Mode().IsRegular() {
		return 0, "", fmt.Errorf("sqlite snapshot is not a regular file")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, "", fmt.Errorf("rewind sqlite snapshot: %w", err)
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
			written, hashErr := hash.Write(buffer[:n])
			if hashErr != nil {
				return 0, "", fmt.Errorf("hash sqlite snapshot: %w", hashErr)
			}
			if written != n {
				return 0, "", fmt.Errorf("hash sqlite snapshot: %w", io.ErrShortWrite)
			}
			size += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return 0, "", fmt.Errorf("read sqlite snapshot: %w", readErr)
		}
	}

	after, err := file.Stat()
	if err != nil {
		return 0, "", fmt.Errorf("restat sqlite snapshot: %w", err)
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) || size != before.Size() || size != after.Size() {
		return 0, "", fmt.Errorf("sqlite snapshot changed while hashing")
	}
	if err := securefile.VerifyRegularPath(path, file); err != nil {
		return 0, "", fmt.Errorf("verify sqlite snapshot while hashing: %w", err)
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

func verifyStableSQLiteSnapshot(
	ctx context.Context,
	path string,
	file *os.File,
	checkIntegrity func() error,
) (int64, string, error) {
	firstSize, firstDigest, err := streamSQLiteSnapshot(ctx, path, file)
	if err != nil {
		return 0, "", err
	}
	if err := checkIntegrity(); err != nil {
		return 0, "", err
	}
	size, digest, err := streamSQLiteSnapshot(ctx, path, file)
	if err != nil {
		return 0, "", err
	}
	if size != firstSize || digest != firstDigest {
		return 0, "", fmt.Errorf("sqlite snapshot changed during integrity verification")
	}
	return size, digest, nil
}

func requirePrivateSnapshotFile(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat sqlite snapshot permissions: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("sqlite snapshot is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return fmt.Errorf("sqlite snapshot permissions changed during capture")
	}
	return nil
}
