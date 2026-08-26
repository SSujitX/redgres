#!/usr/bin/env bash
# Fail-closed backup dry-run skip matrix (OPS-004 Partial).
# Never source, eval, or cat operator --config. Never invoke pg_dump,
# pg_restore, redis-cli, BGSAVE/LASTSAVE, SQLite backup, or mutate anything.
# Live backup/restore is installer-recovery; this Partial is not Complete.
set -euo pipefail

redgres_backup_print_skip_matrix() {
  cat <<'EOF'
Backup (read-only --dry-run; not Complete):
config: path-ok (unread, not sourced)
postgres_dump: skipped (pg_dump/pg_dumpall not invoked; installer-recovery)
redis_snapshot: skipped (BGSAVE/LASTSAVE not invoked; installer-recovery)
sqlite_backup: skipped (SQLite online backup not invoked; installer-recovery)
manifest: skipped (no backup manifest written)
checksums: skipped (no artifact SHA-256 computed)
retention: skipped (no pruning of manifest-owned paths)
off_host: skipped (no encrypted off-host copy)
restore_evidence: skipped (no isolated restore test)
result=partial
EOF
}

# config_path is already validated as an existing regular file; do not read it.
redgres_backup_dry_run() {
  redgres_backup_print_skip_matrix
}
