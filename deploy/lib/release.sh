#!/usr/bin/env bash
# Fail-closed update/rollback skip matrices (OPS-005 Partial).
# Never source, eval, cat, extract, or checksum operator --release.
# Never print --to VERSION. Never mutate /opt/redgres. Never probe healthz.
# This Partial is not Complete.
set -euo pipefail

redgres_update_print_skip_matrix() {
  cat <<'EOF'
Update (read-only --dry-run; not Complete):
release: path-ok (unread, not extracted)
checksum: skipped (no expected digest key; CONFIGURATION.md has none)
extract: skipped (/opt/redgres/releases not written)
symlink: skipped (current not switched)
sqlite_migrate: skipped
systemd: skipped (unit/credentials not written)
health_gate: skipped (GET /api/v1/healthz not probed; curl not invoked)
postgres_packages: skipped (not part of application update)
result=partial
EOF
}

redgres_rollback_print_skip_matrix() {
  cat <<'EOF'
Rollback (read-only --dry-run; not Complete):
target: accepted (unread; symlink not switched)
schema_compat: skipped (SQLite schema compatibility not checked)
symlink: skipped (current not switched)
config_restore: skipped
systemd: skipped (unit not restarted)
health_gate: skipped (GET /api/v1/healthz not probed; curl not invoked)
data_reversal: skipped (rollback never reverses PostgreSQL/Redis/vault/credentials/DNS/schema automatically)
result=partial
EOF
}

# PATH already validated as an existing regular file; do not read it.
redgres_update_dry_run() {
  redgres_update_print_skip_matrix
}

# VERSION already validated as path-safe; do not print it.
redgres_rollback_dry_run() {
  redgres_rollback_print_skip_matrix
}

# Reject empty, slash, or '.' / '..' as a path component. Do not echo VERSION.
# bash parameters cannot contain NUL; a $'\0' glob would match every string.
redgres_rollback_version_ok() {
  local version="$1"
  if [[ -z "${version}" ]]; then
    return 1
  fi
  case "${version}" in
    */*) return 1 ;;
    .|..) return 1 ;;
  esac
  return 0
}
