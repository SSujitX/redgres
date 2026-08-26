#!/usr/bin/env bash
# Fail-closed update/rollback skip matrices (OPS-005 Partial).
# Never source, eval, extract, or print operator --release.
# Never print --to VERSION. Never mutate /opt/redgres. Never probe healthz.
# This Partial is not Complete.
set -euo pipefail

redgres_update_print_skip_matrix() {
  cat <<'EOF'
Update (read-only --dry-run; not Complete):
release: checksum-verified (not extracted)
checksum: verified (adjacent SHA256SUMS; signature/provenance not verified)
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

redgres_update_verify_checksum() {
  local release_path="$1"
  local release_dir release_name checksum_path
  local release_fd release_size checksum_fd checksum_size
  local checksum_snapshot checksum_raw line expected='' actual='' name digest matches=0
  local sha256_bin='/usr/bin/sha256sum'

  release_dir="${release_path%/*}"
  [[ -n "${release_dir}" ]] || release_dir='/'
  release_name="${release_path##*/}"
  checksum_path="${release_dir%/}/SHA256SUMS"

  redgres_open_trusted_readonly '--release' "${release_path}" release_fd release_size
  redgres_open_trusted_readonly 'SHA256SUMS' "${checksum_path}" checksum_fd checksum_size
  if [[ "${checksum_size}" -gt 65536 ]]; then
    redgres_close_trusted_fd "${checksum_fd}"
    redgres_close_trusted_fd "${release_fd}"
    redgres_die 'SHA256SUMS is too large'
  fi

  checksum_snapshot="$(/usr/bin/head -c "${checksum_size}" <&"${checksum_fd}"; builtin printf 'x')" || redgres_die 'SHA256SUMS is not readable'
  redgres_close_trusted_fd "${checksum_fd}"
  checksum_raw="${checksum_snapshot%x}"
  [[ "${#checksum_raw}" -eq "${checksum_size}" ]] || redgres_die 'SHA256SUMS contains NUL'

  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line%$'\r'}"
    [[ -n "${line}" ]] || continue
    if [[ ! "${line}" =~ ^([0-9a-f]{64})\ \ ([A-Za-z0-9._-]+)$ ]]; then
      redgres_close_trusted_fd "${release_fd}"
      redgres_die 'SHA256SUMS has invalid syntax'
    fi
    digest="${BASH_REMATCH[1]}"
    name="${BASH_REMATCH[2]}"
    [[ "${name}" != '.' && "${name}" != '..' ]] || redgres_die 'SHA256SUMS has an unsafe member name'
    if [[ "${name}" == "${release_name}" ]]; then
      expected="${digest}"
      matches=$((matches + 1))
    fi
  done <<< "${checksum_raw}"
  [[ "${matches}" -eq 1 ]] || redgres_die 'SHA256SUMS must contain exactly one release entry'

  redgres_validate_trusted_path 'sha256sum' "${sha256_bin}" executable
  actual="$("${sha256_bin}" <&"${release_fd}")" || redgres_die 'release checksum could not be computed'
  redgres_close_trusted_fd "${release_fd}"
  actual="${actual%% *}"
  [[ "${actual}" =~ ^[0-9a-f]{64}$ && "${actual}" == "${expected}" ]] || redgres_die 'release checksum verification failed'
}

# PATH is validated again and read only for SHA-256 verification. It is never
# extracted or printed in this Partial.
redgres_update_dry_run() {
  local release_path="$1"
  redgres_update_verify_checksum "${release_path}"
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
