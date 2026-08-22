package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	if err := prepareStatePath(path); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
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
	if strings.ContainsAny(path, "?#") || strings.ContainsRune(path, 0) {
		return fmt.Errorf("sqlite path must not contain URI reserved characters")
	}
	return nil
}

func prepareStatePath(path string) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create sqlite directory: %w", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil && !ignorePermission(err) {
			return fmt.Errorf("restrict sqlite directory: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("create sqlite file: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	return restrictStateFiles(path)
}

func restrictStateFiles(path string) error {
	if err := chmodIfExist(path, 0o600); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := chmodIfExist(path+suffix, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func chmodIfExist(path string, mode os.FileMode) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.Chmod(path, mode); err != nil && !ignorePermission(err) {
		return fmt.Errorf("restrict %s: %w", filepath.Base(path), err)
	}
	return nil
}

func ignorePermission(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not supported")
}
