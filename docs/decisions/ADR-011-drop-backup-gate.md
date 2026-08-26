# ADR-011: DROP backup-gate catalog types

Status: Accepted
Date: 2026-08-26

## Context

PG-011 drop is in-request **200** with **BF-1** disclosure only. User freeze: DROP requires a target-matched checksummed backup ≤24h, completed off-host copy, and isolated restore evidence ≤30d. Live dump/copy/restore belongs in installer-recovery, not this slice.

## Decision

Backup catalogs are **filesystem manifests**, not a SQLite table and not `003_*.sql`. The catalog jail is `REDGRES_BACKUP_CATALOG` (directory). The only catalog file is jail-root **`current.json`**. Clients never supply a path.

This slice defines schema **v1** types plus path-jail parse and a pure `EvaluateDropGate`. It does not run `pg_dump`, copy off-host, restore, or wire the HTTP DROP handler on the freeze SHA.

### Predicate (all required)

- Manifest `schema_version` = 1.
- `backup_set_id` is 32 lowercase hex.
- `completed_at` RFC3339Nano, not in the future, and age **≤24h** at evaluation time.
- `cluster.system_identifier` is a decimal string of PostgreSQL `bigint`. The live value is `SELECT system_identifier FROM pg_control_system()` ([17 Table 9.88](https://www.postgresql.org/docs/17/functions-info.html), [18 Table 9.90](https://www.postgresql.org/docs/18/functions-info.html)). Catalog.SystemIdentifier is a freeze stub (`ErrUnavailable`) until the DROP writer implements that SQL.
- PostgreSQL target artifact for DROP of database `D`: `kind=postgres.database`, `name=D`, `sha256` 64 lowercase hex, `size_bytes` ≥ 0, `path` relative and jail-local.
- `off_host.completed=true` and `completed_at` ≤ `copied_at` ≤ evaluation `Now` for **this** set. Off-host age is **not** a second 24h window.
- Restore evidence: `restore.isolated=true`, `restore.outcome=succeeded`, `restore.backup_set_id` equals the set id, `completed_at` ≤ `restore.completed_at` ≤ evaluation `Now`, and restore age ≤30d at evaluation time.
- Evaluation `Now` must be non-zero. Fail closed if `Now` is the zero time, restore `completed_at` is zero, any required field is missing, extra JSON fields are present, or identity does not match.

Truncate and row-delete stay ungated.

### Path defense

Catalog directory is `REDGRES_BACKUP_CATALOG`. Production must be under `/var/lib/redgres`. Reject `?`, `#`, `%`, NUL without echoing the path. Unset is allowed at Load; HTTP DROP fail-closed `503` **Backup catalog is not configured.** is a later writer. Reuse `internal/securefile` (`OpenRegularUnder`, `filepath.IsLocal`). Reject `..`, absolute paths, NUL, `?`, `#`, `%`, symlink escape. Artifact bytes need not exist for Evaluate in this slice; checksum **format** is checked. Missing on-disk files fail closed only when a later slice verifies checksums.

### Parser bounds

The schema-v1 parser fails closed before typed manifest allocation unless the raw document is valid UTF-8 with well-formed JSON surrogate pairs. It streams a structural first pass with maximum depth 32 and 32,768 JSON tokens, requires every root and nested field name to exactly match the schema (case aliases are invalid), rejects duplicate or secret-bearing keys, and caps the canonical `artifacts` array at 1,024 entries before typed slice allocation. It then performs strict typed decoding with unknown fields rejected. The raw manifest is capped at 8 MiB; individual identity and artifact text fields are bounded as specified in [BACKUP_RECOVERY.md](../BACKUP_RECOVERY.md).

Optional identity strings `redgres.version` and `redgres.compatibility_policy_revision` may be empty and do **not** fail DROP evaluation. Full release/capability metadata is a later slice.

Redis/SQLite artifacts are optional on the manifest for this DROP predicate.

## Consequences

- HTTP DROP on `260f1ad` calls `EvaluateDropGate` after AUTH-006 and before PostgreSQL access. Unset/unusable catalog is `503` **Backup catalog is not configured.** (not BF-1). Live dump/copy/restore remains installer-recovery; OPS-004 stays Partial.
- OPS-004 Complete still requires live backup/restore evidence.
- Transport credentials must never appear on the manifest.
