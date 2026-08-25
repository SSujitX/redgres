# ADR-011: DROP backup-gate catalog types

Status: Accepted
Date: 2026-08-26

## Context

PG-011 drop is in-request **200** with **BF-1** disclosure only. User freeze: DROP requires a target-matched checksummed backup ≤24h, completed off-host copy, and isolated restore evidence ≤30d. Live dump/copy/restore belongs in installer-recovery, not this slice.

## Decision

Backup catalogs are **filesystem manifests**, not a SQLite table and not `003_*.sql`. No `REDGRES_BACKUP_CATALOG` env key in this slice.

This slice defines schema **v1** types plus path-jail parse and a pure `EvaluateDropGate`. It does not run `pg_dump`, copy off-host, restore, or wire the HTTP DROP handler.

### Predicate (all required)

- Manifest `schema_version` = 1.
- `backup_set_id` is 32 lowercase hex.
- `completed_at` RFC3339Nano, age **≤24h** at evaluation time.
- `cluster.system_identifier` is a decimal string. The live value is supplied by the caller later from official PostgreSQL `pg_control_system()` ([17](https://www.postgresql.org/docs/17/functions-admin.html), [18](https://www.postgresql.org/docs/18/functions-admin.html)). This slice does not query PostgreSQL.
- PostgreSQL target artifact for DROP of database `D`: `kind=postgres.database`, `name=D`, `sha256` 64 lowercase hex, `size_bytes` ≥ 0, `path` relative and jail-local.
- `off_host.completed=true` and `copied_at` ≥ `completed_at` for **this** set. Off-host age is **not** a second 24h window.
- Restore evidence: `restore.isolated=true`, `restore.outcome=succeeded`, `restore.backup_set_id` equals the set id, `restore.completed_at` non-zero and ≤30d at evaluation time.
- Evaluation `Now` must be non-zero. Fail closed if `Now` is the zero time, restore `completed_at` is zero, any required field is missing, extra JSON fields are present, or identity does not match.

Truncate and row-delete stay ungated.

### Path defense

Catalog directory is a caller-supplied jail (tests use a temp dir; production path is wired later). Reuse `internal/securefile` (`OpenRegularUnder`, `filepath.IsLocal`). Reject `..`, absolute paths, NUL, `?`, `#`, `%`, symlink escape. Artifact bytes need not exist for Evaluate in this slice; checksum **format** is checked. Missing on-disk files fail closed only when a later slice verifies checksums.

Optional identity strings `redgres.version` and `redgres.compatibility_policy_revision` may be empty and do **not** fail DROP evaluation. Full release/capability metadata is a later slice.

Redis/SQLite artifacts are optional on the manifest for this DROP predicate.

## Consequences

- HTTP DROP remains BF-1 until backend-integration calls `EvaluateDropGate` before PostgreSQL access.
- OPS-004 Complete still requires live backup/restore evidence.
- Transport credentials must never appear on the manifest.
