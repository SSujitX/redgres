package database

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/SSujitX/redgres/internal/securefile"
	_ "modernc.org/sqlite"
)

const productionStateDirectory = "/var/lib/redgres"

var stateSidecarSuffixes = [...]string{"-journal", "-wal", "-shm"}

func Open(path string) (*sql.DB, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	stateFile, err := prepareStatePath(path)
	if err != nil {
		return nil, err
	}
	defer stateFile.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable wal: %w", err)
	}
	if err := securefile.VerifyRegularPath(path, stateFile); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("verify sqlite file: %w", err)
	}
	if err := restrictStateFiles(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("sqlite path is required")
	}
	if strings.ContainsAny(path, "?#%") || strings.ContainsRune(path, 0) {
		return fmt.Errorf("sqlite path must not contain URI reserved characters")
	}
	return nil
}

func prepareStatePath(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	var f *os.File
	var err error
	if isProductionStatePath(path) {
		f, err = securefile.OpenRegularUnder(filepath.FromSlash(productionStateDirectory), path, os.O_CREATE|os.O_RDWR, 0o600)
	} else {
		if dir != "." && dir != "" {
			if err := securefile.EnsureRealDir(dir, 0o700); err != nil {
				return nil, fmt.Errorf("create sqlite directory: %w", err)
			}
		}
		f, err = securefile.OpenRegular(path, os.O_CREATE|os.O_RDWR, 0o600)
	}
	if err != nil {
		return nil, fmt.Errorf("create sqlite file: %w", err)
	}
	if dir != "." && dir != "" {
		if err := restrictStateDirectory(dir); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	if err := f.Chmod(0o600); err != nil && !ignorePermission(err) {
		_ = f.Close()
		return nil, fmt.Errorf("restrict sqlite file: %w", err)
	}
	return f, nil
}

func isProductionStatePath(path string) bool {
	cleaned := pathpkg.Clean(strings.ReplaceAll(path, `\`, "/"))
	prefix := productionStateDirectory + "/"
	if !strings.HasPrefix(cleaned, prefix) {
		return false
	}
	rel := strings.TrimPrefix(cleaned, prefix)
	return rel != "" && filepath.IsLocal(filepath.FromSlash(rel))
}

func restrictStateDirectory(path string) error {
	before, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect sqlite directory: %w", err)
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("sqlite directory must be a directory")
	}
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open sqlite directory: %w", err)
	}
	defer dir.Close()
	opened, err := dir.Stat()
	if err != nil || !opened.IsDir() || !os.SameFile(before, opened) {
		return fmt.Errorf("sqlite directory changed while opening")
	}
	if err := dir.Chmod(0o700); err != nil && !ignorePermission(err) {
		return fmt.Errorf("restrict sqlite directory: %w", err)
	}
	after, err := os.Lstat(path)
	if err != nil || !after.IsDir() || !os.SameFile(opened, after) {
		return fmt.Errorf("sqlite directory changed while restricting")
	}
	return nil
}

func restrictStateFiles(path string) error {
	if err := chmodIfExist(path, 0o600); err != nil {
		return err
	}
	for _, suffix := range stateSidecarSuffixes {
		if err := chmodIfExist(path+suffix, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func chmodIfExist(path string, mode os.FileMode) error {
	file, err := securefile.OpenRegular(path, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", filepath.Base(path), err)
	}
	defer file.Close()
	if err := file.Chmod(mode); err != nil && !ignorePermission(err) {
		return fmt.Errorf("restrict %s: %w", filepath.Base(path), err)
	}
	if err := securefile.VerifyRegularPath(path, file); err != nil {
		return fmt.Errorf("verify %s: %w", filepath.Base(path), err)
	}
	return nil
}

func ignorePermission(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not supported")
}
