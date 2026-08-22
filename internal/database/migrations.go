package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var migrationName = regexp.MustCompile(`^(\d{3})_([a-z0-9_]+)\.sql$`)

type migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

func Migrate(db *sql.DB, sources fs.FS) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TEXT NOT NULL
)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	pending, err := loadMigrations(sources)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return fmt.Errorf("no migrations found")
	}

	applied, err := loadApplied(db)
	if err != nil {
		return err
	}
	if err := validateApplied(applied, pending); err != nil {
		return err
	}

	for _, m := range pending {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return err
		}
	}
	return nil
}

func loadMigrations(sources fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(sources, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	seen := map[int]string{}
	var out []migration
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("migration %s: directories are not allowed", entry.Name())
		}
		matches := migrationName.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("migration %s: name must match NNN_name.sql", entry.Name())
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("migration %s: invalid version", entry.Name())
		}
		if existing, ok := seen[version]; ok {
			return nil, fmt.Errorf("migration version %d is duplicated by %s and %s", version, existing, entry.Name())
		}
		seen[version] = entry.Name()
		raw, err := fs.ReadFile(sources, path.Clean(entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(raw)
		out = append(out, migration{
			Version:  version,
			Name:     matches[2],
			SQL:      string(raw),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func loadApplied(db *sql.DB) (map[int]appliedMigration, error) {
	rows, err := db.Query(`SELECT version, name, checksum, applied_at FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]appliedMigration{}
	for rows.Next() {
		var row appliedMigration
		if err := rows.Scan(&row.Version, &row.Name, &row.Checksum, &row.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[row.Version] = row
	}
	return applied, rows.Err()
}

type appliedMigration struct {
	Version   int
	Name      string
	Checksum  string
	AppliedAt string
}

func validateApplied(applied map[int]appliedMigration, pending []migration) error {
	newest := pending[len(pending)-1].Version
	byVersion := map[int]migration{}
	for _, m := range pending {
		byVersion[m.Version] = m
	}
	for version, row := range applied {
		if version > newest {
			return fmt.Errorf("schema version %d is newer than embedded migrations", version)
		}
		m, ok := byVersion[version]
		if !ok {
			return fmt.Errorf("schema version %d is recorded but not embedded", version)
		}
		if !strings.EqualFold(row.Checksum, m.Checksum) {
			return fmt.Errorf("schema version %d checksum mismatch", version)
		}
	}
	return nil
}

func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", m.Version, err)
	}
	if _, err := tx.Exec(m.SQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Name, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		m.Version,
		m.Name,
		m.Checksum,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration %d: %w", m.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", m.Version, err)
	}
	return nil
}
