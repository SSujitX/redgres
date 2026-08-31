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
  redgres_log 'health_gate: GET /api/v1/healthz failed'
  return 1
}

redgres_snapshot_rollback_runtime() {
  local release_dir="$1" snapshot tmp name path
  [[ -n "${release_dir}" && -d "${release_dir}" ]] || return 0
  snapshot="${release_dir}/.rollback-runtime"
  [[ ! -e "${snapshot}" ]] || return 0
  tmp="$(/usr/bin/mktemp -d "${release_dir}/.rollback-runtime.XXXXXX")" || return 1
  while IFS='|' read -r name path; do
    path="${REDGRES_RUNTIME_ROOT_PREFIX:-}${path}"
    if [[ "${name}" == "env" && -f "${path}" && ! -L "${path}" ]]; then
      if [[ "$(grep -c '^REDGRES_TLS_ISSUE_RESULT_FILE=' "${path}" || true)" -gt 1 ]]; then
        rm -rf -- "${tmp}"
        return 1
      fi
      if grep '^REDGRES_TLS_ISSUE_RESULT_FILE=' "${path}" >"${tmp}/${name}"; then
        /usr/bin/chmod 0600 "${tmp}/${name}"
      else
        : >"${tmp}/${name}.absent"
      fi
    elif [[ -f "${path}" && ! -L "${path}" ]]; then
      /usr/bin/cp --preserve=mode,ownership,timestamps -- "${path}" "${tmp}/${name}" || return 1
    else
      : >"${tmp}/${name}.absent"
    fi
  done <<'EOF'
env|/etc/redgres/redgres.env
issue-helper|/usr/libexec/redgres/issue-tls.sh
renew-hook|/etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh
issue-service|/etc/systemd/system/redgres-tls-issue.service
issue-path|/etc/systemd/system/redgres-tls-issue.path
EOF
  /usr/bin/chown -R root:root "${tmp}" 2>/dev/null || true
  /usr/bin/mv -T -- "${tmp}" "${snapshot}"
}

redgres_restore_rollback_runtime() {
  local release_dir="$1" snapshot name path source tmp txn index commit_count=0 fail_after
  local -a restore_paths=() restore_staged=() current_backups=() current_present=()
  snapshot="${release_dir}/.rollback-runtime"
  [[ -d "${snapshot}" && ! -L "${snapshot}" ]] || return 2
  # Validate the complete snapshot before changing any live file. A truncated
  # snapshot must never produce a half-restored root runtime.
  while IFS='|' read -r name path; do
    source="${snapshot}/${name}"
    if [[ -f "${source}" && ! -L "${source}" ]]; then
      [[ ! -e "${snapshot}/${name}.absent" ]] || return 1
    elif [[ -f "${snapshot}/${name}.absent" && ! -L "${snapshot}/${name}.absent" ]]; then
      :
    else
      return 1
    fi
  done <<'EOF'
env|/etc/redgres/redgres.env
issue-helper|/usr/libexec/redgres/issue-tls.sh
renew-hook|/etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh
issue-service|/etc/systemd/system/redgres-tls-issue.service
issue-path|/etc/systemd/system/redgres-tls-issue.path
EOF
  txn="$(/usr/bin/mktemp -d "${snapshot}/.restore.XXXXXX")" || return 1
  index=0
  while IFS='|' read -r name path; do
    path="${REDGRES_RUNTIME_ROOT_PREFIX:-}${path}"
    source="${snapshot}/${name}"
    [[ ! -L "${path}" && ( ! -e "${path}" || -f "${path}" ) ]] || { rm -rf -- "${txn}"; return 1; }
    restore_paths+=("${path}")
    if [[ -f "${path}" ]]; then
      if ! /usr/bin/cp --preserve=mode,ownership,timestamps -- "${path}" "${txn}/current-${index}"; then
        rm -rf -- "${txn}"
        return 1
      fi
      current_backups+=("${txn}/current-${index}")
      current_present+=("1")
    else
      current_backups+=("")
      current_present+=("0")
    fi
    if [[ "${name}" == "env" ]]; then
      if [[ -f "${path}" ]]; then
        /usr/bin/mkdir -p "${path%/*}"
        tmp="$(/usr/bin/mktemp "${path}.rollback-stage.XXXXXX")" || { rm -rf -- "${txn}"; return 1; }
        if ! /usr/bin/awk -v replacement="$([[ -f "${source}" ]] && /usr/bin/head -n1 "${source}" || true)" '
          BEGIN { written = 0 }
          /^REDGRES_TLS_ISSUE_RESULT_FILE=/ {
            if (!written && replacement != "") print replacement
            written = 1
            next
          }
          { print }
          END { if (!written && replacement != "") print replacement }
        ' "${path}" >"${tmp}" ||
          ! /usr/bin/chmod --reference="${path}" "${tmp}" ||
          ! /usr/bin/chown --reference="${path}" "${tmp}"; then
          rm -f -- "${tmp}"
          rm -rf -- "${txn}"
          return 1
        fi
        restore_staged+=("${tmp}")
      elif [[ -f "${source}" ]]; then
        rm -rf -- "${txn}"
        return 1
      else
        restore_staged+=("")
      fi
    elif [[ -f "${source}" && ! -L "${source}" ]]; then
      /usr/bin/mkdir -p "${path%/*}"
      tmp="$(/usr/bin/mktemp "${path}.rollback-stage.XXXXXX")" || { rm -rf -- "${txn}"; return 1; }
      if ! /usr/bin/cp --preserve=mode,ownership,timestamps -- "${source}" "${tmp}"; then
        rm -f -- "${tmp}"
        rm -rf -- "${txn}"
        return 1
      fi
      restore_staged+=("${tmp}")
    else
      restore_staged+=("")
    fi
    index=$((index + 1))
  done <<'EOF'
env|/etc/redgres/redgres.env
issue-helper|/usr/libexec/redgres/issue-tls.sh
renew-hook|/etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh
issue-service|/etc/systemd/system/redgres-tls-issue.service
issue-path|/etc/systemd/system/redgres-tls-issue.path
EOF
  fail_after="${REDGRES_RUNTIME_RESTORE_FAIL_AFTER:-0}"
  [[ "${fail_after}" =~ ^[0-9]+$ ]] || fail_after=0
  [[ -z "${REDGRES_RUNTIME_ROOT_PREFIX:-}" ]] && fail_after=0
  for index in "${!restore_paths[@]}"; do
    path="${restore_paths[$index]}"
    if [[ "${fail_after}" -gt 0 && "${commit_count}" -eq "${fail_after}" ]]; then
      break
    fi
    if [[ -n "${restore_staged[$index]}" ]]; then
      /usr/bin/mv -fT -- "${restore_staged[$index]}" "${path}" || break
      restore_staged[$index]=""
    else
      rm -f -- "${path}" || break
    fi
    commit_count=$((commit_count + 1))
  done
  if [[ "${commit_count}" -ne "${#restore_paths[@]}" ]]; then
    for index in "${!restore_paths[@]}"; do
      path="${restore_paths[$index]}"
      if [[ "${current_present[$index]}" == "1" ]]; then
        tmp="$(/usr/bin/mktemp "${path}.rollback-current.XXXXXX")" || continue
        if /usr/bin/cp --preserve=mode,ownership,timestamps -- "${current_backups[$index]}" "${tmp}"; then
          /usr/bin/mv -fT -- "${tmp}" "${path}" || rm -f -- "${tmp}"
        else
          rm -f -- "${tmp}"
        fi
      else
        rm -f -- "${path}"
      fi
    done
    for tmp in "${restore_staged[@]}"; do
      [[ -z "${tmp}" ]] || rm -f -- "${tmp}"
    done
    rm -rf -- "${txn}"
    return 1
  fi
  rm -rf -- "${txn}"
}

redgres_restore_previous_release() {
  local previous="$1" current_link="$2"
  [[ -n "${previous}" && -d "${previous}" ]] || return 1
  redgres_restore_rollback_runtime "${previous}" || return 1
  /usr/bin/ln -sfn "${previous}" "${current_link}"
  redgres_write_unit_file "${current_link}/redgres"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl restart redgres.service || return 1
    if systemctl cat redgres-tls-issue.path >/dev/null 2>&1; then
      systemctl enable --now redgres-tls-issue.path >/dev/null 2>&1 || return 1
      systemctl is-active --quiet redgres-tls-issue.path || return 1
    fi
  fi
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
  REDGRES_UPDATE_GUARD=0
  REDGRES_UPDATE_PREVIOUS=""
  REDGRES_UPDATE_CURRENT_LINK=""
  REDGRES_UPDATE_STAGING="${staging}"
  trap 'rc=$?; if [[ "${REDGRES_UPDATE_GUARD:-0}" == "1" ]]; then redgres_restore_previous_release "${REDGRES_UPDATE_PREVIOUS}" "${REDGRES_UPDATE_CURRENT_LINK}" || true; fi; rm -rf -- "${REDGRES_UPDATE_STAGING}"; exit "${rc}"' EXIT

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

  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" && -n "${previous}" ]]; then
    redgres_snapshot_rollback_runtime "${previous}" || redgres_die 'could not snapshot version-matched rollback runtime'
  fi

  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && declare -F redgres_adopt_legacy_vault_secret >/dev/null 2>&1; then
    redgres_adopt_legacy_vault_secret || redgres_die 'legacy vault migration requires root authorization: ensure /etc/redgres/secrets is root:redgres 0750, then create legacy-vault-secret.adopt as root:root 0600 containing only the absolute legacy source path and retry'
  fi

  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && command -v systemctl >/dev/null 2>&1; then
    systemctl stop redgres-tls-issue.path >/dev/null 2>&1 || true
    systemctl stop redgres-tls-issue.service >/dev/null 2>&1 || true
    if ! systemctl stop redgres.service >/dev/null 2>&1 || systemctl is-active --quiet redgres.service; then
      systemctl start redgres.service >/dev/null 2>&1 || true
      systemctl cat redgres-tls-issue.path >/dev/null 2>&1 && systemctl start redgres-tls-issue.path >/dev/null 2>&1 || true
      redgres_die 'could not quiesce application and TLS helper before update'
    fi
    if systemctl is-active --quiet redgres-tls-issue.path || systemctl is-active --quiet redgres-tls-issue.service; then
      systemctl start redgres.service >/dev/null 2>&1 || true
      systemctl cat redgres-tls-issue.path >/dev/null 2>&1 && systemctl start redgres-tls-issue.path >/dev/null 2>&1 || true
      redgres_die 'could not quiesce application and TLS helper before update'
    fi
    REDGRES_UPDATE_GUARD=1
    REDGRES_UPDATE_PREVIOUS="${previous}"
    REDGRES_UPDATE_CURRENT_LINK="${current_link}"
  fi

  /usr/bin/ln -sfn "${dest}" "${current_link}"
  redgres_write_unit_file "${current_link}/redgres"
  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && declare -F redgres_ensure_domain_secret_env >/dev/null 2>&1; then
    if [[ -f /etc/redgres/redgres.env ]]; then
      redgres_ensure_domain_secret_env /etc/redgres/redgres.env || true
    fi
  fi
  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && declare -F redgres_ensure_expert_tools >/dev/null 2>&1; then
    if command -v docker >/dev/null 2>&1; then
      if [[ "${REDGRES_EXPERT_TOOLS_IF_MANAGED:-}" == "1" && ! -f /etc/redgres/expert-tools-compose.yml ]]; then
        redgres_log 'expert tools: not managed on this host; skipped'
      elif [[ "${REDGRES_EXPERT_TOOLS_OPTIONAL:-}" == "1" ]]; then
        redgres_ensure_expert_tools || redgres_log 'expert tools: compose not fully applied'
      else
        redgres_ensure_expert_tools || redgres_die 'expert tools compose failed'
      fi
    fi
  fi
  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && declare -F redgres_install_domain_runtime >/dev/null 2>&1; then
    redgres_install_domain_runtime || redgres_die 'domain runtime failed; previous binary, config, and TLS helper restored if available'
  fi
  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable redgres.service >/dev/null 2>&1 || true
    systemctl restart redgres.service || {
      redgres_die 'systemd restart failed; previous current restored if available'
    }
  fi

  if ! redgres_probe_healthz; then
    redgres_die 'health gate failed; previous binary, config, and TLS helper restored if available'
  fi
  REDGRES_UPDATE_GUARD=0
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
  local dest current_link current_previous=''

  redgres_rollback_version_ok "${version}" || redgres_die '--to must be a path-safe version token'
  dest="$(redgres_opt_releases_dir)/${version}"
  [[ -d "${dest}" && -x "${dest}/redgres" ]] || redgres_die 'rollback target release is missing'
  current_link="$(redgres_opt_current_link)"
  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]]; then
    current_previous="$(/usr/bin/readlink -f "${current_link}" 2>/dev/null || true)"
    if [[ -n "${current_previous}" && -d "${current_previous}" ]]; then
      redgres_snapshot_rollback_runtime "${current_previous}" || redgres_die 'could not snapshot current runtime before rollback'
    fi
    if command -v systemctl >/dev/null 2>&1; then
      systemctl stop redgres-tls-issue.path >/dev/null 2>&1 || true
      systemctl stop redgres-tls-issue.service >/dev/null 2>&1 || true
      if ! systemctl stop redgres.service >/dev/null 2>&1 || systemctl is-active --quiet redgres.service ||
        systemctl is-active --quiet redgres-tls-issue.path || systemctl is-active --quiet redgres-tls-issue.service; then
        systemctl start redgres.service >/dev/null 2>&1 || true
        systemctl cat redgres-tls-issue.path >/dev/null 2>&1 && systemctl start redgres-tls-issue.path >/dev/null 2>&1 || true
        redgres_die 'could not quiesce application and TLS helper before rollback'
      fi
    fi
    REDGRES_ROLLBACK_GUARD=1
    REDGRES_ROLLBACK_PREVIOUS="${current_previous}"
    REDGRES_ROLLBACK_CURRENT_LINK="${current_link}"
    trap 'rc=$?; if [[ "${REDGRES_ROLLBACK_GUARD:-0}" == "1" ]]; then redgres_restore_previous_release "${REDGRES_ROLLBACK_PREVIOUS}" "${REDGRES_ROLLBACK_CURRENT_LINK}" || true; fi; exit "${rc}"' EXIT
    if ! redgres_restore_rollback_runtime "${dest}"; then
      redgres_die 'rollback target lacks a version-matched config and TLS-helper snapshot'
    fi
  fi
  /usr/bin/ln -sfn "${dest}" "${current_link}"
  redgres_write_unit_file "${current_link}/redgres"
  if [[ "${REDGRES_OPT_ROOT}" == "/opt/redgres" ]] && command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl restart redgres.service || redgres_die 'systemd restart failed after rollback'
    if systemctl cat redgres-tls-issue.path >/dev/null 2>&1; then
      systemctl enable --now redgres-tls-issue.path >/dev/null 2>&1 || redgres_die 'TLS request path failed after rollback'
    fi
  fi
  redgres_probe_healthz || redgres_die 'health gate failed after rollback'
  REDGRES_ROLLBACK_GUARD=0
  trap - EXIT
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
