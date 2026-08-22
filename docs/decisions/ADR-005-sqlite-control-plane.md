# ADR-005: SQLite for Redgres control-plane state

Status: Accepted
Date: 2026-08-23

## Context

The Redis console already uses SQLite successfully for one-owner auth, sessions, lockouts, and audit. Redgres runs as one instance on one host and does not need a separate control database for v1.

## Decision

Use SQLite via `modernc.org/sqlite` for owner hashes, hashed sessions/CSRF, login attempts, audit events, and operation state. Use WAL, foreign keys, migrations, constrained connections, and online backups.

## Consequences

- Simple single-binary deployment and no circular dependency on managed PostgreSQL.
- Single-writer/single-instance limitation is explicit.
- SQLite is not used for project credentials.
- Multi-instance HA would require a future storage/coordination ADR and migration.
- Copying the main DB alone during WAL activity is not a valid backup.
