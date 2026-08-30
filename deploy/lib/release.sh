#!/usr/bin/env bash
# Fail-closed update/rollback (OPS-005 Partial → live application binary path).
# Never source, eval, or print operator --release contents.
# Never print --to VERSION. Never reverse PostgreSQL/Redis/vault/credentials/DNS/schema.
# Live update extracts only the application binary under /opt/redgres (or REDGRES_OPT_ROOT).
set -euo pipefail

REDGRES_OPT_ROOT="${REDGRES_OPT_ROOT:-/opt/redgres}"
REDGRES_UNIT_PATH="${REDGRES_UNIT_PATH:-/etc/systemd/system/redgres.service}"
REDGRES_HEALTHZ_URL="${REDGRES_HEALTHZ_URL:-http://127.0.0.1:8790/api/v1/healthz}"

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
  local sha256_bin
  local -a sha256_prefix

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

  redgres_ensure_trusted_sha256sum || redgres_die 'trusted sha256sum is unavailable'
  sha256_bin="${REDGRES_SHA256SUM_BIN}"
  sha256_prefix=("${REDGRES_SHA256SUM_PREFIX_ARGS[@]}")
  redgres_validate_trusted_path 'sha256sum' "${sha256_bin}" executable
  actual="$("${sha256_bin}" "${sha256_prefix[@]}" <&"${release_fd}")" || redgres_die 'release checksum could not be computed'
  redgres_close_trusted_fd "${release_fd}"
  actual="${actual%% *}"
  [[ "${actual}" =~ ^[0-9a-f]{64}$ && "${actual}" == "${expected}" ]] || redgres_die 'release checksum verification failed'
}

# PATH is validated again and read only for SHA-256 verification. It is never
# extracted or printed in this Partial dry-run.
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

redgres_opt_releases_dir() {
  printf '%s/releases' "${REDGRES_OPT_ROOT}"
}

# Installer umask is 077, so mkdir would be 0700 and User=redgres could not exec.
redgres_chmod_opt_layout() {
  local dest="$1"
  local root="${REDGRES_OPT_ROOT}"
  local releases
  releases="$(redgres_opt_releases_dir)"
  /usr/bin/mkdir -p "${releases}" "${dest}"
  /usr/bin/chmod 755 "${root}" "${releases}" "${dest}"
  if [[ "${EUID}" -eq 0 ]]; then
    /usr/bin/chown root:root "${root}" "${releases}" "${dest}"
  fi
}

redgres_opt_current_link() {
  printf '%s/current' "${REDGRES_OPT_ROOT}"
}

redgres_version_from_release_basename() {
  local base="$1"
  # redgres_0.1.0_linux_amd64.tar.gz or redgres_v0.1.0_linux_amd64.tar.gz
  if [[ "${base}" =~ ^redgres_(v?[0-9][A-Za-z0-9._-]*)_linux_amd64\.tar\.gz$ ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
    return 0
  fi
  return 1
}

redgres_read_version_file() {
  local version_path="$1"
  local raw
  [[ -f "${version_path}" && ! -L "${version_path}" ]] || redgres_die 'VERSION file is missing'
  raw="$(/usr/bin/tr -d '\r\n' <"${version_path}")" || redgres_die 'VERSION file is not readable'
  redgres_rollback_version_ok "${raw}" || redgres_die 'VERSION file is not a path-safe token'
  printf '%s' "${raw}"
}

redgres_extract_release_staging() {
  local release_path="$1"
  local staging="$2"
  local tar_bin='/usr/bin/tar'
  redgres_validate_trusted_path 'tar' "${tar_bin}" executable
  /usr/bin/mkdir -p "${staging}"
  # Extract only expected member names; refuse absolute/parent paths via --restrict if available.
  if "${tar_bin}" --help 2>&1 | /usr/bin/grep -q -- '--restrict'; then
    "${tar_bin}" --restrict -xzf "${release_path}" -C "${staging}"
  else
    "${tar_bin}" -xzf "${release_path}" -C "${staging}"
  fi
}

redgres_locate_extracted_binary() {
  local staging="$1"
  if [[ -f "${staging}/redgres" && -x "${staging}/redgres" && ! -L "${staging}/redgres" ]]; then
    printf '%s' "${staging}/redgres"
    return 0
  fi
  local nested
  nested="$(/usr/bin/find "${staging}" -maxdepth 2 -type f -name redgres 2>/dev/null | /usr/bin/head -n 1 || true)"
  [[ -n "${nested}" && -x "${nested}" && ! -L "${nested}" ]] || redgres_die 'release archive is missing redgres binary'
  printf '%s' "${nested}"
}

redgres_locate_extracted_version() {
  local staging="$1"
  local bin_dir version_path
  if [[ -f "${staging}/VERSION" && ! -L "${staging}/VERSION" ]]; then
    printf '%s' "${staging}/VERSION"
    return 0
  fi
  bin_dir="$(/usr/bin/dirname "$(redgres_locate_extracted_binary "${staging}")")"
  version_path="${bin_dir}/VERSION"
  [[ -f "${version_path}" && ! -L "${version_path}" ]] || redgres_die 'release archive is missing VERSION'
  printf '%s' "${version_path}"
}

redgres_write_unit_file() {
  local binary_path="$1"
  local unit_path="${REDGRES_UNIT_PATH}"
  /usr/bin/mkdir -p "$(/usr/bin/dirname "${unit_path}")"
  if declare -F redgres_app_unit_body >/dev/null 2>&1; then
    redgres_app_unit_body "${binary_path}" >"${unit_path}"
    return 0
  fi
  cat >"${unit_path}" <<EOF
[Unit]
Description=Redgres control plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=redgres
Group=redgres
UMask=0077
EnvironmentFile=-/etc/redgres/redgres.env
ExecStart=${binary_path} serve
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/redgres /etc/redgres

[Install]
WantedBy=multi-user.target
EOF
}

redgres_probe_healthz() {
  local curl_bin='/usr/bin/curl'
  local i
  if [[ "${REDGRES_SKIP_HEALTHZ:-}" == "1" ]]; then
    redgres_log 'health_gate: skipped (REDGRES_SKIP_HEALTHZ=1)'
    return 0
  fi
  if [[ ! -x "${curl_bin}" ]]; then
    redgres_log 'health_gate: curl unavailable; skipped'
    return 0
  fi
  for i in 1 2 3 4 5 6 7 8 9 10; do
    if "${curl_bin}" -fsS --connect-timeout 2 --max-time 5 "${REDGRES_HEALTHZ_URL}" >/dev/null 2>&1; then
      redgres_log 'health_gate: ok'
      return 0
    fi
    /usr/bin/sleep 1
  done
  redgres_die 'health_gate: GET /api/v1/healthz failed'
}

redgres_systemctl_try() {
  if command -v systemctl >/dev/null 2>&1; then
    systemctl "$@" || true
  fi
}

# Live application update: checksum → extract → /opt/redgres/releases/<ver> → current → unit → restart → healthz.
# Does not install PostgreSQL/Redis packages. Does not reverse data.
redgres_update_apply() {
  local release_path="$1"
  local staging version dest binary_src version_src previous='' current_link releases_dir

  redgres_update_verify_checksum "${release_path}"

  releases_dir="$(redgres_opt_releases_dir)"
  current_link="$(redgres_opt_current_link)"
  /usr/bin/mkdir -p "${releases_dir}"

  if [[ -L "${current_link}" ]]; then
    previous="$(/usr/bin/readlink -f "${current_link}" 2>/dev/null || true)"
  fi

  staging="$(/usr/bin/mktemp -d "${TMPDIR:-/tmp}/redgres-update.XXXXXX")"
  # shellcheck disable=SC2064
  trap "rm -rf '${staging}'" EXIT

  redgres_extract_release_staging "${release_path}" "${staging}"
  binary_src="$(redgres_locate_extracted_binary "${staging}")"
  version_src="$(redgres_locate_extracted_version "${staging}")"
  version="$(redgres_read_version_file "${version_src}")"

  dest="${releases_dir}/${version}"
  if [[ -e "${dest}" ]]; then
    redgres_die 'release version directory already exists'
  fi
  redgres_chmod_opt_layout "${dest}"
  /usr/bin/install -m 0755 "${binary_src}" "${dest}/redgres"
  /usr/bin/install -m 0644 "${version_src}" "${dest}/VERSION"
  if [[ -f "${staging}/SHA256SUMS" ]]; then
    /usr/bin/install -m 0644 "${staging}/SHA256SUMS" "${dest}/SHA256SUMS" || true
  fi

  /usr/bin/ln -sfn "${dest}" "${current_link}"
  redgres_write_unit_file "${current_link}/redgres"
  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && declare -F redgres_ensure_domain_secret_env >/dev/null 2>&1; then
    if [[ -f /etc/redgres/redgres.env ]]; then
      redgres_ensure_domain_secret_env /etc/redgres/redgres.env || true
    fi
  fi
  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && declare -F redgres_install_domain_runtime >/dev/null 2>&1; then
    if [[ "${REDGRES_DOMAIN_RUNTIME_OPTIONAL:-}" == "1" ]]; then
      redgres_install_domain_runtime || redgres_log 'domain runtime: units/packages not fully applied'
    else
      redgres_install_domain_runtime || redgres_die 'domain runtime units or packages failed'
    fi
  fi

  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable redgres.service >/dev/null 2>&1 || true
    systemctl restart redgres.service || {
      if [[ -n "${previous}" && -d "${previous}" ]]; then
        /usr/bin/ln -sfn "${previous}" "${current_link}"
        redgres_write_unit_file "${current_link}/redgres"
        systemctl daemon-reload || true
        systemctl restart redgres.service || true
      fi
      redgres_die 'systemd restart failed; previous current restored if available'
    }
  fi

  redgres_probe_healthz
  rm -rf "${staging}"
  trap - EXIT
  cat <<EOF
Update applied:
release: extracted
checksum: verified (adjacent SHA256SUMS; signature/provenance not verified)
symlink: switched
sqlite_migrate: deferred to serve startup
systemd: unit written/restarted when managing /opt/redgres
health_gate: probed
postgres_packages: skipped (not part of application update)
data_reversal: skipped (never reverses PostgreSQL/Redis/vault/credentials/DNS/schema)
result=applied
EOF
}

redgres_rollback_apply() {
  local version="$1"
  local dest current_link

  redgres_rollback_version_ok "${version}" || redgres_die '--to must be a path-safe version token'
  dest="$(redgres_opt_releases_dir)/${version}"
  [[ -d "${dest}" && -x "${dest}/redgres" ]] || redgres_die 'rollback target release is missing'
  current_link="$(redgres_opt_current_link)"
  /usr/bin/ln -sfn "${dest}" "${current_link}"
  redgres_write_unit_file "${current_link}/redgres"
  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl restart redgres.service || redgres_die 'systemd restart failed after rollback'
  fi
  redgres_probe_healthz
  cat <<'EOF'
Rollback applied:
symlink: switched
schema_compat: not checked (operator responsibility)
systemd: unit rewritten/restarted when managing /opt/redgres
health_gate: probed
data_reversal: skipped (rollback never reverses PostgreSQL/Redis/vault/credentials/DNS/schema automatically)
result=applied
EOF
}
