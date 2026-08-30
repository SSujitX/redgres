#!/usr/bin/env bash
# Redgres public application installer (control-plane binary + systemd only).
# Does NOT install PostgreSQL, Redis, PgBouncer, Docker, or extensions.
#
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install.sh | sudo bash
#   curl -fsSL .../install.sh | sudo bash -s -- v=1.0.0
#   curl -fsSL .../install.sh | sudo bash -s -- --non-interactive
#
# Full stack wizard (preview): git clone + deploy/install.sh --non-interactive --dry-run
# Pinning/downgrading replaces the application release only. Prefer upgrade.sh to move forward.

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

REPO="SSujitX/redgres"
OPT_ROOT="/opt/redgres"
ETC_ROOT="/etc/redgres"
VAR_ROOT="/var/lib/redgres"
UNIT_PATH="/etc/systemd/system/redgres.service"
API="https://api.github.com/repos/${REPO}"
HEALTHZ="http://127.0.0.1:8790/api/v1/healthz"

die() { printf '%b\n' "${RED}Error: $*${NC}" >&2; exit 1; }
log() { printf '%b\n' "$*"; }

_redgres_load_install_summary() {
  local _self="${BASH_SOURCE[0]}" _dir
  # curl | bash: BASH_SOURCE is not a real file. dirname becomes "." and would
  # source a stale checkout scripts/install-summary.sh from cwd (e.g. ~/redgres).
  if [[ -n "${_self}" && -f "${_self}" ]]; then
    _dir="$(cd "$(dirname "${_self}")" 2>/dev/null && pwd || true)"
    if [[ -n "${_dir}" && -f "${_dir}/scripts/install-summary.sh" ]]; then
      # shellcheck source=/dev/null
      source "${_dir}/scripts/install-summary.sh"
      if declare -F redgres_ensure_domain_secret_env >/dev/null 2>&1; then
        return 0
      fi
    fi
  fi
  # shellcheck source=/dev/null
  source /dev/stdin <<'REDGRES_INSTALL_SUMMARY'
# REDGRES_INSTALL_SUMMARY_BEGIN
# Shared install/upgrade finish summary.
# Canonical source: edit here, then run: ./scripts/sync-install-summary-embed.sh
# Public scripts embed this block between REDGRES_INSTALL_SUMMARY_BEGIN/END markers.

redgres_summary_env_value() {
  local key="$1" file="${2:-/etc/redgres/redgres.env}" line
  [[ -f "${file}" ]] || return 0
  line="$(grep -E "^${key}=" "${file}" 2>/dev/null | tail -n1 || true)"
  printf '%s' "${line#*=}"
}

# Domain wizard secret *paths* only. Files are created later by the UI/API.
redgres_domain_secret_env_defaults() {
  cat <<'EOF'
REDGRES_CLOUDFLARE_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-api-token
REDGRES_TUNNEL_TOKEN_FILE=/var/lib/redgres/secrets/cloudflared-tunnel-token
REDGRES_CLOUDFLARE_OAUTH_CLIENT_FILE=/var/lib/redgres/secrets/cloudflare-oauth-client.json
REDGRES_CLOUDFLARE_OAUTH_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-oauth-token.json
REDGRES_CERTBOT_DNS_TOKEN_FILE=/var/lib/redgres/secrets/certbot-dns.ini
REDGRES_TLS_ISSUE_REQUEST_FILE=/var/lib/redgres/tls-issue.request
REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres/tls-issue.result
EOF
}

redgres_env_ensure_lines() {
  local env_file="$1"
  local line key added=0
  [[ -f "${env_file}" ]] || return 1
  while IFS= read -r line || [[ -n "${line}" ]]; do
    [[ -n "${line}" ]] || continue
    key="${line%%=*}"
    [[ -n "${key}" && "${key}" != "${line}" ]] || continue
    if grep -qE "^${key}=" "${env_file}"; then
      continue
    fi
    printf '%s\n' "${line}" >>"${env_file}"
    added=1
  done
  [[ "${added}" -eq 1 ]]
}

redgres_ensure_secrets_dir() {
  local dir="${REDGRES_SECRETS_DIR:-/var/lib/redgres/secrets}"
  mkdir -p "${dir}"
  if command -v getent >/dev/null 2>&1 && getent passwd redgres >/dev/null 2>&1; then
    chown redgres:redgres "${dir}" 2>/dev/null || true
  fi
  chmod 700 "${dir}"
}

redgres_ensure_domain_secret_env() {
  local env_file="${1:-/etc/redgres/redgres.env}"
  redgres_ensure_secrets_dir
  [[ -f "${env_file}" ]] || return 1
  if redgres_domain_secret_env_defaults | redgres_env_ensure_lines "${env_file}"; then
    return 0
  fi
  return 1
}

redgres_public_ipv4() {
  local ip candidate
  for candidate in $(hostname -I 2>/dev/null); do
    candidate="${candidate// /}"
    [[ -n "${candidate}" ]] || continue
    case "${candidate}" in
      127.*|10.*|192.168.*|172.1[6-9].*|172.2[0-9].*|172.3[0-1].*|169.254.*) continue ;;
    esac
    printf '%s' "${candidate}"
    return 0
  done
  ip="$(curl -fsS --max-time 3 https://api.ipify.org 2>/dev/null || true)"
  printf '%s' "${ip}"
}

# Origin the browser will send. Must match REDGRES_BASE_URL or login is 403.
redgres_bootstrap_login_origin() {
  local pub port="${1:-8989}"
  pub="$(redgres_public_ipv4)"
  if [[ "${pub}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf 'http://%s:%s' "${pub}" "${port}"
  else
    printf 'http://127.0.0.1:%s' "${port}"
  fi
}

redgres_assert_github_download_url() {
  local url="$1" name="$2" tag="$3"
  [[ -n "${tag}" && "${tag}" != */* && "${tag}" != *..* ]] || return 1
  [[ "${url}" == "https://github.com/SSujitX/redgres/releases/download/${tag}/${name}" ]] || return 1
}

redgres_bootstrap_port() {
  local baddr="${1:-$(redgres_summary_env_value REDGRES_BOOTSTRAP_ADDRESS)}"
  local port="${baddr##*:}"
  if [[ -n "${port}" && "${port}" != "${baddr}" ]]; then
    printf '%s' "${port}"
  else
    printf '8989'
  fi
}

redgres_wait_for_healthz() {
  local healthz="${1:-http://127.0.0.1:8790/api/v1/healthz}" tries="${2:-20}"
  local ok=0 n
  for ((n = 0; n < tries; n++)); do
    if curl -fsS --connect-timeout 2 --max-time 5 "${healthz}" >/dev/null 2>&1; then
      ok=1
      break
    fi
    sleep 1
  done
  (( ok == 1 ))
}

redgres_bootstrap_port_open() {
  local addr port env_file="/etc/redgres/redgres.env"
  addr="${1:-$(redgres_summary_env_value REDGRES_BOOTSTRAP_ADDRESS "${env_file}")}"
  port="$(redgres_bootstrap_port "${addr}")"
  if command -v ss >/dev/null; then
    ss -ltn 2>/dev/null | grep -qE ":${port}\\b"
    return $?
  fi
  if command -v netstat >/dev/null; then
    netstat -ltn 2>/dev/null | grep -qE ":${port}\\b"
    return $?
  fi
  return 1
}

redgres_bootstrap_public_url() {
  local addr pub host port env_file="/etc/redgres/redgres.env"
  addr="$(redgres_summary_env_value REDGRES_BOOTSTRAP_ADDRESS "${env_file}")"
  pub="$(redgres_public_ipv4)"
  [[ -n "${addr}" && -n "${pub}" ]] || return 0
  host="${addr%%:*}"
  port="$(redgres_bootstrap_port "${addr}")"
  if [[ "${host}" == "0.0.0.0" || "${host}" == "::" || "${host}" == "[::]" ]]; then
    printf 'http://%s:%s' "${pub}" "${port}"
  else
    printf 'http://%s:%s' "${host}" "${port}"
  fi
}

redgres_load_console_url() {
  local sqlite env_file="/etc/redgres/redgres.env" db_path
  sqlite="$(redgres_summary_env_value REDGRES_SQLITE_PATH "${env_file}")"
  [[ -n "${sqlite}" ]] || sqlite="/var/lib/redgres/redgres.db"
  [[ -f "${sqlite}" ]] || return 0
  command -v python3 >/dev/null || return 0
  db_path="${sqlite}"
  printf '%s' "$(python3 - "${db_path}" <<'PY'
import json, sqlite3, sys

db = sys.argv[1]
try:
    con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    row = con.execute("SELECT payload FROM domain_deployment WHERE id = 1").fetchone()
    if not row:
        raise SystemExit(0)
    d = json.loads(row[0])
except Exception:
    raise SystemExit(0)

zone = (d.get("zone_name") or "").strip()
console = (d.get("console_hostname") or d.get("hostname") or "").strip()
if not console and zone:
    console = f"console.{zone}"
if console:
    print(f"https://{console}")
PY
)"
}

redgres_owner_configured() {
  local sqlite env_file="/etc/redgres/redgres.env" db_path count
  sqlite="$(redgres_summary_env_value REDGRES_SQLITE_PATH "${env_file}")"
  [[ -n "${sqlite}" ]] || sqlite="/var/lib/redgres/redgres.db"
  [[ -f "${sqlite}" ]] || return 1
  if command -v python3 >/dev/null; then
    db_path="${sqlite}"
    count="$(python3 - "${db_path}" <<'PY'
import sqlite3, sys

db = sys.argv[1]
try:
    con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    row = con.execute("SELECT COUNT(*) FROM owners").fetchone()
    print(row[0] if row else 0)
except Exception:
    print(0)
PY
)"
    [[ "${count:-0}" -gt 0 ]]
    return $?
  fi
  if command -v sqlite3 >/dev/null; then
    count="$(sqlite3 "file:${sqlite}?mode=ro" "SELECT COUNT(*) FROM owners;" 2>/dev/null || echo 0)"
    [[ "${count:-0}" -gt 0 ]]
    return $?
  fi
  return 2
}

redgres_fw_note() {
  if declare -F redgres_log >/dev/null 2>&1; then
    redgres_log "$*"
  elif declare -F log >/dev/null 2>&1; then
    log "  $*"
  else
    printf '%s\n' "$*"
  fi
}

# Single operator IP only (no CIDR). Loopback/unspecified rejected.
redgres_assert_bootstrap_allow_ip() {
  local ip="$1"
  [[ -n "${ip}" ]] || return 1
  case "${ip}" in
    */*|*' '*|0.0.0.0|::|::1) return 1 ;;
  esac
  if [[ "${ip}" == *:* ]]; then
    [[ "${ip}" =~ ^[0-9a-fA-F:]+$ ]] || return 1
    return 0
  fi
  [[ "${ip}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || return 1
  case "${ip}" in
    127.*) return 1 ;;
  esac
  return 0
}

redgres_ip_from_who_line() {
  local line="$1" host
  [[ -n "${line}" ]] || return 1
  case "${line}" in
    *'('*')') ;;
    *) return 1 ;;
  esac
  host="${line##*(}"
  host="${host%)}"
  host="${host#[}"
  host="${host%]}"
  redgres_assert_bootstrap_allow_ip "${host}" || return 1
  printf '%s' "${host}"
}

redgres_ssh_client_ip_from_environ_stream() {
  local key raw conn="" client=""
  while IFS= read -r -d '' key; do
    case "${key}" in
      SSH_CONNECTION=*)
        raw="${key#SSH_CONNECTION=}"
        conn="${raw%% *}"
        ;;
      SSH_CLIENT=*)
        raw="${key#SSH_CLIENT=}"
        client="${raw%% *}"
        ;;
    esac
  done
  if redgres_assert_bootstrap_allow_ip "${conn}"; then
    printf '%s' "${conn}"
    return 0
  fi
  if redgres_assert_bootstrap_allow_ip "${client}"; then
    printf '%s' "${client}"
    return 0
  fi
  return 1
}

redgres_proc_ppid() {
  local line pid=""
  [[ -r "$1" ]] || return 1
  while IFS= read -r line; do
    case "${line}" in
      PPid:*)
        pid="${line#PPid:}"
        pid="${pid#"${pid%%[![:space:]]*}"}"
        printf '%s' "${pid}"
        return 0
        ;;
    esac
  done <"$1"
  return 1
}

redgres_ssh_client_ip_from_ancestors() {
  local envf pid i found
  if [[ -n "${REDGRES_BOOTSTRAP_PROC_ENVIRON+x}" ]]; then
    envf="${REDGRES_BOOTSTRAP_PROC_ENVIRON}"
    [[ -n "${envf}" && -r "${envf}" ]] || return 1
    redgres_ssh_client_ip_from_environ_stream <"${envf}"
    return
  fi
  pid="${PPID:-}"
  i=0
  while [[ "${i}" -lt 8 && "${pid}" =~ ^[1-9][0-9]*$ ]]; do
    envf="/proc/${pid}/environ"
    if [[ -r "${envf}" ]]; then
      found="$(redgres_ssh_client_ip_from_environ_stream <"${envf}" || true)"
      if redgres_assert_bootstrap_allow_ip "${found}"; then
        printf '%s' "${found}"
        return 0
      fi
    fi
    pid="$(redgres_proc_ppid "/proc/${pid}/status" || true)"
    i=$((i + 1))
  done
  return 1
}

redgres_ssh_client_ip_from_who() {
  local line
  if [[ -n "${REDGRES_BOOTSTRAP_WHO_LINE+x}" ]]; then
    line="${REDGRES_BOOTSTRAP_WHO_LINE}"
  else
    line="$(who -m 2>/dev/null || true)"
    [[ -n "${line}" ]] || line="$(who am i 2>/dev/null || true)"
  fi
  redgres_ip_from_who_line "${line}"
}

redgres_bootstrap_allow_from() {
  local raw="${REDGRES_BOOTSTRAP_ALLOW_FROM:-}"
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  raw=""
  if [[ -n "${SSH_CONNECTION:-}" ]]; then
    raw="${SSH_CONNECTION%% *}"
  fi
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  raw=""
  if [[ -n "${SSH_CLIENT:-}" ]]; then
    raw="${SSH_CLIENT%% *}"
  fi
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  raw="$(redgres_ssh_client_ip_from_ancestors || true)"
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  raw="$(redgres_ssh_client_ip_from_who || true)"
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  return 1
}

redgres_prompt_say() {
  if [[ -w /dev/tty ]]; then
    printf '%s\n' "$1" >/dev/tty
  else
    printf '%s\n' "$1" >&2
  fi
}

redgres_prompt_bootstrap_allow_ip() {
  local input="${REDGRES_BOOTSTRAP_ALLOW_TTY:-/dev/tty}" answer i
  [[ -r "${input}" ]] || return 1
  for i in 1 2 3; do
    redgres_prompt_say 'First-run console (port 8989) opens only for your current public IP.'
    redgres_prompt_say 'It closes after Domain & Network confirm. Not opened to the internet.'
    if [[ -w /dev/tty ]]; then
      printf '%s' 'Public IP you will browse from: ' >/dev/tty
    else
      printf '%s' 'Public IP you will browse from: ' >&2
    fi
    IFS= read -r answer <"${input}" || return 1
    answer="${answer//[$' \t\r\n']/}"
    if redgres_assert_bootstrap_allow_ip "${answer}"; then
      printf '%s' "${answer}"
      return 0
    fi
    redgres_prompt_say 'That is not a single operator IP (no 0.0.0.0, no CIDR).'
  done
  return 1
}

redgres_resolve_bootstrap_allow_from() {
  local from
  from="$(redgres_bootstrap_allow_from || true)"
  if [[ -n "${from}" ]]; then
    printf '%s' "${from}"
    return 0
  fi
  from="$(redgres_prompt_bootstrap_allow_ip || true)"
  if redgres_assert_bootstrap_allow_ip "${from}"; then
    REDGRES_BOOTSTRAP_ALLOW_FROM="${from}"
    printf '%s' "${from}"
    return 0
  fi
  return 1
}

redgres_ufw_bootstrap_allow_argv() {
  local from="$1"
  printf 'allow from %s to any port 8989 proto tcp comment redgres-bootstrap' "${from}"
}

redgres_ensure_app_identity() {
  local getent='/usr/bin/getent' groupadd='/usr/sbin/groupadd' useradd='/usr/sbin/useradd'
  [[ -x "${getent}" ]] || getent="$(command -v getent || true)"
  [[ -n "${getent}" ]] || return 1
  "${getent}" group redgres >/dev/null || "${groupadd}" --system redgres
  "${getent}" passwd redgres >/dev/null || "${useradd}" --system --gid redgres --home-dir /var/lib/redgres --shell /usr/sbin/nologin redgres
  mkdir -p /var/lib/redgres /var/lib/redgres/secrets /etc/redgres
  chown redgres:redgres /var/lib/redgres
  chmod 750 /var/lib/redgres
  chown root:redgres /etc/redgres
  chmod 750 /etc/redgres
}

redgres_restore_pgadmin_master_ownership() {
  local master='/var/lib/redgres/secrets/pgadmin.master'
  local hook='/var/lib/redgres/secrets/pgadmin-master-hook'
  if [[ -f "${master}" && ! -L "${master}" ]]; then
    chown 5050:redgres "${master}"
    chmod 640 "${master}"
  fi
  if [[ -f "${hook}" && ! -L "${hook}" ]]; then
    chown 5050:redgres "${hook}"
    chmod 750 "${hook}"
  fi
}

redgres_chown_app_state() {
  local db='/var/lib/redgres/redgres.db'
  chown redgres:redgres /var/lib/redgres 2>/dev/null || true
  if [[ -e "${db}" ]]; then
    chown redgres:redgres "${db}" 2>/dev/null || true
    chown redgres:redgres "${db}-wal" "${db}-shm" 2>/dev/null || true
  fi
  if [[ -d /var/lib/redgres/secrets ]]; then
    chown -R redgres:redgres /var/lib/redgres/secrets
  fi
  redgres_restore_pgadmin_master_ownership
  if [[ -f /etc/redgres/redgres.env ]]; then
    chown root:redgres /etc/redgres/redgres.env
    chmod 660 /etc/redgres/redgres.env
  fi
}

redgres_app_unit_body() {
  local binary_path="$1"
  cat <<EOF
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

redgres_write_app_unit() {
  local binary_path="$1" unit_path="${2:-/etc/systemd/system/redgres.service}"
  mkdir -p "$(dirname "${unit_path}")"
  redgres_app_unit_body "${binary_path}" >"${unit_path}"
}

redgres_install_bootstrap_ufw_helper() {
  mkdir -p /usr/libexec/redgres /etc/systemd/system
  cat >/usr/libexec/redgres/bootstrap-ufw-remove.sh <<'EOF'
#!/bin/bash
set -euo pipefail
umask 077
PATH=/usr/sbin:/usr/bin:/sbin:/bin
if command -v ufw >/dev/null 2>&1; then
  i=0
  while [[ "${i}" -lt 20 ]]; do
    rule="$(ufw status numbered 2>/dev/null | grep -E '8989' | head -1 | grep -oE '[[:space:]]*[0-9]+' | tr -d ' ' || true)"
    [[ -n "${rule}" ]] || break
    ufw --force delete "${rule}" 2>/dev/null || break
    i=$((i + 1))
  done
  ufw delete allow 8989/tcp 2>/dev/null || true
fi
rm -f /var/lib/redgres/bootstrap-ufw-remove.requested
EOF
  chmod 0755 /usr/libexec/redgres/bootstrap-ufw-remove.sh
  chown root:root /usr/libexec/redgres/bootstrap-ufw-remove.sh 2>/dev/null || true
  cat >/etc/systemd/system/redgres-bootstrap-ufw-remove.service <<'EOF'
[Unit]
Description=Remove Redgres bootstrap :8989 UFW rule

[Service]
Type=oneshot
User=root
ExecStart=/usr/libexec/redgres/bootstrap-ufw-remove.sh
EOF
  cat >/etc/systemd/system/redgres-bootstrap-ufw-remove.path <<'EOF'
[Unit]
Description=Remove Redgres bootstrap UFW rule when requested

[Path]
PathExists=/var/lib/redgres/bootstrap-ufw-remove.requested
Unit=redgres-bootstrap-ufw-remove.service

[Install]
WantedBy=multi-user.target
EOF
  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable --now redgres-bootstrap-ufw-remove.path >/dev/null 2>&1 || true
  fi
}

redgres_ufw_restrict_bootstrap() {
  local from=""
  if ! command -v ufw >/dev/null 2>&1; then
    redgres_fw_note 'ufw: not installed; bootstrap :8989 is not source-restricted'
    return 0
  fi
  from="$(redgres_resolve_bootstrap_allow_from || true)"
  if [[ -z "${from}" ]]; then
    redgres_fw_note 'ufw: operator source IP unknown; not opening 8989 to the world'
    return 0
  fi
  # Never `ufw allow 8989/tcp` (that is 0.0.0.0/0).
  if ufw allow from "${from}" to any port 8989 proto tcp comment 'redgres-bootstrap'; then
    if ufw status 2>/dev/null | grep -q '^Status: active'; then
      redgres_fw_note "ufw: allow 8989/tcp from ${from} only"
    else
      redgres_fw_note "ufw: inactive; queued allow from ${from} to 8989 (enable UFW or a cloud firewall to enforce)"
    fi
  else
    redgres_fw_note 'ufw: failed to add source-restricted 8989 rule'
    return 1
  fi
}

redgres_install_bootstrap_firewall() {
  redgres_install_bootstrap_ufw_helper
  redgres_ufw_restrict_bootstrap || true
}

# print_install_summary VERSION CHANNEL [MODE]
# CHANNEL: stable | dev | upgrade
# MODE: unchanged (dev only — already on this version, restarted)
redgres_print_install_summary() {
  local version="$1" channel="${2:-stable}" mode="${3:-}"
  local listen pub boot_url console_url health_ok="" svc="" owner_ok=0 bport
  local opt="/opt/redgres" etc_dir="/etc/redgres" env_file="/etc/redgres/redgres.env"
  local healthz="http://127.0.0.1:8790/api/v1/healthz"

  listen="$(redgres_summary_env_value REDGRES_ADDRESS "${env_file}")"
  [[ -n "${listen}" ]] || listen="127.0.0.1:8790"
  pub="$(redgres_public_ipv4)"
  console_url="$(redgres_load_console_url || true)"
  local base_url baddr
  base_url="$(redgres_summary_env_value REDGRES_BASE_URL "${env_file}")"
  if [[ -z "${console_url}" && "${base_url}" == https://* ]]; then
    console_url="${base_url}"
  fi
  baddr="$(redgres_summary_env_value REDGRES_BOOTSTRAP_ADDRESS "${env_file}")"
  bport="$(redgres_bootstrap_port "${baddr}")"

  if curl -fsS --max-time 3 "${healthz}" >/dev/null 2>&1; then
    health_ok="yes"
  fi
  if redgres_owner_configured; then
    owner_ok=1
  else
    case "$?" in
      2) owner_ok=-1 ;;
    esac
  fi
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet redgres.service 2>/dev/null; then
    svc="active"
  else
    svc="inactive"
  fi

  printf '\n'
  case "${channel}" in
    dev)
      if [[ "${mode}" == "unchanged" ]]; then
        printf '  \033[0;32m\033[1mAlready on %s\033[0m \033[2m(restarted; config preserved)\033[0m\n' "${version}"
      else
        printf '  \033[0;32m\033[1mDevelopment install complete\033[0m \033[2m→ %s\033[0m\n' "${version}"
      fi
      ;;
    upgrade) printf '  \033[0;32m\033[1mUpgrade complete\033[0m \033[2m→ %s\033[0m\n' "${version}" ;;
    *) printf '  \033[0;32m\033[1mRedgres application installed\033[0m \033[2m→ %s\033[0m\n' "${version}" ;;
  esac
  printf '  \033[1mBinary\033[0m     %s/current/redgres\n' "${opt}"
  printf '  \033[1mConfig\033[0m     %s/redgres.env\n' "${etc_dir}"
  if [[ "${health_ok}" == "yes" ]]; then
    if [[ "${owner_ok}" -eq 1 ]]; then
      printf '  \033[1mService\033[0m    redgres.service → %s  ·  \033[1mAPI\033[0m  \033[0;32mup\033[0m  \033[2m(owner configured)\033[0m\n' "${svc}"
    elif [[ "${owner_ok}" -eq -1 ]]; then
      printf '  \033[1mService\033[0m    redgres.service → %s  ·  \033[1mAPI\033[0m  \033[0;32mup\033[0m\n' "${svc}"
    else
      printf '  \033[1mService\033[0m    redgres.service → %s  ·  \033[1mAPI\033[0m  \033[0;32mup\033[0m  \033[2m(create owner before login)\033[0m\n' "${svc}"
    fi
  else
    printf '  \033[1mService\033[0m    redgres.service → %s  ·  \033[1mAPI\033[0m  \033[0;33mnot ready\033[0m\n' "${svc}"
  fi
  printf '\n'

  if [[ "${channel}" == "stable" ]]; then
    printf '  \033[2mScope:\033[0m control-plane binary only — PostgreSQL, Redis, PgBouncer, and Docker are \033[1mnot\033[0m installed by this script.\n'
    printf '  \033[2mFull stack wizard:\033[0m in development — preview with \033[1mgit clone\033[0m + \033[1mdeploy/install.sh --non-interactive --dry-run\033[0m (see docs/INSTALLER_SPEC.md).\n'
    printf '\n'
  elif [[ "${channel}" == "upgrade" ]]; then
    printf '  \033[2mScope:\033[0m application binary upgraded — config and SQLite preserved; PostgreSQL/Redis data only if installed separately.\n'
    printf '  \033[2mRollback:\033[0m app only via deploy/install.sh rollback --non-interactive --to PRIOR_VERSION (from git clone).\n'
    printf '\n'
  elif [[ "${channel}" == "dev" ]]; then
    printf '  \033[2mScope:\033[0m control-plane binary only — PostgreSQL, Redis, PgBouncer, and Docker are \033[1mnot\033[0m installed by this script.\n'
    printf '\n'
  fi

  local printed_open=0
  if redgres_bootstrap_port_open; then
    if [[ -n "${base_url}" ]]; then
      printf '  \033[1mOpen\033[0m       %s\n' "${base_url}"
      printed_open=1
    else
      boot_url="$(redgres_bootstrap_public_url || true)"
      if [[ -n "${boot_url}" ]]; then
        printf '  \033[1mOpen\033[0m       %s\n' "${boot_url}"
        printed_open=1
      fi
    fi
    if [[ -n "${console_url}" ]]; then
      printf '  \033[1mConsole\033[0m    %s  \033[2m(Cloudflare Access)\033[0m\n' "${console_url}"
    fi
  elif [[ -n "${console_url}" ]]; then
    printf '  \033[1mConsole\033[0m    %s\n' "${console_url}"
  fi

  if [[ "${printed_open}" -eq 0 && -n "${pub}" && ( "${baddr}" == 0.0.0.0:* || "${baddr}" == [::]:* ) ]]; then
    if redgres_bootstrap_port_open; then
      printf '  \033[1mOpen\033[0m       http://%s:%s\n' "${pub}" "${bport}"
      printed_open=1
    else
      printf '  \033[2mExpected\033[0m  http://%s:%s  \033[2m(when bootstrap is listening)\033[0m\n' "${pub}" "${bport}"
    fi
  fi

  if [[ "${baddr}" == 0.0.0.0:* || "${baddr}" == [::]:* ]]; then
    printf '  \033[0;33mBootstrap\033[0m  Open the printed URL, then finish Domain & Network to close :8989.\n'
  fi

  printf '  \033[2mDomain & TLS:\033[0m System → Domain & Network in the console.\n'
  printf '\n'
  if [[ "${owner_ok}" -eq 1 ]]; then
    printf '  \033[1mNext\033[0m       sign in at bootstrap URL → change password if prompted → Domain & Network for hostnames.\n'
  elif [[ "${owner_ok}" -eq -1 ]]; then
    printf '  \033[1mNext\033[0m       sign in at bootstrap URL (or create owner if this is a fresh host).\n'
  else
    printf '  \033[1mNext\033[0m       create owner:\n'
    printf '    %s/current/redgres create-owner --username admin --sqlite-path /var/lib/redgres/redgres.db\n' "${opt}"
    printf '  \033[2mThen\033[0m       sign in at bootstrap URL → configure PostgreSQL/Redis in Settings → Domain & Network for hostnames.\n'
  fi
  printf '\n'
  printf '  \033[2mLoopback:\033[0m http://%s  (tunnel origin — not public)\n' "${listen}"
  case "${channel}" in
    dev) printf '  \033[2mChannel:\033[0m install-dev.sh  ·  stable: install.sh / upgrade.sh\n' ;;
    upgrade) printf '  \033[2mChannel:\033[0m upgrade.sh  ·  dev: install-dev.sh\n' ;;
    *) printf '  \033[2mChannel:\033[0m install.sh / upgrade.sh  ·  dev: install-dev.sh\n' ;;
  esac
  printf '\n'
}
# REDGRES_INSTALL_SUMMARY_END
REDGRES_INSTALL_SUMMARY
}
_redgres_load_install_summary

[[ "${EUID}" -eq 0 ]] || die "Run with sudo"

REQUESTED_VERSION="${REDGRES_VERSION:-}"
NONINTERACTIVE=0
RELEASE_JSON=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    v=*|version=*|--version=*) REQUESTED_VERSION="${1#*=}" ;;
    --version|-v) shift; REQUESTED_VERSION="${1:-}" ;;
    latest|LATEST) REQUESTED_VERSION="" ;;
    v[0-9]*|[0-9]*.[0-9]*) REQUESTED_VERSION="$1" ;;
    --force) FORCE=1 ;;
    --non-interactive|-y) NONINTERACTIVE=1 ;;
    *) die "unknown install arg: $1 (use v=1.0.0, --non-interactive, or REDGRES_VERSION=1.0.0)" ;;
  esac
  shift || true
done
FORCE="${FORCE:-0}"
[[ "${REDGRES_NONINTERACTIVE:-}" == "1" ]] && NONINTERACTIVE=1

normalize_tag() {
  local raw="$1"
  raw="$(printf '%s' "${raw}" | tr -d '[:space:]')"
  [[ -z "${raw}" || "${raw}" == "latest" || "${raw}" == "LATEST" ]] && { printf ''; return 0; }
  case "${raw}" in
    v*) printf '%s' "${raw}" ;;
    *) printf 'v%s' "${raw}" ;;
  esac
}

if [[ -L "${OPT_ROOT}/current" || -d "${OPT_ROOT}/releases" ]]; then
  if [[ "${FORCE}" -ne 1 ]]; then
    die "Redgres already installed at ${OPT_ROOT}. Use upgrade.sh to move forward, or re-run with --force (application only)."
  fi
  log "${YELLOW}Existing install detected; --force will replace the application release.${NC}"
fi

command -v curl >/dev/null || die "curl is required"
command -v tar >/dev/null || die "tar is required"
command -v sha256sum >/dev/null || die "sha256sum is required"

resolve_release() {
  local tag="$1" url body
  if [[ -n "${RELEASE_JSON}" ]]; then
    printf '%s' "${RELEASE_JSON}"
    return 0
  fi
  if [[ -z "${tag}" ]]; then
    url="${API}/releases/latest"
  else
    url="${API}/releases/tags/${tag}"
  fi
  body="$(curl -fsSL -H 'Accept: application/vnd.github+json' "${url}")" || die "GitHub release not found: ${tag:-latest}"
  RELEASE_JSON="${body}"
  printf '%s' "${body}"
}

release_version_from_json() {
  local json="$1" ver
  ver="$(printf '%s' "${json}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  ver="${ver#v}"
  [[ -n "${ver}" ]] || return 1
  printf '%s' "${ver}"
}

pick_release_tag() {
  local latest_ver choice pinned json
  if [[ -n "${REQUESTED_VERSION}" ]]; then
    TAG="$(normalize_tag "${REQUESTED_VERSION}")"
    return 0
  fi
  if [[ "${NONINTERACTIVE}" -eq 1 ]]; then
    TAG=""
    return 0
  fi

  json="$(curl -fsSL -H 'Accept: application/vnd.github+json' "${API}/releases/latest")" || die "GitHub latest release not found"
  RELEASE_JSON="${json}"
  latest_ver="$(release_version_from_json "${json}" || true)"
  [[ -n "${latest_ver}" ]] || latest_ver="unknown"

  log ""
  log "  ${BOLD}Redgres version${NC}"
  log "    1) Latest release (${latest_ver}) ${DIM}[default]${NC}"
  log "    2) Pin a version (e.g. 1.0.0)"
  if [[ -e /dev/tty ]]; then
    read -r -p "  Choice [1]: " choice </dev/tty || choice="1"
  else
    choice="1"
  fi
  choice="${choice:-1}"
  case "${choice}" in
    2)
      if [[ -e /dev/tty ]]; then
        read -r -p "  Version: " pinned </dev/tty || pinned=""
      else
        pinned=""
      fi
      [[ -n "${pinned}" ]] || die "no version entered"
      TAG="$(normalize_tag "${pinned}")"
      RELEASE_JSON=""
      ;;
    *)
      TAG=""
      ;;
  esac
}

ASSET_NAME=""
TARBALL_URL=""
SUMS_URL=""
VERSION_FILE=""

pick_assets() {
  local json="$1"
  VERSION_FILE="$(printf '%s' "${json}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  VERSION_FILE="${VERSION_FILE#v}"
  [[ -n "${VERSION_FILE}" ]] || die "release tag_name missing"
  ASSET_NAME="redgres_${VERSION_FILE}_linux_amd64.tar.gz"
  TARBALL_URL="$(printf '%s' "${json}" | tr ',' '\n' | sed -n "s/.*\"browser_download_url\"[[:space:]]*:[[:space:]]*\"\\([^\"]*${ASSET_NAME}\\)\".*/\\1/p" | head -n1)"
  SUMS_URL="$(printf '%s' "${json}" | tr ',' '\n' | sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*SHA256SUMS\)".*/\1/p' | head -n1)"
  [[ -n "${TARBALL_URL}" ]] || die "release asset ${ASSET_NAME} not found"
  [[ -n "${SUMS_URL}" ]] || die "release asset SHA256SUMS not found"
  redgres_assert_github_download_url "${TARBALL_URL}" "${ASSET_NAME}" "v${VERSION_FILE}" || die "release URL is not the expected GitHub asset"
  redgres_assert_github_download_url "${SUMS_URL}" "SHA256SUMS" "v${VERSION_FILE}" || die "SHA256SUMS URL is not the expected GitHub asset"
}

log ""
log "  ${CYAN}${BOLD}Redgres application installer${NC}"
log "  ${DIM}Control-plane binary only — PostgreSQL/Redis/PgBouncer/Docker are separate${NC}"
log ""

pick_release_tag

RELEASE_JSON="$(resolve_release "${TAG:-}")"
pick_assets "${RELEASE_JSON}"

WORKDIR="$(mktemp -d /tmp/redgres-install.XXXXXX)"
trap 'rm -rf "${WORKDIR}"' EXIT
cd "${WORKDIR}"

log "  Downloading ${ASSET_NAME}..."
curl -fsSL -o "${ASSET_NAME}" "${TARBALL_URL}"
curl -fsSL -o SHA256SUMS "${SUMS_URL}"

grep -E "^[0-9a-f]{64}  ${ASSET_NAME}\$" SHA256SUMS >/dev/null || die "SHA256SUMS missing exact entry for ${ASSET_NAME}"
sha256sum -c SHA256SUMS --ignore-missing >/dev/null 2>&1 || {
  expected="$(awk -v f="${ASSET_NAME}" '$2==f {print $1}' SHA256SUMS)"
  actual="$(sha256sum "${ASSET_NAME}" | awk '{print $1}')"
  [[ "${expected}" == "${actual}" ]] || die "release checksum verification failed"
}

mkdir -p extract
tar -xzf "${ASSET_NAME}" -C extract
BIN="$(find extract -type f -name redgres | head -n1)"
VERF="$(find extract -type f -name VERSION | head -n1)"
[[ -n "${BIN}" && -x "${BIN}" ]] || die "archive missing redgres binary"
[[ -n "${VERF}" ]] || die "archive missing VERSION"
VERSION_FILE="$(tr -d '\r\n' <"${VERF}")"

DEST="${OPT_ROOT}/releases/${VERSION_FILE}"
mkdir -p "${DEST}" "${ETC_ROOT}" "${VAR_ROOT}/secrets"
redgres_ensure_app_identity
install -m 0755 "${BIN}" "${DEST}/redgres"
install -m 0644 "${VERF}" "${DEST}/VERSION"
ln -sfn "${DEST}" "${OPT_ROOT}/current"

if [[ ! -f "${ETC_ROOT}/redgres.env" ]]; then
  ORIGIN="$(redgres_bootstrap_login_origin 8989)"
  cat >"${ETC_ROOT}/redgres.env" <<EOF
REDGRES_ENVIRONMENT=production
REDGRES_ADDRESS=127.0.0.1:8790
REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989
REDGRES_BASE_URL=${ORIGIN}
REDGRES_SQLITE_PATH=/var/lib/redgres/redgres.db
REDGRES_COOKIE_SECURE=false
REDGRES_BOOTSTRAP_UFW_REMOVE_CMD=/usr/libexec/redgres/bootstrap-ufw-remove.sh
EOF
  redgres_domain_secret_env_defaults >>"${ETC_ROOT}/redgres.env"
  chmod 0660 "${ETC_ROOT}/redgres.env"
  chown root:redgres "${ETC_ROOT}/redgres.env"
fi
redgres_ensure_domain_secret_env "${ETC_ROOT}/redgres.env" || true
chmod 0660 "${ETC_ROOT}/redgres.env"
chown root:redgres "${ETC_ROOT}/redgres.env"

redgres_write_app_unit "${OPT_ROOT}/current/redgres" "${UNIT_PATH}"
redgres_install_bootstrap_firewall
redgres_chown_app_state

systemctl daemon-reload
systemctl enable redgres.service >/dev/null
systemctl restart redgres.service || log "  ${YELLOW}Service start deferred (create an owner first if needed).${NC}"

redgres_wait_for_healthz "${HEALTHZ}" 20 || log "  ${YELLOW}API check still pending — create owner if this is a fresh host.${NC}"
redgres_print_install_summary "${VERSION_FILE}" stable
