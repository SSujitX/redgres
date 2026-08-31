#!/usr/bin/env bash
# Redgres uninstaller (DockLift-parity UX).
# Default: remove every Redgres-owned resource so the host is clean for a fresh install.
#
# Non-interactive (recommended for scripts):
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash -s -- -y
# Interactive (y/n prompt):
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash
#
# App binary only (preserve PostgreSQL, Redis, config, and data):
#   curl ... | sudo bash -s -- -y --app-only [--purge-config] [--purge-state]
set -uo pipefail
PATH=/usr/sbin:/usr/bin:/sbin:/bin

# Git clone detection (sourced by tests with REDGRES_UNINSTALL_FUNCTIONS_ONLY=1).
redgres_uninstall_is_git_checkout() {
  local dir="$1"
  [[ -n "${dir}" && "${dir}" == /* ]] || return 1
  case "${dir}" in
    /|/root|/home|/tmp|/opt|/usr|/var|/etc|/bin|/sbin) return 1 ;;
    /tmp/*|/opt/redgres|/opt/redgres/*) return 1 ;;
    */./*|*/../*|*/.|*/..) return 1 ;;
  esac
  [[ ! -L "${dir}" && -d "${dir}" ]] || return 1
  [[ -d "${dir}/.git" || -f "${dir}/.git" ]] || return 1
  [[ -f "${dir}/deploy/install.sh" && -f "${dir}/uninstall.sh" ]] || return 1
  return 0
}

redgres_uninstall_collect_git_checkouts() {
  local script="${BASH_SOURCE[0]:-}"
  local script_dir="" dir
  local -a found=() out=()
  local seen dup

  if [[ -n "${script}" ]]; then
    if [[ "${script}" == /* ]]; then
      script_dir="${script%/*}"
    else
      script_dir="$(pwd -P)/${script%/*}"
      script_dir="${script_dir%/.}"
    fi
    if [[ -d "${script_dir}" ]]; then
      script_dir="$(cd "${script_dir}" && pwd -P)" || script_dir=""
    else
      script_dir=""
    fi
    if redgres_uninstall_is_git_checkout "${script_dir}"; then
      found+=("${script_dir}")
    fi
  fi

  for dir in /root/redgres "${HOME}/redgres"; do
    if redgres_uninstall_is_git_checkout "${dir}"; then
      found+=("${dir}")
    fi
  done
  if [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" && "${SUDO_USER}" != */* && "${SUDO_USER}" != *".."* ]]; then
    dir="/home/${SUDO_USER}/redgres"
    if redgres_uninstall_is_git_checkout "${dir}"; then
      found+=("${dir}")
    fi
  fi

  for dir in "${found[@]}"; do
    dup=0
    for seen in "${out[@]+"${out[@]}"}"; do
      [[ "${seen}" == "${dir}" ]] && dup=1 && break
    done
    [[ "${dup}" -eq 0 ]] && out+=("${dir}")
  done
  if [[ "${#out[@]}" -gt 0 ]]; then
    printf '%s\n' "${out[@]}"
  fi
}

redgres_uninstall_path_footprint_present() {
  local path
  for path in "$@"; do
    if [[ -e "${path}" || -L "${path}" ]]; then
      return 0
    fi
  done
  return 1
}

redgres_uninstall_local_footprint_present() {
  local installed dpkg_rc=0 docker_rows docker_part socket_rows listeners ufw_rows
  redgres_uninstall_path_footprint_present "$@" && return 0
  id redgres >/dev/null 2>&1 && return 0
  getent group redgres >/dev/null 2>&1 && return 0
  if command -v dpkg-query >/dev/null 2>&1; then
    installed="$(dpkg-query -W -f='${binary:Package} ${db:Status-Status}\n' \
      'postgresql*' 'redis*' pgbouncer cloudflared 2>/dev/null)" || dpkg_rc=$?
    (( dpkg_rc <= 1 )) || return 0
    if printf '%s\n' "${installed}" | awk '$2 == "installed" { found=1 } END { exit found ? 0 : 1 }'; then
      return 0
    fi
  fi
  if command -v docker >/dev/null 2>&1; then
    docker info >/dev/null 2>&1 || return 0
    docker_rows=""
    docker_part="$(docker ps -a --format '{{.Names}}|{{.Label "com.docker.compose.project"}}' 2>/dev/null)" || return 0
    docker_rows+="${docker_part}"$'\n'
    docker_part="$(docker volume ls --format '{{.Name}}|{{.Labels}}' 2>/dev/null)" || return 0
    docker_rows+="${docker_part}"$'\n'
    docker_part="$(docker network ls --format '{{.Name}}|{{.Labels}}' 2>/dev/null)" || return 0
    docker_rows+="${docker_part}"
    docker_rows="$(printf '%s\n' "${docker_rows}" | awk '$0 ~ /^(redgres|redis-insight|redisinsight|pgadmin-redgres|dpage\/pgadmin4|redis\/redisinsight)/ || $0 ~ /com\.docker\.compose\.project=redgres/')"
    [[ -n "${docker_rows}" ]] && return 0
  fi
  if command -v ss >/dev/null 2>&1; then
    socket_rows="$(ss -tlnH 2>/dev/null)" || return 0
    listeners="$(printf '%s\n' "${socket_rows}" | awk '{print $4}' | grep -E ':(8790|8989|5432|6379|6380|6432|5542|5052)$' || true)"
    [[ -n "${listeners}" ]] && return 0
  fi
  if command -v ufw >/dev/null 2>&1; then
    ufw_rows="$(ufw status 2>/dev/null)" || return 0
    if printf '%s\n' "${ufw_rows}" | grep -E '(^|[[:space:]])8989(/tcp)?([[:space:]]|$)' | grep -Fvq 'redgres-bootstrap'; then
      return 0
    fi
  fi
  return 1
}

redgres_uninstall_exit_if_already_removed() {
  local app_only="$1"
  shift
  [[ "${app_only}" == "0" ]] || return 0
  redgres_uninstall_local_footprint_present "$@" && return 0
  printf '%s\n' \
    'No installed Redgres footprint was detected; only tagged bootstrap firewall rules and verified repository checkouts will be removed.' \
    'Package, service, database, and remote cleanup will not be attempted.' \
    'Cloudflare ownership cannot be verified without saved domain state. Check manually:' \
    '  DNS      https://dash.cloudflare.com/' \
    '  Access   https://one.dash.cloudflare.com/access/applications' \
    '  Tunnels  https://one.dash.cloudflare.com/networks/tunnels'
  return 10
}

# Leftover cluster dirs after apt purge. dpkg will not remove non-empty
# /etc/postgresql or /var/log/postgresql; a shell cwd there also blocks rmdir.
redgres_uninstall_postgres_leftover_dirs() {
  local root="${1:-}"
  printf '%s\n' \
    "${root}/etc/postgresql" \
    "${root}/etc/postgresql-common" \
    "${root}/var/log/postgresql" \
    "${root}/var/lib/postgresql"
}

redgres_uninstall_remove_postgres_leftovers() {
  local root="${1:-}" dir
  while IFS= read -r dir || [[ -n "${dir}" ]]; do
    [[ -n "${dir}" ]] || continue
    rm -rf "${dir}" 2>/dev/null || true
  done < <(redgres_uninstall_postgres_leftover_dirs "${root}")
}

# Uninstall reconnects stdin to /dev/tty. needrestart ignores DEBIAN_FRONTEND
# and will prompt "Restart services?" — that looks like a hang.
redgres_uninstall_export_apt_env() {
  export DEBIAN_FRONTEND=noninteractive
  export NEEDRESTART_MODE=l
  export NEEDRESTART_SUSPEND=1
  export APT_LISTCHANGES_FRONTEND=none
}

# apt/dpkg maintainer scripts call getcwd(). Deleting the clone while cwd is
# still inside it (or standing in /etc/postgresql) makes later apt look stuck.
redgres_uninstall_enter_safe_cwd() {
  cd / || cd /tmp || return 1
}

redgres_uninstall_drop_postgres_clusters() {
  local ver name _rest
  command -v pg_lsclusters >/dev/null 2>&1 || return 0
  while read -r ver name _rest; do
    [[ -n "${ver}" && -n "${name}" ]] || continue
    if command -v pg_ctlcluster >/dev/null 2>&1; then
      pg_ctlcluster "${ver}" "${name}" stop -m fast 2>/dev/null ||
        pg_ctlcluster "${ver}" "${name}" stop -m immediate 2>/dev/null || true
    fi
    pg_dropcluster --stop "${ver}" "${name}" 2>/dev/null || true
  done < <(pg_lsclusters --no-header 2>/dev/null || true)
}

# Operator command output must never echo passwords, URLs-with-userinfo, or AUTH.
redgres_uninstall_cmd_log_safe() {
  printf '%s\n' "$1" | awk 'BEGIN { IGNORECASE=1 }
    /requirepass|password|AUTH |masterauth/ { next }
    { gsub(/:\/\/[^\/[:space:]]+:[^@\/[:space:]]+@/, "://[redacted]@"); gsub(/token=[^[:space:]]+/, "token=[redacted]"); print }'
}

# Capture apt-get. Success is quiet; failure dumps a secret-safe tail.
# Callers still use || true so a purge miss does not abort the rest.
redgres_uninstall_apt_handle_log() {
  local log="$1" rc="$2"
  if [[ "${rc}" -ne 0 ]]; then
    printf '%s\n' 'apt-get failed' >&2
    redgres_uninstall_cmd_log_safe "$(tail -n 50 "${log}")" >&2
    return 1
  fi
  return 0
}

redgres_uninstall_pkg_installed() {
  local pkg="$1" status
  command -v dpkg-query >/dev/null 2>&1 || return 1
  status="$(dpkg-query -W -f '${Status}' "${pkg}" 2>/dev/null || true)"
  [[ "${status}" == *"install ok installed"* ]]
}

# Purge only packages that are installed. Missing optional packages stay quiet.
redgres_uninstall_purge_installed() {
  local -a pkgs=()
  local pkg
  for pkg in "$@"; do
    if redgres_uninstall_pkg_installed "${pkg}"; then
      pkgs+=("${pkg}")
    fi
  done
  [[ "${#pkgs[@]}" -gt 0 ]] || return 0
  redgres_uninstall_apt_get purge -y "${pkgs[@]}"
}

redgres_uninstall_purge_native_redis_packages() {
  redgres_uninstall_purge_installed redis-server redis redis-tools
}

redgres_uninstall_purge_postgresql_packages() {
  local -a pkgs=()
  local pkg status
  command -v dpkg-query >/dev/null 2>&1 || return 0
  while IFS=$'\t' read -r pkg status || [[ -n "${pkg}" ]]; do
    [[ "${pkg}" == postgresql || "${pkg}" == postgresql-* ]] || continue
    [[ "${status}" == *"install ok installed"* ]] || continue
    pkgs+=("${pkg}")
  done < <(dpkg-query -W -f '${Package}\t${Status}\n' 2>/dev/null || true)
  [[ "${#pkgs[@]}" -gt 0 ]] || return 0
  redgres_uninstall_apt_get purge -y "${pkgs[@]}"
}

redgres_uninstall_apt_get() {
  local log rc=0 apt
  apt="${REDGRES_UNINSTALL_APT_GET:-apt-get}"
  redgres_uninstall_export_apt_env
  # curl | bash re-execs with </dev/tty. apt/needrestart then SIGTTIN-stop (T+)
  # at the PostgreSQL purge summary and look hung. Never give them a TTY.
  if [[ "${REDGRES_UNINSTALL_VERBOSE:-${REDGRES_INSTALL_VERBOSE:-}}" == '1' ]]; then
    "${apt}" -o Dpkg::Use-Pty=0 -o APT::Get::Assume-Yes=true \
      -o Dpkg::Options::=--force-confdef \
      -o Dpkg::Options::=--force-confold \
      "$@" </dev/null 2>&1 | awk 'BEGIN { IGNORECASE=1 }
        /requirepass|password|AUTH |masterauth/ { next }
        { gsub(/:\/\/[^\/[:space:]]+:[^@\/[:space:]]+@/, "://[redacted]@"); gsub(/token=[^[:space:]]+/, "token=[redacted]"); print }'
    return "${PIPESTATUS[0]}"
  fi
  log="$(mktemp /tmp/redgres-uninstall-apt.XXXXXX)" || {
    printf '%s\n' 'apt-get capture failed (mktemp)' >&2
    return 1
  }
  # shellcheck disable=SC2064
  trap "rm -f $(printf '%q' "${log}")" RETURN
  "${apt}" -o Dpkg::Use-Pty=0 -o APT::Get::Assume-Yes=true \
    -o Dpkg::Options::=--force-confdef \
    -o Dpkg::Options::=--force-confold \
    "$@" </dev/null >"${log}" 2>&1 || rc=$?
  if ! redgres_uninstall_apt_handle_log "${log}" "${rc}"; then
    rm -f "${log}"
    return "${rc}"
  fi
  rm -f "${log}"
  return 0
}

redgres_uninstall_cloudflare_status_confirmed() {
  case "${1:-}" in
    api_ok|no_domain|manual_dns) return 0 ;;
    *) return 1 ;;
  esac
}

redgres_uninstall_restore_unit() {
  local unit="$1" attempt
  if ! systemctl --no-block start "${unit}" >/dev/null 2>&1; then
    printf '         warn: could not request restart for %s\n' "${unit}" >&2
    return 0
  fi
  for attempt in 1 2 3; do
    if systemctl is-active --quiet "${unit}"; then
      return 0
    fi
    if systemctl is-failed --quiet "${unit}"; then
      printf '         warn: %s failed while restoring pre-uninstall state\n' "${unit}" >&2
      return 0
    fi
    sleep 1
  done
  printf '         warn: %s restoration was not confirmed within 3 seconds\n' "${unit}" >&2
  return 0
}

redgres_uninstall_restore_quiesced() {
  [[ "${REDGRES_UNINSTALL_QUIESCE_GUARD:-0}" == "1" ]] || return 0
  if ! command -v systemctl >/dev/null 2>&1; then
    REDGRES_UNINSTALL_QUIESCE_GUARD=0
    return 0
  fi
  if [[ "${REDGRES_UNINSTALL_APP_WAS_ACTIVE:-0}" == "1" ]]; then
    redgres_uninstall_restore_unit redgres.service
  fi
  if [[ "${REDGRES_UNINSTALL_TLS_PATH_WAS_ACTIVE:-0}" == "1" ]]; then
    redgres_uninstall_restore_unit redgres-tls-issue.path
  fi
  if [[ "${REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE:-0}" == "1" ]]; then
    redgres_uninstall_restore_unit cloudflared-redgres.service
  fi
  if [[ "${REDGRES_UNINSTALL_TUNNEL_PATH_WAS_ACTIVE:-0}" == "1" ]]; then
    redgres_uninstall_restore_unit cloudflared-redgres.path
  fi
  REDGRES_UNINSTALL_QUIESCE_GUARD=0
  return 0
}

redgres_uninstall_quiesce_cloudflare() {
  local unit
  command -v systemctl >/dev/null 2>&1 || return 0
  REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE=0
  REDGRES_UNINSTALL_TUNNEL_PATH_WAS_ACTIVE=0
  systemctl is-active --quiet cloudflared-redgres.service && REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE=1
  systemctl is-active --quiet cloudflared-redgres.path && REDGRES_UNINSTALL_TUNNEL_PATH_WAS_ACTIVE=1
  systemctl stop cloudflared-redgres.path >/dev/null 2>&1 || true
  systemctl stop cloudflared-redgres.service >/dev/null 2>&1 || true
  for unit in cloudflared-redgres.path cloudflared-redgres.service; do
    if systemctl is-active --quiet "${unit}"; then
      printf '%s\n' 'Error: Cloudflare Tunnel connector could not be quiesced; cleanup aborted.' >&2
      return 1
    fi
  done
}

redgres_uninstall_apply_cloudflare_result() {
  local cf_out="$1"
  CF_TUNNEL_STATUS="$(printf '%s\n' "${cf_out}" | grep '^TUNNEL:' | tail -n1 | cut -d: -f2- || true)"
  CF_API_STATUS="$(printf '%s\n' "${cf_out}" | grep '^STATUS:' | tail -n1 | cut -d: -f2- || true)"
  CF_CLEANUP_SCOPE="$(printf '%s\n' "${cf_out}" | grep '^SCOPE:' | tail -n1 | cut -d: -f2- || true)"
  [[ -n "${CF_API_STATUS}" ]] || CF_API_STATUS="unknown"
  [[ -n "${CF_CLEANUP_SCOPE}" ]] || CF_CLEANUP_SCOPE="full"
  if [[ "${CF_TUNNEL_STATUS}" == "deleted" ]]; then
    REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE=0
    REDGRES_UNINSTALL_TUNNEL_PATH_WAS_ACTIVE=0
  fi
}

redgres_uninstall_delete_trusted_lineage() {
  local lineage_file="$1" certbot_bin="$2" metadata lineage cert_name expected='root:root:600'
  [[ -e "${lineage_file}" ]] || return 2
  [[ -f "${lineage_file}" && ! -L "${lineage_file}" ]] || return 1
  if [[ "${lineage_file}" != "/etc/redgres/tls-lineage" ]]; then
    expected="${REDGRES_UNINSTALL_LINEAGE_EXPECTED_METADATA:-${expected}}"
  fi
  metadata="$(/usr/bin/stat -c '%U:%G:%a' "${lineage_file}" 2>/dev/null || true)"
  [[ "${metadata}" == "${expected}" ]] || return 1
  lineage="$(/usr/bin/head -n1 "${lineage_file}")"
  case "${lineage}" in
    /etc/letsencrypt/live/*) ;;
    *) return 1 ;;
  esac
  cert_name="${lineage##*/}"
  [[ "${cert_name}" =~ ^[a-z0-9.-]+(-[0-9]+)?$ ]] || return 1
  [[ -x "${certbot_bin}" ]] || return 1
  "${certbot_bin}" delete --non-interactive --cert-name "${cert_name}" 2>/dev/null
}

if [[ "${REDGRES_UNINSTALL_FUNCTIONS_ONLY:-}" == "1" ]]; then
  return 0 2>/dev/null || exit 0
fi

REDGRES_UNINSTALL_URL="${REDGRES_UNINSTALL_URL:-https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh}"

# curl | bash leaves stdin on a pipe — re-run from a real file so confirm + heredocs work.
# With -y, do not attach /dev/tty: apt-get then SIGTTIN-stops on the PG purge prompt.
if [[ ! -t 0 && "${REDGRES_UNINSTALL_FROM_FILE:-}" != "1" ]]; then
  export REDGRES_UNINSTALL_FROM_FILE=1
  _uninstall_tmp="$(mktemp /tmp/redgres-uninstall.XXXXXX.sh)"
  if ! curl -fsSL "${REDGRES_UNINSTALL_URL}" -o "${_uninstall_tmp}"; then
    echo "Error: could not download uninstall script" >&2
    exit 1
  fi
  chmod 700 "${_uninstall_tmp}"
  _uninstall_force=0
  for _uninstall_arg in "$@"; do
    case "${_uninstall_arg}" in
      -y|--force) _uninstall_force=1 ;;
    esac
  done
  if [[ "${_uninstall_force}" -eq 1 || ! -e /dev/tty ]]; then
    exec bash "${_uninstall_tmp}" "$@" </dev/null
  fi
  exec bash "${_uninstall_tmp}" "$@" </dev/tty
fi

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

OPT_ROOT="/opt/redgres"
ETC_ROOT="/etc/redgres"
VAR_ROOT="/var/lib/redgres"
BACKUP_ROOT="/var/backups/redgres"
UNIT_PATH="/etc/systemd/system/redgres.service"
LIBEXEC_ROOT="/usr/libexec/redgres"

# Docker resources Redgres may create (never touch unrelated workloads).
DOCKER_NAME_RE='^(redgres|redgres-|redis-insight|redisinsight|pgadmin-redgres)'
DOCKER_PROJECT_RE='^redgres'

FORCE=0
APP_ONLY=0
PURGE_CONFIG=0
PURGE_STATE=0
KEEP_REMOTE=0

for arg in "$@"; do
  case "${arg}" in
    -y|--force) FORCE=1 ;;
    --app-only) APP_ONLY=1 ;;
    --purge-config) PURGE_CONFIG=1 ;;
    --purge-state) PURGE_STATE=1 ;;
    --keep-remote) KEEP_REMOTE=1 ;;
    --help|-h)
      cat <<EOF
Usage: uninstall.sh [-y|--force] [--app-only] [--keep-remote] [--purge-config] [--purge-state]

Default (no --app-only): full purge — application, config, SQLite state, Cloudflare
DNS/tunnel/Access (via stored API token + domain inventory, or reconstructible
local Redgres evidence when SQLite is missing), certbot db/rs certificates,
tunnel units, bootstrap firewall rule, Redgres Docker workloads, PostgreSQL,
Redis, PgBouncer, and git checkouts that look like this repository
(/root/redgres, ~/redgres, or the tree that contains this script).

--keep-remote: local purge only; skip Cloudflare API and certbot delete.
--app-only: remove only /opt/redgres and systemd units; databases preserved unless
            --purge-config / --purge-state are set.
EOF
      exit 0
      ;;
    *)
      printf '%b\n' "${RED}Unknown arg: ${arg}${NC}" >&2
      exit 1
      ;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { printf '%b\n' "${RED}Error: Run with sudo${NC}" >&2; exit 1; }

UNINSTALL_LOCAL_PATHS=(
  "${OPT_ROOT}" "${ETC_ROOT}" "${VAR_ROOT}" "${BACKUP_ROOT}" "${LIBEXEC_ROOT}"
  /var/lib/redgres-tls /var/lib/redgres-release /var/log/redgres /etc/ssl/redgres
  /etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh
  /usr/local/bin/redgres "${UNIT_PATH}"
  /etc/systemd/system/redgres-bootstrap-ufw-remove.service
  /etc/systemd/system/redgres-bootstrap-ufw-remove.path
  /etc/systemd/system/cloudflared-redgres.service
  /etc/systemd/system/cloudflared-redgres.path
  /etc/systemd/system/cloudflared-redgres-restart.service
  /etc/systemd/system/redgres-tls-issue.service
  /etc/systemd/system/redgres-tls-issue.path
)
printf '%b\n' ""
printf '%b\n' "  ${CYAN}${BOLD}Redgres uninstaller${NC}"
printf '%b\n' "  ${DIM}Removes Redgres from this host. Full purge is destructive.${NC}"
printf '%b\n' ""

step() { printf '%b' "$1"; }
step_done() { printf '%b\n' "${GREEN}done${NC}"; }
step_skip() { printf '%b\n' "${DIM}none${NC}"; }
count_lines() { [[ -z "${1:-}" ]] && { echo 0; return; }; grep -c . <<<"${1}" || echo 0; }

if [[ "${APP_ONLY}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}${BOLD}This removes the Redgres application binary and systemd unit.${NC}"
  printf '%b\n' "${DIM}PostgreSQL and Redis are left alone unless you pass --purge-config / --purge-state.${NC}"
else
  printf '%b\n' ""
  printf '%b\n' "${RED}${BOLD}  WARNING: This permanently deletes ALL Redgres data on this host.${NC}"
  printf '%b\n' "${RED}${BOLD}  PostgreSQL clusters, Redis, PgBouncer, config, SQLite state, and backups under ${BACKUP_ROOT} will be destroyed.${NC}"
  printf '%b\n' ""
  printf '%b\n' "${YELLOW}${BOLD}  Back up before continuing:${NC}"
  printf '%b\n' "${YELLOW}    · PostgreSQL  pg_dumpall or your usual backup tool${NC}"
  printf '%b\n' "${YELLOW}    · Redis       RDB/AOF copy or redis-cli SAVE + archive data dir${NC}"
  printf '%b\n' "${YELLOW}    · Redgres     copy ${VAR_ROOT} and ${ETC_ROOT}${NC}"
  printf '%b\n' "${DIM}  There is no undo. This script does not create or verify backups.${NC}"
  printf '%b\n' ""
  printf '%b\n' "${DIM}  Also removes: Cloudflare DNS/tunnel/Access (API), certbot db/rs certs, tunnel units,${NC}"
  printf '%b\n' "${DIM}  bootstrap :8989 firewall rule, Redgres Docker workloads, cloudflared package,${NC}"
  printf '%b\n' "${DIM}  and git clones of this repo at /root/redgres or ~/redgres.${NC}"
  printf '%b\n' "${DIM}  Docker Engine stays installed. Use --keep-remote to skip Cloudflare/certbot.${NC}"
  printf '%b\n' ""
fi

if [[ "${APP_ONLY}" -eq 1 && "${PURGE_CONFIG}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}--purge-config: will delete ${ETC_ROOT}${NC}"
fi
if [[ "${APP_ONLY}" -eq 1 && "${PURGE_STATE}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}--purge-state: will delete ${VAR_ROOT}${NC}"
fi

confirm_uninstall() {
  local response=""
  printf '%b\n' "${YELLOW}${BOLD}Uninstall this server?${NC}  ${DIM}yes / y to continue  ·  no / n to abort${NC}"
  printf '%b' "${YELLOW}${BOLD}Choice [y/N]: ${NC}"
  read -r response || true
  response="${response//$'\r'/}"
  case "${response}" in
    [yY]|[yY][eE][sS])
      printf '%b\n' "${GREEN}Confirmed — uninstalling…${NC}"
      return 0
      ;;
    [nN]|[nN][oO]|"")
      printf '%b\n' "${DIM}Aborted.${NC}"
      exit 1
      ;;
    *)
      printf '%b\n' "${DIM}Aborted.${NC}"
      exit 1
      ;;
  esac
}

if [[ "${FORCE}" -ne 1 ]]; then
  confirm_uninstall
  printf '%b\n' ""
else
  printf '%b\n' "${YELLOW}Force mode (-y): warnings shown above; confirmation skipped.${NC}"
  printf '%b\n' ""
fi

redgres_uninstall_enter_safe_cwd || true
redgres_uninstall_export_apt_env

DOMAIN_SNAPSHOT="$(mktemp /tmp/redgres-uninstall-domain.XXXXXX)"
CF_API_STATUS="unknown"
CF_CLEANUP_SCOPE="full"
REDGRES_UNINSTALL_QUIESCE_GUARD=0
REDGRES_UNINSTALL_APP_WAS_ACTIVE=0
REDGRES_UNINSTALL_TLS_PATH_WAS_ACTIVE=0
redgres_uninstall_exit_cleanup() {
  redgres_uninstall_restore_quiesced
  rm -f "${DOMAIN_SNAPSHOT}" /tmp/redgres-uninstall-apt.*
}
trap 'rc=$?; redgres_uninstall_exit_cleanup; exit "${rc}"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

write_domain_snapshot() {
  local sqlite
  : >"${DOMAIN_SNAPSHOT}"
  command -v python3 >/dev/null 2>&1 || return 0
  sqlite="${VAR_ROOT}/redgres.db"
  python3 - "${sqlite}" "${DOMAIN_SNAPSHOT}" "${VAR_ROOT}" "${ETC_ROOT}" \
    "/var/lib/redgres-tls/issue.result" "/etc/redgres/tls-lineage" <<'PY' || true
import base64, json, os, re, sqlite3, stat, sys
from urllib.parse import urlparse

sqlite, out_path, var_root, etc_root, issue_result, tls_lineage = sys.argv[1:7]
HOST_RE = re.compile(r"^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$")
nofollow = getattr(os, "O_NOFOLLOW", 0)
directory = getattr(os, "O_DIRECTORY", 0)

def safe_secret(name):
    if not name or "/" in name or "\\" in name or name in (".", "..") or ".." in name:
        return b""
    try:
        root_fd = os.open(var_root, os.O_RDONLY | directory | nofollow)
    except OSError:
        root_fd = None
    if root_fd is not None:
        try:
            secrets_fd = os.open("secrets", os.O_RDONLY | directory | nofollow, dir_fd=root_fd)
            try:
                fd = os.open(name, os.O_RDONLY | nofollow, dir_fd=secrets_fd)
                try:
                    info = os.fstat(fd)
                    if not stat.S_ISREG(info.st_mode) or info.st_size > 65536:
                        return b""
                    return os.read(fd, 65537)
                finally:
                    os.close(fd)
            finally:
                os.close(secrets_fd)
        except OSError:
            return b""
        finally:
            os.close(root_fd)
    # Fallback when O_DIRECTORY/openat is unavailable (e.g. some Windows Python builds).
    path = os.path.join(var_root, "secrets", name)
    if os.path.islink(path) or not os.path.isfile(path):
        return b""
    try:
        fd = os.open(path, os.O_RDONLY | nofollow)
    except OSError:
        return b""
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_size > 65536:
            return b""
        return os.read(fd, 65537)
    finally:
        os.close(fd)

def valid_host(value):
    value = (value or "").strip().lower().rstrip(".")
    if not value or not HOST_RE.fullmatch(value):
        return ""
    if value in ("localhost",) or value.endswith(".localhost"):
        return ""
    return value

def hostname_from_url(raw):
    raw = (raw or "").strip()
    if not raw:
        return ""
    try:
        parsed = urlparse(raw)
    except ValueError:
        return ""
    if parsed.scheme != "https":
        return ""
    host = valid_host(parsed.hostname or "")
    if not host:
        return ""
    if host.startswith("127.") or host == "::1":
        return ""
    return host

def decode_tunnel_ids():
    raw = safe_secret("cloudflared-tunnel-token")
    if not raw:
        return "", ""
    text = raw.decode("utf-8", errors="ignore").strip()
    if not text:
        return "", ""
    pad = "=" * ((4 - (len(text) % 4)) % 4)
    try:
        payload = json.loads(base64.urlsafe_b64decode(text + pad))
    except (ValueError, json.JSONDecodeError):
        try:
            payload = json.loads(base64.b64decode(text + pad))
        except (ValueError, json.JSONDecodeError):
            return "", ""
    if not isinstance(payload, dict):
        return "", ""
    account = str(payload.get("a") or payload.get("AccountTag") or "").strip()
    tunnel = str(payload.get("t") or payload.get("TunnelID") or "").strip()
    id_re = re.compile(r"^[A-Za-z0-9-]{8,64}$")
    if not id_re.fullmatch(account):
        account = ""
    if not id_re.fullmatch(tunnel):
        tunnel = ""
    return account, tunnel

def safe_regular_text(path, max_bytes=65536):
    if not path or not os.path.isfile(path) or os.path.islink(path):
        return ""
    try:
        fd = os.open(path, os.O_RDONLY | nofollow)
    except OSError:
        return ""
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_size > max_bytes:
            return ""
        return os.read(fd, max_bytes + 1).decode("utf-8", errors="ignore")
    except OSError:
        return ""
    finally:
        os.close(fd)

def collect_hosts():
    # Snapshot display may include env URLs; destructive deletes use root TLS only.
    hosts = set()
    env_text = safe_regular_text(os.path.join(etc_root, "redgres.env"))
    for line in env_text.splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        if key in ("REDGRES_BASE_URL", "REDGRES_PGADMIN_URL", "REDGRES_REDISINSIGHT_URL"):
            host = hostname_from_url(value.strip().strip('"').strip("'"))
            if host:
                hosts.add(host)
    for line in safe_regular_text(issue_result).splitlines():
        if line.startswith("host="):
            host = valid_host(line[5:])
            if host:
                hosts.add(host)
    lineage_text = safe_regular_text(tls_lineage, max_bytes=4096)
    if lineage_text:
        first = (lineage_text.splitlines() or [""])[0].strip()
        if first.startswith("/etc/letsencrypt/live/"):
            base = re.sub(r"-\d+$", "", first.rsplit("/", 1)[-1])
            host = valid_host(base)
            if host:
                hosts.add(host)
    return hosts

dep = None
if os.path.isfile(sqlite):
    try:
        con = sqlite3.connect(f"file:{sqlite}?mode=ro", uri=True)
        row = con.execute("SELECT payload FROM domain_deployment WHERE id = 1").fetchone()
        if row:
            dep = json.loads(row[0])
    except Exception:
        dep = None

if not isinstance(dep, dict):
    account, tunnel_id = decode_tunnel_ids()
    hosts = sorted(collect_hosts())
    if not account and not tunnel_id and not hosts:
        raise SystemExit(0)
    dep = {
        "account_id": account,
        "tunnel_id": tunnel_id,
        "console_hostname": next((h for h in hosts if h.startswith("console.")), hosts[0] if hosts else ""),
        "db_hostname": next((h for h in hosts if h.startswith("db.")), ""),
        "rs_hostname": next((h for h in hosts if h.startswith("rs.")), ""),
        "pgadmin_hostname": next((h for h in hosts if h.startswith("pgadmin.")), ""),
        "redis_insight_hostname": next((h for h in hosts if h.startswith("redis.")), ""),
    }

zone = (dep.get("zone_name") or "").strip()
console = (dep.get("console_hostname") or dep.get("hostname") or "").strip()
db_host = (dep.get("db_hostname") or "").strip()
rs = (dep.get("rs_hostname") or dep.get("redis_hostname") or "").strip()
pgadmin = (dep.get("pgadmin_hostname") or "").strip()
insight = (dep.get("redis_insight_hostname") or "").strip()
if not console and zone:
    console = f"console.{zone}"
if not db_host and zone:
    db_host = f"db.{zone}"
if not rs and zone:
    rs = f"rs.{zone}"
if not pgadmin and zone:
    pgadmin = f"pgadmin.{zone}"
if not insight and zone:
    insight = f"redis.{zone}"
tunnel_id = (dep.get("tunnel_id") or "").strip()
tunnel_name = (dep.get("tunnel_name") or "").strip()
dns_provider = (dep.get("dns_provider") or "").strip().lower()

with open(out_path, "w", encoding="utf-8") as fh:
    json.dump({
        "configured": True,
        "zone": zone,
        "console": console,
        "db": db_host,
        "rs": rs,
        "pgadmin": pgadmin,
        "insight": insight,
        "tunnel_id": tunnel_id,
        "tunnel_name": tunnel_name,
        "dns_provider": dns_provider,
    }, fh)
PY
}

write_domain_snapshot

print_cloudflare_followup() {
  local snap="${DOMAIN_SNAPSHOT}" need_manual=1 ZONE CONSOLE DB RS PGADMIN INSIGHT TUNNEL_ID TUNNEL_NAME
  [[ "${APP_ONLY}" -eq 1 ]] && return 0
  [[ "${KEEP_REMOTE}" -eq 1 ]] && need_manual=1
  [[ "${CF_API_STATUS}" == "api_ok" && "${CF_CLEANUP_SCOPE:-full}" != "tunnel_only" ]] && need_manual=0
  [[ "${CF_API_STATUS}" == "api_ok" && "${CF_CLEANUP_SCOPE:-full}" == "tunnel_only" ]] && need_manual=1

  if [[ -f "${snap}" ]] && python3 - "${snap}" >/dev/null 2>&1 <<'PY'
import json, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
raise SystemExit(0 if data.get("configured") is True else 1)
PY
  then
    snapshot_value() {
      python3 - "${snap}" "$1" <<'PY'
import json, re, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
value = data.get(sys.argv[2], "")
if isinstance(value, str) and len(value) <= 253 and re.fullmatch(r"[A-Za-z0-9._:-]*", value):
    print(value, end="")
PY
    }
    ZONE="$(snapshot_value zone)"
    CONSOLE="$(snapshot_value console)"
    DB="$(snapshot_value db)"
    RS="$(snapshot_value rs)"
    PGADMIN="$(snapshot_value pgadmin)"
    INSIGHT="$(snapshot_value insight)"
    TUNNEL_ID="$(snapshot_value tunnel_id)"
    TUNNEL_NAME="$(snapshot_value tunnel_name)"
    printf '%b\n' ""
    if [[ "${need_manual}" -eq 0 ]]; then
      printf '%b\n' "  ${BOLD}Cloudflare${NC}  ${GREEN}API cleanup attempted${NC} ${DIM}(zone ${ZONE:-unknown})${NC}"
      printf '%b\n' "  ${DIM}Verify nothing remains — dashboard links below if needed.${NC}"
    else
      printf '%b\n' "  ${YELLOW}${BOLD}Cloudflare — check dashboard and remove anything left:${NC}"
    fi
    printf '%b\n' "    ${BOLD}DNS${NC}       https://dash.cloudflare.com/  →  ${ZONE:-your zone}  →  DNS"
    if [[ -n "${CONSOLE:-}" ]]; then
      printf '%b\n' "              remove: ${CONSOLE}"
      [[ -n "${DB:-}" ]] && printf '%b\n' "                        ${DB}  ${DIM}(grey-cloud A/AAAA)${NC}"
      [[ -n "${RS:-}" ]] && printf '%b\n' "                        ${RS}  ${DIM}(grey-cloud A/AAAA)${NC}"
      [[ -n "${PGADMIN:-}" ]] && printf '%b\n' "                        ${PGADMIN}  ${DIM}(proxied CNAME)${NC}"
      [[ -n "${INSIGHT:-}" ]] && printf '%b\n' "                        ${INSIGHT}  ${DIM}(proxied CNAME)${NC}"
    fi
    printf '%b\n' "    ${BOLD}Access${NC}    https://one.dash.cloudflare.com/access/applications"
    if [[ -n "${CONSOLE:-}" ]]; then
      printf '%b\n' "              delete apps for: ${CONSOLE}"
    else
      printf '%b\n' "              delete console/pgadmin/redis UI apps"
    fi
    [[ -n "${PGADMIN:-}" ]] && printf '%b\n' "                        ${PGADMIN}"
    [[ -n "${INSIGHT:-}" ]] && printf '%b\n' "                        ${INSIGHT}"
    printf '%b\n' "    ${BOLD}Tunnels${NC}   https://one.dash.cloudflare.com/networks/tunnels"
    if [[ -n "${TUNNEL_NAME:-}" ]]; then
      printf '%b\n' "              delete tunnel: ${TUNNEL_NAME}"
    elif [[ -n "${TUNNEL_ID:-}" ]]; then
      printf '%b\n' "              delete tunnel id: ${TUNNEL_ID}"
    else
      printf '%b\n' "              delete Redgres tunnel for this host"
    fi
    if [[ -n "${DB:-}" ]]; then
      printf '%b\n' "    ${BOLD}TLS (host)${NC}  certbot certificates  ${DIM}→ delete ${DB} if still listed${NC}"
    fi
    return 0
  fi

  if [[ "${CF_API_STATUS}" == "no_state" || "${CF_API_STATUS}" == "insufficient_evidence" || "${CF_API_STATUS}" == "no_token" || "${KEEP_REMOTE}" -eq 1 || "${CF_CLEANUP_SCOPE:-}" == "tunnel_only" ]]; then
    printf '%b\n' ""
    printf '%b\n' "  ${YELLOW}${BOLD}Cloudflare — verify remaining DNS/Access/tunnels in the dashboard:${NC}"
    printf '%b\n' "    DNS      https://dash.cloudflare.com/"
    printf '%b\n' "    Access   https://one.dash.cloudflare.com/access/applications"
    printf '%b\n' "    Tunnels  https://one.dash.cloudflare.com/networks/tunnels"
  fi
}

remote_cloudflare_disconnect() {
  local sqlite cf_out
  [[ "${KEEP_REMOTE}" -eq 1 ]] && {
    CF_API_STATUS="skipped_keep_remote"
    return 0
  }
  sqlite="${VAR_ROOT}/redgres.db"
  command -v python3 >/dev/null 2>&1 || {
    printf '%b\n' "${RED}Error: python3 is required for confirmed Cloudflare cleanup; local evidence was preserved.${NC}" >&2
    CF_API_STATUS="no_python"
    return 1
  }
  cf_out="$(python3 - "${sqlite}" "${VAR_ROOT}" "${ETC_ROOT}" \
    "/var/lib/redgres-tls/issue.result" "/etc/redgres/tls-lineage" <<'PY' 2>&1 || true
import base64, json, os, re, sqlite3, stat, sys, urllib.error, urllib.parse, urllib.request
from urllib.parse import urlparse

sqlite, var_root, etc_root, issue_result, tls_lineage = sys.argv[1:6]
HOST_RE = re.compile(r"^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$")
ID_RE = re.compile(r"^[A-Za-z0-9-]{8,64}$")
nofollow = getattr(os, "O_NOFOLLOW", 0)
directory = getattr(os, "O_DIRECTORY", 0)

def safe_secret(name):
    if not name or "/" in name or "\\" in name or name in (".", "..") or ".." in name:
        return b""
    try:
        root_fd = os.open(var_root, os.O_RDONLY | directory | nofollow)
    except OSError:
        root_fd = None
    if root_fd is not None:
        try:
            secrets_fd = os.open("secrets", os.O_RDONLY | directory | nofollow, dir_fd=root_fd)
            try:
                fd = os.open(name, os.O_RDONLY | nofollow, dir_fd=secrets_fd)
                try:
                    info = os.fstat(fd)
                    if not stat.S_ISREG(info.st_mode) or info.st_size > 65536:
                        return b""
                    return os.read(fd, 65537)
                finally:
                    os.close(fd)
            finally:
                os.close(secrets_fd)
        except OSError:
            return b""
        finally:
            os.close(root_fd)
    # Fallback when O_DIRECTORY/openat is unavailable (e.g. some Windows Python builds).
    path = os.path.join(var_root, "secrets", name)
    if os.path.islink(path) or not os.path.isfile(path):
        return b""
    try:
        fd = os.open(path, os.O_RDONLY | nofollow)
    except OSError:
        return b""
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_size > 65536:
            return b""
        return os.read(fd, 65537)
    finally:
        os.close(fd)

def read_token():
    raw = safe_secret("cloudflare-api-token")
    if raw:
        token = raw.decode("utf-8").strip()
        if token:
            return token
    raw = safe_secret("cloudflare-oauth-token.json")
    if raw:
        try:
            payload = json.loads(raw.decode("utf-8"))
            token = (payload.get("access_token") or "").strip()
            if token:
                return token
        except (UnicodeDecodeError, json.JSONDecodeError):
            pass
    return ""

def decode_tunnel_ids():
    raw = safe_secret("cloudflared-tunnel-token")
    if not raw:
        return "", ""
    text = raw.decode("utf-8", errors="ignore").strip()
    if not text:
        return "", ""
    pad = "=" * ((4 - (len(text) % 4)) % 4)
    for decoder in (base64.urlsafe_b64decode, base64.b64decode):
        try:
            payload = json.loads(decoder(text + pad))
            break
        except (ValueError, json.JSONDecodeError):
            payload = None
    if not isinstance(payload, dict):
        return "", ""
    account = str(payload.get("a") or payload.get("AccountTag") or "").strip()
    tunnel = str(payload.get("t") or payload.get("TunnelID") or "").strip()
    if not ID_RE.fullmatch(account):
        account = ""
    if not ID_RE.fullmatch(tunnel):
        tunnel = ""
    return account, tunnel

def valid_host(value):
    value = (value or "").strip().lower().rstrip(".")
    if not value or not HOST_RE.fullmatch(value):
        return ""
    if value in ("localhost",) or value.endswith(".localhost"):
        return ""
    return value

def hostname_from_url(raw):
    raw = (raw or "").strip()
    if not raw:
        return ""
    try:
        parsed = urlparse(raw)
    except ValueError:
        return ""
    if parsed.scheme != "https":
        return ""
    host = valid_host(parsed.hostname or "")
    if not host or host.startswith("127.") or host == "::1":
        return ""
    return host

def safe_regular_text(path, max_bytes=65536):
    """Read a trusted Redgres text file without following a leaf symlink."""
    if not path or not os.path.isfile(path) or os.path.islink(path):
        return ""
    flags = os.O_RDONLY | nofollow
    try:
        fd = os.open(path, flags)
    except OSError:
        return ""
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_size > max_bytes:
            return ""
        return os.read(fd, max_bytes + 1).decode("utf-8", errors="ignore")
    except OSError:
        return ""
    finally:
        os.close(fd)

def collect_root_hosts():
    """Hostname inventory from root-owned TLS evidence only (not app-writable env)."""
    hosts = set()
    text = safe_regular_text(issue_result)
    for line in text.splitlines():
        if line.startswith("host="):
            host = valid_host(line[5:])
            if host:
                hosts.add(host)
    text = safe_regular_text(tls_lineage, max_bytes=4096)
    if text:
        first = (text.splitlines() or [""])[0].strip()
        if first.startswith("/etc/letsencrypt/live/"):
            base = re.sub(r"-\d+$", "", first.rsplit("/", 1)[-1])
            host = valid_host(base)
            if host:
                hosts.add(host)
    return hosts

def root_uninstall_footprint():
    """True when root-owned Redgres markers exist (app cannot forge these alone)."""
    for path in (
        issue_result,
        tls_lineage,
        "/etc/systemd/system/cloudflared-redgres.service",
        "/etc/systemd/system/redgres.service",
    ):
        if path and os.path.isfile(path) and not os.path.islink(path):
            try:
                st = os.stat(path)
            except OSError:
                continue
            if stat.S_ISREG(st.st_mode):
                return True
    return False

def collect_hosts():
    # Destructive DNS/Access targeting uses root TLS hosts only.
    return collect_root_hosts()

def normalize_access_domain(value):
    value = (value or "").strip().lower()
    if value.startswith("https://"):
        value = value[8:]
    elif value.startswith("http://"):
        value = value[7:]
    return value.rstrip("/")

def access_app_matches(app_domain, app_name, domain):
    del app_name
    want = normalize_access_domain(domain)
    return normalize_access_domain(app_domain) == want

def cf_request(method, path, token, query=""):
    url = "https://api.cloudflare.com/client/v4" + path + query
    req = urllib.request.Request(
        url,
        method=method,
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read()
            status = resp.status
    except urllib.error.HTTPError as err:
        if err.code == 404 and method == "DELETE":
            return True, None
        if method == "DELETE":
            print(f"         warn: Cloudflare DELETE failed ({err.code})", file=sys.stderr)
            return False, None
        print(f"         warn: Cloudflare {method} failed ({err.code})", file=sys.stderr)
        return False, None
    except Exception:
        print(f"         warn: Cloudflare {method} failed", file=sys.stderr)
        return False, None
    if method == "DELETE":
        return status in (200, 404), None
    try:
        payload = json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return False, None
    if not payload.get("success"):
        return False, None
    return True, payload.get("result")

def cf_delete(path, token):
    ok, _ = cf_request("DELETE", path, token)
    return ok

def discover_zone(token, hostname):
    parts = hostname.split(".")
    for i in range(len(parts) - 2, -1, -1):
        candidate = ".".join(parts[i:])
        ok, result = cf_request("GET", "/zones", token, "?name=" + urllib.parse.quote(candidate))
        if not ok or not isinstance(result, list):
            continue
        for zone in result:
            name = str(zone.get("name") or "")
            if name.lower() != candidate.lower():
                continue
            zone_id = str(zone.get("id") or "").strip()
            account_id = ""
            acct = zone.get("account") or {}
            if isinstance(acct, dict):
                account_id = str(acct.get("id") or "").strip()
            if ID_RE.fullmatch(zone_id):
                return zone_id, account_id if ID_RE.fullmatch(account_id) else "", candidate
    return "", "", ""

def ui_hosts(hosts):
    out = []
    for host in hosts:
        if host.startswith("db.") or host.startswith("rs."):
            continue
        out.append(host)
    return out

def load_sqlite_dep():
    if not os.path.isfile(sqlite):
        return None, "missing"
    try:
        con = sqlite3.connect(f"file:{sqlite}?mode=ro", uri=True)
        row = con.execute("SELECT payload FROM domain_deployment WHERE id = 1").fetchone()
    except Exception:
        return None, "unreadable"
    if not row:
        return None, "empty"
    try:
        dep = json.loads(row[0])
    except (TypeError, json.JSONDecodeError):
        return None, "unreadable"
    if not isinstance(dep, dict):
        return None, "unreadable"
    return dep, "sqlite"

dep, dep_source = load_sqlite_dep()
if dep_source == "empty":
    print("         no domain configured")
    print("SCOPE:none")
    print("TUNNEL:none")
    print("STATUS:no_domain")
    raise SystemExit(0)

if dep is not None and (dep.get("dns_provider") or "").strip().lower() == "manual":
    print("         manual DNS — Cloudflare API skipped")
    print("SCOPE:none")
    print("TUNNEL:none")
    print("STATUS:manual_dns")
    raise SystemExit(0)

token = read_token()
if not token:
    print("         warn: no Cloudflare token — remove DNS/tunnel/Access in dashboard", file=sys.stderr)
    print("SCOPE:none")
    print("TUNNEL:none")
    print("STATUS:no_token")
    raise SystemExit(0)

hosts = set()
account = ""
zone = ""
tunnel_id = ""
apps = []
records = []
scope = "full"

if dep is not None:
    account = str(dep.get("account_id") or "").strip()
    zone = str(dep.get("zone_id") or "").strip()
    tunnel_id = str(dep.get("tunnel_id") or "").strip()
    apps = list(dep.get("access_apps") or [])
    if not apps and dep.get("access_app_id"):
        apps = [{
            "app_id": dep.get("access_app_id"),
            "policy_id": dep.get("access_policy_id") or "",
        }]
    records = list(dep.get("records") or [])
    for key in ("console_hostname", "hostname", "db_hostname", "rs_hostname", "redis_hostname",
                "pgadmin_hostname", "redis_insight_hostname"):
        host = valid_host(dep.get(key) or "")
        if host:
            hosts.add(host)
else:
    print("         reconstructing Cloudflare inventory from local Redgres evidence")
    account, tunnel_id = decode_tunnel_ids()
    hosts = collect_hosts()

if not account or not tunnel_id:
    fb_account, fb_tunnel = decode_tunnel_ids()
    if not account:
        account = fb_account
    if not tunnel_id:
        tunnel_id = fb_tunnel
if not hosts:
    hosts = collect_hosts()

hosts = {h for h in hosts if valid_host(h)}
has_ids = bool(apps or records)
# SQLite may store short fixture IDs; decoded tunnel tokens stay ID_RE-validated above.
has_tunnel_ids = bool(account and tunnel_id and re.fullmatch(r"^[A-Za-z0-9_-]{1,128}$", account) and re.fullmatch(r"^[A-Za-z0-9_-]{1,128}$", tunnel_id))
# App-writable tunnel-token alone must not authorize DELETE; require SQLite or root footprint.
has_tunnel = bool(has_tunnel_ids and (dep_source == "sqlite" or root_uninstall_footprint()))
needs_named = bool(hosts) and not has_ids

if has_tunnel_ids and not has_tunnel:
    print("         warn: tunnel token present without root Redgres footprint; refusing remote tunnel delete", file=sys.stderr)

if needs_named:
    if not zone or not account:
        sample = sorted(hosts)[0]
        found_zone, found_account, _ = discover_zone(token, sample)
        if not zone:
            zone = found_zone
        if not account and found_account:
            account = found_account
    if not zone or not account:
        print("         warn: hostnames present but zone/account could not be resolved", file=sys.stderr)
        print("SCOPE:none")
        print("TUNNEL:preserved")
        print("STATUS:insufficient_evidence")
        raise SystemExit(0)

if not has_ids and not needs_named and not has_tunnel:
    print("         no reconstructible Cloudflare inventory")
    print("SCOPE:none")
    print("TUNNEL:none")
    print("STATUS:no_state")
    raise SystemExit(0)

if not has_ids and not needs_named and has_tunnel:
    scope = "tunnel_only"

failed = False
deleted_apps = set()

if apps and account:
    for binding in apps:
        app_id = binding.get("app_id") or ""
        policy_id = binding.get("policy_id") or ""
        if app_id and policy_id and not cf_delete(f"/accounts/{account}/access/apps/{app_id}/policies/{policy_id}", token):
            failed = True
        if app_id and app_id not in deleted_apps:
            if not cf_delete(f"/accounts/{account}/access/apps/{app_id}", token):
                failed = True
            deleted_apps.add(app_id)
elif needs_named and account:
    ok, listed = cf_request("GET", f"/accounts/{account}/access/apps", token)
    if not ok:
        failed = True
    else:
        listed = listed if isinstance(listed, list) else []
        for host in ui_hosts(hosts):
            for app in listed:
                app_id = str(app.get("id") or "").strip()
                if not app_id or app_id in deleted_apps:
                    continue
                if not access_app_matches(app.get("domain"), app.get("name"), host):
                    continue
                if not cf_delete(f"/accounts/{account}/access/apps/{app_id}", token):
                    failed = True
                deleted_apps.add(app_id)

if records and zone:
    for rec in records:
        rec_id = rec.get("id") or ""
        if rec_id and not cf_delete(f"/zones/{zone}/dns_records/{rec_id}", token):
            failed = True
elif needs_named and zone:
    for host in sorted(hosts):
        ok, listed = cf_request(
            "GET",
            f"/zones/{zone}/dns_records",
            token,
            "?name=" + urllib.parse.quote(host),
        )
        if not ok:
            failed = True
            continue
        listed = listed if isinstance(listed, list) else []
        for rec in listed:
            rec_id = str(rec.get("id") or "").strip()
            name = str(rec.get("name") or "").strip().lower().rstrip(".")
            if not rec_id or name != host:
                continue
            if not cf_delete(f"/zones/{zone}/dns_records/{rec_id}", token):
                failed = True

tunnel_status = "none"
if has_tunnel:
    tunnel_status = "preserved"
    if not failed:
        if cf_delete(f"/accounts/{account}/cfd_tunnel/{tunnel_id}/connections", token):
            if cf_delete(f"/accounts/{account}/cfd_tunnel/{tunnel_id}", token):
                tunnel_status = "deleted"
            else:
                failed = True
        else:
            failed = True

if scope == "tunnel_only":
    print("         Cloudflare tunnel disconnect attempted (DNS/Access require manual check)")
else:
    print("         Cloudflare disconnect attempted (DNS records, tunnel, Access)")
print(f"SCOPE:{scope}")
print(f"TUNNEL:{tunnel_status}")
print("STATUS:api_partial" if failed else "STATUS:api_ok")
PY
)"
  printf '%s\n' "${cf_out}" | grep -Ev '^(STATUS|TUNNEL|SCOPE):' || true
  redgres_uninstall_apply_cloudflare_result "${cf_out}"
  if [[ "${CF_API_STATUS}" == "no_state" || "${CF_API_STATUS}" == "insufficient_evidence" ]]; then
    printf '%b\n' "${RED}Error: no reconstructible Cloudflare inventory from local Redgres evidence; local state was preserved. Use --keep-remote only after accepting manual Cloudflare cleanup.${NC}" >&2
    return 1
  fi
  if ! redgres_uninstall_cloudflare_status_confirmed "${CF_API_STATUS}"; then
    printf '%b\n' "${RED}Error: Cloudflare cleanup was not confirmed; local state and credentials were preserved for retry. Use --keep-remote only to accept manual remote cleanup.${NC}" >&2
    return 1
  fi
}

purge_tls_certs() {
  local lineage_file='/etc/redgres/tls-lineage' lineage cert_name rc=0
  [[ "${KEEP_REMOTE}" -eq 1 ]] && return 0
  if [[ ! -e "${lineage_file}" ]]; then
    printf '%b\n' "         ${YELLOW}no trusted Redgres lineage; legacy Certbot certificates require manual review${NC}"
    return 0
  fi
  redgres_uninstall_delete_trusted_lineage "${lineage_file}" /usr/bin/certbot || rc=$?
  if [[ "${rc}" -ne 0 ]]; then
    printf '%b\n' "${RED}Error: Certbot lineage cleanup could not be verified; trusted evidence was preserved for retry.${NC}" >&2
    return 1
  fi
  lineage="$(/usr/bin/head -n1 "${lineage_file}")"
  cert_name="${lineage##*/}"
  printf '%b\n' "         certbot lineage deleted for ${cert_name}"
}

purge_tls_local_copies() {
  local targets='/etc/redgres/tls-targets' metadata key value cluster='' pgbouncer=0 version name
  if [[ -f "${targets}" && ! -L "${targets}" ]]; then
    metadata="$(/usr/bin/stat -c '%U:%G:%a' "${targets}" 2>/dev/null || true)"
    if [[ "${metadata}" == "root:root:600" ]]; then
      while IFS='=' read -r key value; do
        case "${key}" in
          postgres_cluster) [[ "${value}" =~ ^[0-9]+/[a-z0-9_-]+$ ]] && cluster="${value}" ;;
          pgbouncer) [[ "${value}" == "0" || "${value}" == "1" ]] && pgbouncer="${value}" ;;
        esac
      done <"${targets}"
    fi
  fi
  rm -rf /etc/ssl/redgres /etc/redgres/tls 2>/dev/null || true
  rm -f /etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh 2>/dev/null || true
  if [[ -n "${cluster}" ]]; then
    version="${cluster%%/*}"
    name="${cluster#*/}"
    rm -f "/etc/postgresql/${version}/${name}/conf.d/redgres-ssl.conf" \
      "/etc/postgresql/${version}/${name}/redgres-fullchain.pem" \
      "/etc/postgresql/${version}/${name}/redgres-privkey.pem" 2>/dev/null || true
  fi
  if [[ "${pgbouncer}" == "1" ]]; then
    rm -f /etc/pgbouncer/redgres-fullchain.pem /etc/pgbouncer/redgres-privkey.pem 2>/dev/null || true
  fi
  rm -f /etc/redgres/tls-targets /etc/redgres/tls-lineage 2>/dev/null || true
}

purge_cloudflared_package() {
  [[ "${APP_ONLY}" -eq 1 ]] && return 0
  if command -v apt-get >/dev/null 2>&1; then
    redgres_uninstall_purge_installed cloudflared || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf remove -y cloudflared 2>/dev/null || true
  elif command -v yum >/dev/null 2>&1; then
    yum remove -y cloudflared 2>/dev/null || true
  fi
}

redgres_docker_containers() {
  command -v docker >/dev/null 2>&1 || return 0
  docker ps -a --format '{{.ID}}|{{.Names}}|{{.Label "com.docker.compose.project"}}' 2>/dev/null |
    awk -F'|' -v n="${DOCKER_NAME_RE}" -v p="${DOCKER_PROJECT_RE}" \
      '$2 ~ n || $3 ~ p {print $1}'
}

redgres_docker_images() {
  command -v docker >/dev/null 2>&1 || return 0
  docker images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null |
    awk '$0 ~ /^(redgres|redis-insight|redisinsight|pgadmin-redgres|dpage\/pgadmin4|redis\/redisinsight)/ && $0 !~ / /'
}

redgres_docker_volumes() {
  command -v docker >/dev/null 2>&1 || return 0
  docker volume ls --format '{{.Name}}|{{.Labels}}' 2>/dev/null |
    awk -F'|' -v n="${DOCKER_NAME_RE}" \
      '$1 ~ n || $2 ~ /com\.docker\.compose\.project=redgres/ {print $1}'
}

redgres_docker_networks() {
  command -v docker >/dev/null 2>&1 || return 0
  docker network ls --format '{{.Name}}|{{.Labels}}' 2>/dev/null |
    awk -F'|' -v n="${DOCKER_NAME_RE}" \
      '$1 ~ n || $2 ~ /com\.docker\.compose\.project=redgres/ {print $1}'
}

stop_systemd_unit() {
  local unit="$1"
  systemctl stop "${unit}" 2>/dev/null || true
  systemctl disable "${unit}" 2>/dev/null || true
}

remove_bootstrap_firewall() {
  if [[ -x "${LIBEXEC_ROOT}/bootstrap-ufw-remove.sh" ]]; then
    "${LIBEXEC_ROOT}/bootstrap-ufw-remove.sh" 2>/dev/null || true
    return 0
  fi
  command -v ufw >/dev/null 2>&1 || return 0
  local i rule
  for ((i = 0; i < 20; i++)); do
    rule="$(ufw status numbered 2>/dev/null | grep -E '8989' | head -1 | grep -oE '[[:space:]]*[0-9]+' | tr -d ' ' || true)"
    [[ -n "${rule}" ]] || break
    ufw --force delete "${rule}" 2>/dev/null || break
  done
  ufw delete allow 8989/tcp 2>/dev/null || true
}

remove_owned_bootstrap_firewall_rules() {
  command -v ufw >/dev/null 2>&1 || return 0
  local i line rule
  for ((i = 0; i < 20; i++)); do
    line="$(ufw status numbered 2>/dev/null | grep -E '8989.*redgres-bootstrap' | head -1 || true)"
    [[ -n "${line}" ]] || return 0
    rule="$(printf '%s\n' "${line}" | grep -oE '^\[[[:space:]]*[0-9]+\]' | tr -cd '0-9')"
    [[ -n "${rule}" ]] || return 1
    ufw --force delete "${rule}" >/dev/null 2>&1 || return 1
  done
  return 1
}

purge_postgresql() {
  redgres_uninstall_drop_postgres_clusters
  redgres_uninstall_remove_postgres_leftovers
  stop_systemd_unit postgresql.service
  stop_systemd_unit postgresql@.service
  if command -v apt-get >/dev/null 2>&1; then
    redgres_uninstall_purge_postgresql_packages || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf remove -y postgresql\* 2>/dev/null || true
  elif command -v yum >/dev/null 2>&1; then
    yum remove -y postgresql\* 2>/dev/null || true
  fi
  redgres_uninstall_remove_postgres_leftovers
}

purge_redis_native() {
  stop_systemd_unit redis-server.service
  stop_systemd_unit redis.service
  if command -v apt-get >/dev/null 2>&1; then
    redgres_uninstall_purge_native_redis_packages || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf remove -y redis 2>/dev/null || true
  elif command -v yum >/dev/null 2>&1; then
    yum remove -y redis 2>/dev/null || true
  fi
}

purge_pgbouncer() {
  stop_systemd_unit pgbouncer.service
  if command -v apt-get >/dev/null 2>&1; then
    redgres_uninstall_purge_installed pgbouncer || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf remove -y pgbouncer 2>/dev/null || true
  elif command -v yum >/dev/null 2>&1; then
    yum remove -y pgbouncer 2>/dev/null || true
  fi
}

redgres_quiesce_domain_tls() {
  local unit
  command -v systemctl >/dev/null 2>&1 || return 0
  REDGRES_UNINSTALL_APP_WAS_ACTIVE=0
  REDGRES_UNINSTALL_TLS_PATH_WAS_ACTIVE=0
  systemctl is-active --quiet redgres.service && REDGRES_UNINSTALL_APP_WAS_ACTIVE=1
  systemctl is-active --quiet redgres-tls-issue.path && REDGRES_UNINSTALL_TLS_PATH_WAS_ACTIVE=1
  REDGRES_UNINSTALL_QUIESCE_GUARD=1
  redgres_uninstall_quiesce_cloudflare || return 1
  systemctl stop redgres-tls-issue.path >/dev/null 2>&1 || true
  systemctl stop redgres.service >/dev/null 2>&1 || true
  systemctl stop redgres-tls-issue.service >/dev/null 2>&1 || true
  for unit in redgres-tls-issue.path redgres.service redgres-tls-issue.service; do
    if systemctl is-active --quiet "${unit}"; then
      printf '%b\n' "${RED}Error: domain/TLS services could not be quiesced; cleanup aborted.${NC}" >&2
      return 1
    fi
  done
}

already_removed_rc=0
redgres_uninstall_exit_if_already_removed "${APP_ONLY}" "${UNINSTALL_LOCAL_PATHS[@]}" || already_removed_rc=$?
case "${already_removed_rc}" in
  0) ;;
  10)
    if ! remove_owned_bootstrap_firewall_rules; then
      printf '%s\n' 'Error: tagged Redgres bootstrap firewall rules could not be removed.' >&2
      exit 1
    fi
    redgres_uninstall_enter_safe_cwd || true
    while IFS= read -r checkout || [[ -n "${checkout}" ]]; do
      [[ -n "${checkout}" ]] || continue
      rm -rf "${checkout}" 2>/dev/null || true
    done < <(redgres_uninstall_collect_git_checkouts)
    exit 0
    ;;
  *) exit "${already_removed_rc}" ;;
esac

# ── 0. Cloudflare + TLS (before local state is deleted) ─────────────────────
if [[ "${APP_ONLY}" -eq 0 ]]; then
  redgres_quiesce_domain_tls || exit 1
fi
if [[ "${APP_ONLY}" -eq 0 && "${KEEP_REMOTE}" -eq 0 ]]; then
  step "  ${CYAN}[0/8]${NC} Cloudflare + TLS cleanup... "
  remote_cloudflare_disconnect || exit 1
  purge_tls_certs || exit 1
  purge_tls_local_copies
  step_done
elif [[ "${APP_ONLY}" -eq 0 && "${KEEP_REMOTE}" -eq 1 ]]; then
  step "  ${CYAN}[0/8]${NC} Preserving remote Cloudflare + Certbot lineage; removing local TLS copies... "
  purge_tls_local_copies
  step_done
fi
REDGRES_UNINSTALL_QUIESCE_GUARD=0

# ── 1. Stop Redgres + tunnel units ───────────────────────────────────────────
if command -v systemctl >/dev/null 2>&1; then
  step "  ${CYAN}[1/8]${NC} Stopping Redgres and tunnel services... "
  stop_systemd_unit redgres.service
  stop_systemd_unit redgres-bootstrap-ufw-remove.path
  stop_systemd_unit redgres-bootstrap-ufw-remove.service
  stop_systemd_unit cloudflared-redgres.service
  stop_systemd_unit cloudflared-redgres.path
  stop_systemd_unit cloudflared-redgres-restart.service
  stop_systemd_unit redgres-tls-issue.path
  stop_systemd_unit redgres-tls-issue.service
  step_done
else
  step "  ${CYAN}[1/8]${NC} systemd not found "
  step_skip
fi

# ── 2. Bootstrap firewall ────────────────────────────────────────────────────
step "  ${CYAN}[2/8]${NC} Removing bootstrap firewall rule (8989)... "
remove_bootstrap_firewall
step_done

# ── 3. Docker workloads ──────────────────────────────────────────────────────
if [[ "${APP_ONLY}" -eq 0 ]]; then
  step "  ${CYAN}[3/8]${NC} Removing Redgres Docker containers... "
  if command -v docker >/dev/null 2>&1; then
    if [[ -f "${VAR_ROOT}/redis/docker-compose.yml" ]]; then
      (cd "${VAR_ROOT}/redis" && docker compose down --volumes --remove-orphans 2>/dev/null) || true
    fi
    if [[ -f "${ETC_ROOT}/redis-compose.yml" ]]; then
      docker compose -f "${ETC_ROOT}/redis-compose.yml" down --volumes --remove-orphans 2>/dev/null || true
    fi
    if [[ -f "${ETC_ROOT}/expert-tools-compose.yml" ]]; then
      docker compose -f "${ETC_ROOT}/expert-tools-compose.yml" down --volumes --remove-orphans 2>/dev/null || true
    fi
    CONTAINERS="$(redgres_docker_containers || true)"
    if [[ -n "${CONTAINERS}" ]]; then
      docker stop ${CONTAINERS} 2>/dev/null || true
      docker rm -f -v ${CONTAINERS} 2>/dev/null || true
      step_done
      printf '%b\n' "         ${DIM}removed $(count_lines "${CONTAINERS}") container(s)${NC}"
    else
      step_skip
    fi
    IMAGES="$(redgres_docker_images || true)"
    [[ -n "${IMAGES}" ]] && docker rmi -f ${IMAGES} 2>/dev/null || true
    VOLUMES="$(redgres_docker_volumes || true)"
    [[ -n "${VOLUMES}" ]] && docker volume rm -f ${VOLUMES} 2>/dev/null || true
    NETWORKS="$(redgres_docker_networks || true)"
    [[ -n "${NETWORKS}" ]] && docker network rm ${NETWORKS} 2>/dev/null || true
  else
    step_skip
  fi
else
  step "  ${CYAN}[3/8]${NC} Docker cleanup "
  step_skip
fi

# ── 4. PostgreSQL / Redis / PgBouncer ───────────────────────────────────────
if [[ "${APP_ONLY}" -eq 0 ]]; then
  step "  ${CYAN}[4/8]${NC} Removing PostgreSQL clusters and packages... "
  purge_postgresql
  step_done

  step "  ${CYAN}[5/8]${NC} Removing Redis, PgBouncer, and cloudflared... "
  purge_redis_native
  purge_pgbouncer
  purge_cloudflared_package
  step_done
else
  step "  ${CYAN}[4/8]${NC} PostgreSQL/Redis/PgBouncer "
  step_skip
  step "  ${CYAN}[5/8]${NC} (skipped — --app-only) "
  step_skip
fi

# ── 6. Systemd units + libexec ───────────────────────────────────────────────
step "  ${CYAN}[6/8]${NC} Removing systemd units and helpers... "
rm -f "${UNIT_PATH}" \
  /etc/systemd/system/redgres-bootstrap-ufw-remove.service \
  /etc/systemd/system/redgres-bootstrap-ufw-remove.path \
  /etc/systemd/system/cloudflared-redgres.service \
  /etc/systemd/system/cloudflared-redgres.path \
  /etc/systemd/system/cloudflared-redgres-restart.service \
  /etc/systemd/system/redgres-tls-issue.service \
  /etc/systemd/system/redgres-tls-issue.path \
  2>/dev/null || true
rm -rf "${LIBEXEC_ROOT}" 2>/dev/null || true
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload 2>/dev/null || true
  systemctl reset-failed 2>/dev/null || true
fi
step_done

# ── 7. Filesystem ────────────────────────────────────────────────────────────
step "  ${CYAN}[7/8]${NC} Removing Redgres files... "
redgres_uninstall_enter_safe_cwd || true
rm -f \
  "${VAR_ROOT}/secrets/cloudflare-api-token" \
  "${VAR_ROOT}/secrets/cloudflared-tunnel-token" \
  "${VAR_ROOT}/secrets/certbot-dns.ini" \
  "${VAR_ROOT}/secrets/cloudflare-oauth-client.json" \
  "${VAR_ROOT}/secrets/cloudflare-oauth-token.json" \
  "${ETC_ROOT}/secrets/cloudflare-oauth-token" \
  "${ETC_ROOT}/secrets/certbot-dns-token" \
  2>/dev/null || true
rmdir "${VAR_ROOT}/secrets" "${ETC_ROOT}/secrets" 2>/dev/null || true
rm -rf "${OPT_ROOT}" 2>/dev/null || true
rm -f /usr/local/bin/redgres 2>/dev/null || true

if [[ "${APP_ONLY}" -eq 0 || "${PURGE_CONFIG}" -eq 1 ]]; then
  rm -rf "${ETC_ROOT}" 2>/dev/null || true
fi
if [[ "${APP_ONLY}" -eq 0 || "${PURGE_STATE}" -eq 1 ]]; then
  rm -rf "${VAR_ROOT}" /var/lib/redgres-tls /var/lib/redgres-release 2>/dev/null || true
fi
if [[ "${APP_ONLY}" -eq 0 ]]; then
  rm -rf "${BACKUP_ROOT}" /var/log/redgres 2>/dev/null || true
  while IFS= read -r checkout || [[ -n "${checkout}" ]]; do
    [[ -n "${checkout}" ]] || continue
    rm -rf "${checkout}" 2>/dev/null || true
  done < <(redgres_uninstall_collect_git_checkouts)
fi
step_done

# ── 8. Leftover apt packages (after cwd is off the deleted checkout) ─────────
if [[ "${APP_ONLY}" -eq 0 ]] && command -v apt-get >/dev/null 2>&1; then
  step "  ${CYAN}[8/8]${NC} Removing leftover packages... "
  redgres_uninstall_enter_safe_cwd || true
  redgres_uninstall_apt_get autoremove -y || true
  step_done
else
  step "  ${CYAN}[8/8]${NC} Leftover packages "
  step_skip
fi

if id redgres >/dev/null 2>&1; then
  userdel redgres 2>/dev/null || true
fi
if getent group redgres >/dev/null 2>&1; then
  groupdel redgres 2>/dev/null || true
fi

BUSY=""
if command -v ss >/dev/null 2>&1; then
  BUSY="$(ss -tlnH 2>/dev/null | awk '{print $4}' | grep -oE ':(8790|8989|5432|6379|6380|6432|5540|5050)$' | sort -u | tr '\n' ' ' || true)"
fi

log=""
if [[ "${APP_ONLY}" -eq 1 ]]; then
  log="${GREEN}${BOLD}Redgres application uninstalled.${NC}"
else
  log="${GREEN}${BOLD}Redgres fully removed. Host is clean for a fresh install.${NC}"
fi
printf '%b\n' ""
printf '%b\n' "  ${log}"
if [[ -n "${BUSY}" ]]; then
  printf '%b\n' "  ${YELLOW}Note:${NC} ports still in use (non-Redgres or stale socket): ${BUSY}"
  printf '%b\n' "  ${DIM}Check with: ss -tlnp${NC}"
else
  printf '%b\n' "  ${DIM}Redgres ports (8790, 8989, 5432, 6379, 6380, 6432) are free.${NC}"
fi
if [[ "${APP_ONLY}" -eq 0 ]]; then
  print_cloudflare_followup
  printf '%b\n' "  ${DIM}Reinstall: curl -fsSL .../install.sh | sudo bash${NC}"
else
  printf '%b\n' "  ${DIM}PostgreSQL and Redis were not removed (--app-only).${NC}"
fi
printf '%b\n' ""
