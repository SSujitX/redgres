# ADR-005: SQLite for Redgres control-plane state

Status: Accepted
Date: 2026-08-23

## Context

The Redis console already uses SQLite successfully for one-owner auth, sessions, lockouts, and audit. Redgres runs as one instance on one host and does not need a separate control database for v1.

## Decision

Use SQLite via `modernc.org/sqlite` for owner hashes, hashed sessions/CSRF, login attempts, audit events, and operation state. Use WAL, foreign keys, migrations, constrained connections, and online backups.

## Migration mechanism

Use hand-rolled, embedded, ordered `.sql` files. Do not add a third-party migration library: the control plane has a linear schema history, and fail-closed checksum/downgrade behavior is simpler to own than to configure through another driver surface.

- Sources live in `migrations/` as `NNN_name.sql` and are exposed through `//go:embed *.sql`.
- The runner creates `schema_migrations(version INTEGER PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, applied_at TEXT NOT NULL)`.
- `checksum` is lowercase hex SHA-256 of the file bytes. A recorded checksum that differs from the embedded file fails closed.
- A recorded version newer than the newest embedded file fails closed (older binary vs newer schema).
- Each unapplied file runs in its own transaction: execute SQL, insert the bookkeeping row, commit. Failure rolls back that file only.
- `SetMaxOpenConns(1)` serializes writers, so `SQLITE_BUSY` transaction-upgrade deadlocks are structurally excluded.
- Until the first tagged release, `001_initial.sql` may still be edited; developers must delete the local development database after such an edit. After the first tag, only new numbered files.

`operations` remains a later numbered file; its columns depend on the long-operation state machine.

## Consequences

- Simple single-binary deployment and no circular dependency on managed PostgreSQL.
- Single-writer/single-instance limitation is explicit.
- SQLite is not used for project credentials.
- Multi-instance HA would require a future storage/coordination ADR and migration.
- Copying the main DB alone during WAL activity is not a valid backup.
