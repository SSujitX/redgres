#!/usr/bin/env bash
# Redgres upgrade (DockLift-parity UX). Always targets GitHub latest release.
# Preserves /etc/redgres and /var/lib/redgres (SQLite). PostgreSQL/Redis only if installed separately.
#
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/upgrade.sh | sudo bash
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

REPO="SSujitX/redgres"
OPT_ROOT="/opt/redgres"
UNIT_PATH="/etc/systemd/system/redgres.service"
API="https://api.github.com/repos/${REPO}"
HEALTHZ="http://127.0.0.1:8790/api/v1/healthz"

die() { printf '%b\n' "${RED}Error: $*${NC}" >&2; exit 1; }
log() { printf '%b\n' "$*"; }

_redgres_load_install_summary() {
  local _self="${BASH_SOURCE[0]}" _dir
  _dir="$(cd "$(dirname "${_self}")" 2>/dev/null && pwd || true)"
  if [[ -n "${_dir}" && -f "${_dir}/scripts/install-summary.sh" ]]; then
    # shellcheck source=/dev/null
    source "${_dir}/scripts/install-summary.sh"
    return 0
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

redgres_bootstrap_allow_from() {
  local raw="${REDGRES_BOOTSTRAP_ALLOW_FROM:-}"
  if [[ -z "${raw}" && -n "${SSH_CONNECTION:-}" ]]; then
    raw="${SSH_CONNECTION%% *}"
  fi
  if [[ -z "${raw}" && -n "${SSH_CLIENT:-}" ]]; then
    raw="${SSH_CLIENT%% *}"
  fi
  redgres_assert_bootstrap_allow_ip "${raw}" || return 1
  printf '%s' "${raw}"
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
  from="$(redgres_bootstrap_allow_from 2>/dev/null || true)"
  if [[ -z "${from}" ]]; then
    redgres_fw_note 'ufw: operator source IP unknown; set REDGRES_BOOTSTRAP_ALLOW_FROM; not opening 8989 to the world'
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
    printf '  \033[0;33mBootstrap\033[0m  :8989 is source-restricted when UFW is active (operator IP or REDGRES_BOOTSTRAP_ALLOW_FROM). Finish Domain & Network to close it.\n'
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
[[ -d "${OPT_ROOT}/releases" || -L "${OPT_ROOT}/current" ]] || die "Redgres not found at ${OPT_ROOT}. Run install.sh first."
command -v curl >/dev/null || die "curl is required"
command -v tar >/dev/null || die "tar is required"
command -v sha256sum >/dev/null || die "sha256sum is required"

PREVIOUS=""
if [[ -L "${OPT_ROOT}/current" ]]; then
  PREVIOUS="$(readlink -f "${OPT_ROOT}/current" || true)"
fi

log ""
log "  ${CYAN}${BOLD}Redgres upgrade${NC}"
log "  ${DIM}Preserves config and SQLite; installs latest GitHub release only${NC}"
log ""

JSON="$(curl -fsSL -H 'Accept: application/vnd.github+json' "${API}/releases/latest")" || die "GitHub latest release not found"
TAG="$(printf '%s' "${JSON}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
VERSION="${TAG#v}"
[[ -n "${VERSION}" ]] || die "latest tag_name missing"
ASSET="redgres_${VERSION}_linux_amd64.tar.gz"
TARBALL_URL="$(printf '%s' "${JSON}" | tr ',' '\n' | sed -n "s/.*\"browser_download_url\"[[:space:]]*:[[:space:]]*\"\\([^\"]*${ASSET}\\)\".*/\\1/p" | head -n1)"
SUMS_URL="$(printf '%s' "${JSON}" | tr ',' '\n' | sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*SHA256SUMS\)".*/\1/p' | head -n1)"
[[ -n "${TARBALL_URL}" && -n "${SUMS_URL}" ]] || die "latest release assets missing (${ASSET} / SHA256SUMS)"
redgres_assert_github_download_url "${TARBALL_URL}" "${ASSET}" "${TAG}" || die "release URL is not the expected GitHub asset"
redgres_assert_github_download_url "${SUMS_URL}" "SHA256SUMS" "${TAG}" || die "SHA256SUMS URL is not the expected GitHub asset"

WORKDIR="$(mktemp -d /tmp/redgres-upgrade.XXXXXX)"
trap 'rm -rf "${WORKDIR}"' EXIT
cd "${WORKDIR}"
curl -fsSL -o "${ASSET}" "${TARBALL_URL}"
curl -fsSL -o SHA256SUMS "${SUMS_URL}"
expected="$(awk -v f="${ASSET}" '$2==f {print $1}' SHA256SUMS)"
actual="$(sha256sum "${ASSET}" | awk '{print $1}')"
[[ -n "${expected}" && "${expected}" == "${actual}" ]] || die "release checksum verification failed"

mkdir -p extract
tar -xzf "${ASSET}" -C extract
BIN="$(find extract -type f -name redgres | head -n1)"
VERF="$(find extract -type f -name VERSION | head -n1)"
[[ -n "${BIN}" && -x "${BIN}" ]] || die "archive missing redgres binary"
VERSION="$(tr -d '\r\n' <"${VERF}")"

DEST="${OPT_ROOT}/releases/${VERSION}"
if [[ -e "${DEST}" ]]; then
  log "  ${YELLOW}Release ${VERSION} already present; switching current.${NC}"
else
  mkdir -p "${DEST}"
  install -m 0755 "${BIN}" "${DEST}/redgres"
  install -m 0644 "${VERF}" "${DEST}/VERSION"
fi

ln -sfn "${DEST}" "${OPT_ROOT}/current"

redgres_ensure_app_identity
redgres_write_app_unit "${OPT_ROOT}/current/redgres" "${UNIT_PATH}"
redgres_install_bootstrap_firewall
redgres_chown_app_state

systemctl daemon-reload
if ! systemctl restart redgres.service; then
  if [[ -n "${PREVIOUS}" && -d "${PREVIOUS}" ]]; then
    ln -sfn "${PREVIOUS}" "${OPT_ROOT}/current"
    systemctl daemon-reload || true
    systemctl restart redgres.service || true
  fi
  die "systemd restart failed; previous release restored when available"
fi

if ! redgres_wait_for_healthz "${HEALTHZ}" 15; then
  if [[ -n "${PREVIOUS}" && -d "${PREVIOUS}" ]]; then
    ln -sfn "${PREVIOUS}" "${OPT_ROOT}/current"
    systemctl daemon-reload || true
    systemctl restart redgres.service || true
  fi
  die "API check failed after upgrade; previous release restored when available"
fi

redgres_print_install_summary "${VERSION}" upgrade
