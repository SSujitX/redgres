# Backup and recovery specification

A successful backup command is not proof of recoverability. Redgres backup sets require consistency, checksums, retention, off-host copies, and restore evidence.

## 1. Recovery objectives

Initial targets, to be adjusted after business review:

- PostgreSQL RPO: 24 hours for logical backups; lower RPO requires WAL archiving/PITR as a separate design.
- Redis RPO: persistence-policy dependent; target at most 24 hours for backup copies, with AOF/RDB durability documented.
- Redgres SQLite RPO: 24 hours plus audit-loss acceptance; consider more frequent snapshots.
- RTO: 4 hours for a single-server rebuild after infrastructure is available.

Do not claim PITR or near-zero RPO unless WAL/AOF handling and restore drills prove it.

## 2. Backup set

One timestamped manifest links:

- PostgreSQL globals (`pg_dumpall --globals-only`) and per-database custom-format dumps, including `database_console_vault`.
- PostgreSQL version, cluster system identifier, extension package origin/exact versions, installed extension version/schema/owner per database, preload configuration, PgBouncer version/config checksum, role/database inventory, and dump tool version.
- Redis verified RDB snapshot, AOF directory/files when enabled, ACL file (`users.acl`) and sanitized configuration metadata.
- Consistent Redgres SQLite backup using SQLite online backup API or `.backup`, followed by integrity check.
- Redgres version/release metadata and non-secret configuration checksums.
- Cloudflare/DNS/TLS reconstruction notes or exported non-secret identifiers; secrets are backed up separately under an approved secret-management process.
- SHA-256 checksum and size for every artifact.

## 3. PostgreSQL procedure

1. Confirm server and `pg_dump` major compatibility.
2. Capture globals.
3. Enumerate intended databases from the server, not shell-parsed untrusted output.
4. Dump each database in custom format with failure-on-error behavior and restrictive umask.
5. Run `pg_restore --list` for structural readability and checksum all files.
6. Copy encrypted/off-host.
7. Periodically restore into clean test clusters for every supported PostgreSQL major and run logical validation. Cross-major restore/upgrade claims require separate evidence.

For larger/stricter RPO deployments, add base backups and WAL archiving through an ADR; logical dumps alone are not PITR.

## 4. Redis atomic capture

Never copy an actively changing live RDB/AOF directory blindly.

1. Record `LASTSAVE`, persistence configuration, AOF state, data directory, and ACL file location.
2. Trigger `BGSAVE` through a backup identity allowed only the required persistence/status commands.
3. Poll persistence status and require successful completion plus an advanced `LASTSAVE`; fail on `rdb_last_bgsave_status != ok`.
4. Copy the completed RDB to a staging directory on the same filesystem where possible, then checksum.
5. If AOF is enabled, use Redis-supported safe AOF backup procedure for the installed version; capture the complete manifest/multipart AOF set consistently.
6. Copy `users.acl` and required sanitized config with permissions preserved.
7. Restore into an isolated instance of every supported Redis series; verify load, key count/sample, ACL users/rules, representative auth, and detected persistence layout.

The exact AOF procedure must be validated against the deployed Redis version because persistence layouts and safe capture procedures can vary between series. Backup manifests record the full detected version and compatibility-policy revision.

## 5. SQLite capture

- Use SQLite online backup API while the app runs, or stop Redgres cleanly and copy DB plus any required WAL state.
- Prefer online backup into a standalone database, then run `PRAGMA integrity_check` on the copy.
- Copying only `redgres.db` while WAL mode is active is not a valid general backup procedure.
- Confirm owner/session/audit/operation table counts within expected ranges after restore. Sessions may be intentionally invalidated during disaster recovery.

## 6. Storage and retention

- Local staging: `/var/backups/redgres`, `root:root 0700`, separate capacity monitoring.
- Off-host: encrypted at rest and in transit, with credentials not stored inside the same backup set.
- Example retention: 7 daily, 4 weekly, 12 monthly; business/legal requirements override.
- Do not delete the last known-good backup until a newer restore is verified.
- Backup pruning targets only manifest-owned paths and is tested against path traversal/broad deletion.

## 7. Restore order

1. Provision isolated host/network; do not restore over production first.
2. Install exact compatible PostgreSQL/Redis/Redgres versions, PgBouncer, and every extension package/control/library required by the manifest before database restore.
3. Verify manifest/signatures/checksums.
4. Restore PostgreSQL globals then databases; verify extension objects/data against [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md) and validate vault access using the protected legacy secret.
5. Restore Redis data/AOF/ACL using the matching persistence configuration.
6. Restore SQLite; invalidate sessions if security policy requires.
7. Restore non-secret configuration and inject secrets through approved mechanism.
8. Run complete verification and application smoke tests.
9. For production recovery, change DNS/firewall only after approval and final backup/evidence capture.

## 8. Required restore evidence

Each drill records date, backup manifest, isolated target, commands/tool versions, duration, checksums, database counts, Redis key/ACL checks, vault fixture reveal, Redgres login/audit check, failures, and approver. A backup status UI must display last successful **backup** and last successful **restore test** separately.

## 9. DROP gate catalog (schema v1)

Live dump, off-host copy, restore, and `deploy/backup.sh` are installer-recovery. This section freezes the filesystem manifest types used by `backup.EvaluateDropGate` ([ADR-011](decisions/ADR-011-drop-backup-gate.md)). There is no SQLite backup table. The catalog jail is `REDGRES_BACKUP_CATALOG`. The only catalog file is jail-root **`current.json`**. Clients never supply a path.

Required identity: `schema_version` = 1; `backup_set_id` 32 lowercase hex; `completed_at` RFC3339Nano; `cluster.system_identifier` decimal string of `SELECT system_identifier FROM pg_control_system()` ([17](https://www.postgresql.org/docs/17/functions-info.html), [18](https://www.postgresql.org/docs/18/functions-info.html)).

Required PostgreSQL artifact for DROP of database `D`: `kind=postgres.database`, `name=D`, `sha256` 64 lowercase hex, `size_bytes` ≥ 0, `path` relative and jail-local via `internal/securefile`.

Off-host: `off_host.completed=true` and `copied_at` ≥ `completed_at` for this set. Off-host age is not a second 24h window. No transport credentials on the manifest.

Restore evidence: `restore.isolated=true`, `restore.outcome=succeeded`, `restore.backup_set_id` equal, `restore.completed_at` non-zero and ≤30d. Evaluation `Now` must be non-zero (fail closed).

`completed_at` must be ≤24h at evaluation time. Fail closed on missing fields, extra JSON, traversal (`..`, absolute, NUL, `?`, `#`, `%`), or identity mismatch. Truncate and row-delete stay ungated. Optional empty `redgres.version` / `redgres.compatibility_policy_revision` do not fail DROP evaluation. Redis/SQLite artifacts are optional for this predicate. Artifact bytes need not exist for Evaluate in this slice.
