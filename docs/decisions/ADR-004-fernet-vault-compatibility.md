# ADR-004: Preserve the existing PostgreSQL Fernet vault during migration

Status: Accepted
Date: 2026-08-23

## Context

The Python console stores project passwords as Fernet tokens in `database_console_vault`, with a key derived from `SESSION_SECRET`. Existing application connection URLs depend on these credentials remaining recoverable.

## Decision

Redgres initially reads/writes the exact legacy vault format after Python↔Go and copied-record compatibility tests. The database, table, KDF prefix, SHA-256 derivation, URL-safe base64, Fernet format, and UTF-8 handling remain unchanged. No vault move to SQLite and no algorithm change occur during initial cutover.

## Consequences

- Existing passwords remain revealable without forced rotation.
- The legacy secret remains critical and must be backed up/protected.
- A Go Fernet dependency/implementation requires careful review and known-answer tests.
- Future dedicated keys/envelope versioning require a new ADR and reversible migration.
- PostgreSQL role alteration and vault update are cross-store operations needing explicit compensation/recovery state.
