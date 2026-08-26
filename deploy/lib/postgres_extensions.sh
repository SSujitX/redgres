#!/usr/bin/env bash
# Fail-closed postgres-extensions apply dry-run skip matrix (OPS-007 Partial).
# Reuses redgres_plan_validate for plan validation. Never sources operator
# --config. Never resolves packages, reads live cluster state, merges preload,
# restarts PostgreSQL, or runs extension DDL. Live apply is not implemented.
set -euo pipefail

redgres_extensions_print_skip_matrix() {
  cat <<'EOF'
postgres-extensions apply (read-only --dry-run; not Complete):
config: path-ok (unread, not sourced)
plan: path-ok (validated, not applied)
package_resolution: skipped (no release manifest in this Partial)
inventory: skipped (live cluster state not probed)
backup_verification: skipped (backup evidence not checked)
preload_merge: skipped (shared_preload_libraries not read)
restart_approval: skipped (--approve-postgres-restart is apply-time)
extension_ddl: skipped (CREATE EXTENSION not executed)
verification: skipped (capability smoke checks deferred)
result=partial
EOF
}

# config_path is validated as an existing regular file; never read it.
redgres_extensions_apply_dry_run() {
  local config_path="$1" plan_path="$2"
  local line
  redgres_plan_validate "${plan_path}"
  redgres_log "policy: ${redgres_plan_policy}"
  if [[ -n "${redgres_plan_preview}" ]]; then
    while IFS= read -r line; do
      [[ -n "${line}" ]] && redgres_log "selection: ${line}"
    done <<< "${redgres_plan_preview}"
  else
    redgres_log 'selection: (none)'
  fi
  redgres_extensions_print_skip_matrix
}
