#!/usr/bin/env bash
# Redgres development installer (DockLift-parity UX).
# Installs the latest master snapshot from the rolling GitHub dev release.
# Build runs in CI — this script only downloads, verifies, and installs (like upgrade.sh).
#
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install-dev.sh | sudo bash
#
# Optional: REDGRES_DEV_BUILD_LOCAL=1 compiles on this machine (slow; needs Go + Node).
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

REPO="SSujitX/redgres"
REPO_URL="https://github.com/${REPO}.git"
OPT_ROOT="/opt/redgres"
ETC_ROOT="/etc/redgres"
VAR_ROOT="/var/lib/redgres"
UNIT_PATH="/etc/systemd/system/redgres.service"
API="https://api.github.com/repos/${REPO}"
DEV_TAG="dev"
HEALTHZ="http://127.0.0.1:8790/api/v1/healthz"

die() { printf '%b\n' "${RED}Error: $*${NC}" >&2; exit 1; }
log() { printf '%b\n' "$*"; }

env_value() {
  local key="$1" file="${ETC_ROOT}/redgres.env" line
  [[ -f "${file}" ]] || return 0
  line="$(grep -E "^${key}=" "${file}" 2>/dev/null | tail -n1 || true)"
  printf '%s' "${line#*=}"
}

public_ipv4() {
  local ip=""
  ip="$(curl -fsS --max-time 3 https://api.ipify.org 2>/dev/null || true)"
  if [[ -n "${ip}" ]]; then
    printf '%s' "${ip}"
    return 0
  fi
  ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  printf '%s' "${ip}"
}

wait_for_healthz() {
  local ok=0
  for _ in $(seq 1 20); do
    if curl -fsS --connect-timeout 2 --max-time 5 "${HEALTHZ}" >/dev/null 2>&1; then
      ok=1
      break
    fi
    sleep 1
  done
  (( ok == 1 )) || die "healthz failed after install (check journalctl -u redgres)"
}

bootstrap_port_open() {
  local addr="${1:-$(env_value REDGRES_BOOTSTRAP_ADDRESS)}"
  local port="${addr##*:}"
  [[ -n "${port}" && "${port}" != "${addr}" ]] || return 1
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

bootstrap_public_url() {
  local addr pub host port
  addr="$(env_value REDGRES_BOOTSTRAP_ADDRESS)"
  pub="$(public_ipv4)"
  [[ -n "${addr}" && -n "${pub}" ]] || return 0
  host="${addr%%:*}"
  port="${addr##*:}"
  if [[ "${host}" == "0.0.0.0" || "${host}" == "::" || "${host}" == "[::]" ]]; then
    printf 'http://%s:%s' "${pub}" "${port}"
  else
    printf 'http://%s:%s' "${host}" "${port}"
  fi
}

# Reads domain_deployment from SQLite; prints DOMAIN_*= shell assignments.
load_domain_config() {
  local sqlite db_path
  sqlite="$(env_value REDGRES_SQLITE_PATH)"
  [[ -n "${sqlite}" ]] || sqlite="${VAR_ROOT}/redgres.db"
  [[ -f "${sqlite}" ]] || return 1
  command -v python3 >/dev/null || return 1
  db_path="${sqlite}"
  eval "$(python3 - "${db_path}" <<'PY'
import json, sqlite3, sys

def q(value: str) -> str:
    return "'" + value.replace("'", "'\\''") + "'"

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
db_host = (d.get("db_hostname") or "").strip()
if not db_host and zone:
    db_host = f"db.{zone}"
rs = (d.get("rs_hostname") or "").strip()
redis_legacy = (d.get("redis_hostname") or "").strip()
if not rs and redis_legacy:
    rs = redis_legacy
pgadmin = (d.get("pgadmin_hostname") or "").strip()
if not pgadmin and zone:
    pgadmin = f"pgadmin.{zone}"
insight = (d.get("redis_insight_hostname") or "").strip()
if not insight and zone:
    insight = f"redis.{zone}"
if redis_legacy and redis_legacy != rs and not d.get("redis_insight_hostname"):
    insight = redis_legacy

if not console:
    raise SystemExit(0)

print("DOMAIN_CONFIGURED=1")
if zone:
    print(f"DOMAIN_ZONE={q(zone)}")
print(f"DOMAIN_CONSOLE={q(console)}")
if db_host:
    print(f"DOMAIN_DB={q(db_host)}")
if rs:
    print(f"DOMAIN_RS={q(rs)}")
if pgadmin:
    print(f"DOMAIN_PGADMIN={q(pgadmin)}")
if insight:
    print(f"DOMAIN_INSIGHT={q(insight)}")
PY
)"
}

print_endpoint_row() {
  local label="$1" url="$2" note="$3"
  printf '%b\n' "    ${BOLD}%-18s${NC} ${url}"
  if [[ -n "${note}" ]]; then
    printf '%b\n' "    ${DIM}%-18s  ${note}${NC}"
  fi
}

print_summary() {
  local version="$1" mode="${2:-installed}" listen pub health_ok="" svc=""
  listen="$(env_value REDGRES_ADDRESS)"
  [[ -n "${listen}" ]] || listen="127.0.0.1:8790"
  pub="$(public_ipv4)"
  DOMAIN_CONFIGURED=""
  DOMAIN_ZONE=""
  DOMAIN_CONSOLE=""
  DOMAIN_DB=""
  DOMAIN_RS=""
  DOMAIN_PGADMIN=""
  DOMAIN_INSIGHT=""
  load_domain_config || true

  if curl -fsS --max-time 3 "${HEALTHZ}" >/dev/null 2>&1; then
    health_ok="yes"
  fi
  if systemctl is-active --quiet redgres.service 2>/dev/null; then
    svc="active"
  else
    svc="inactive"
  fi

  log ""
  if [[ "${mode}" == "unchanged" ]]; then
    log "  ${GREEN}${BOLD}Already on ${version}${NC} ${DIM}(restarted; config preserved)${NC}"
  else
    log "  ${GREEN}${BOLD}Development install complete${NC} → ${version}"
  fi
  if [[ "${health_ok}" == "yes" ]]; then
    log "  ${BOLD}Service${NC}  redgres.service → ${svc}  ·  ${BOLD}Health${NC}  ${GREEN}OK${NC}"
  else
    log "  ${BOLD}Service${NC}  redgres.service → ${svc}  ·  ${BOLD}Health${NC}  ${YELLOW}not ready${NC}"
  fi
  log ""

  if [[ "${DOMAIN_CONFIGURED:-}" == "1" && -n "${DOMAIN_CONSOLE:-}" ]]; then
    log "  ${BOLD}Public endpoints${NC}  ${DIM}(zone ${DOMAIN_ZONE:-unknown})${NC}"
    print_endpoint_row "Redgres UI" "https://${DOMAIN_CONSOLE}" "Tunnel + Cloudflare Access"
    print_endpoint_row "Domain & Network" "https://${DOMAIN_CONSOLE}  →  System → Domain & Network" "change hostnames / Cloudflare / TLS"
    if [[ -n "${DOMAIN_DB:-}" ]]; then
      print_endpoint_row "PostgreSQL" "${DOMAIN_DB}:5432  ·  pooled ${DOMAIN_DB}:6432" "DNS-only (grey cloud) + TLS"
    fi
    if [[ -n "${DOMAIN_RS:-}" ]]; then
      print_endpoint_row "Redis clients" "${DOMAIN_RS}:6380" "DNS-only (grey cloud) + TLS"
    fi
    if [[ -n "${DOMAIN_PGADMIN:-}" ]]; then
      print_endpoint_row "pgAdmin UI" "https://${DOMAIN_PGADMIN}" "Tunnel + Cloudflare Access"
    fi
    if [[ -n "${DOMAIN_INSIGHT:-}" ]]; then
      print_endpoint_row "Redis Insight UI" "https://${DOMAIN_INSIGHT}" "Tunnel + Cloudflare Access"
    fi
    log ""
  fi

  if bootstrap_port_open; then
    local boot_url
    boot_url="$(bootstrap_public_url)"
    log "  ${BOLD}Bootstrap (temporary)${NC}"
    if [[ -n "${boot_url}" ]]; then
      print_endpoint_row "Setup UI" "${boot_url}" "use until console opens through Access; then close in Domain & Network"
    else
      print_endpoint_row "Setup UI" "http://127.0.0.1:8989" "bootstrap listener (check firewall / REDGRES_BOOTSTRAP_ADDRESS)"
    fi
    log ""
  elif [[ "${DOMAIN_CONFIGURED:-}" != "1" ]]; then
    log "  ${BOLD}Bootstrap${NC}"
    print_endpoint_row "Setup UI" "$(bootstrap_public_url || echo "not listening")" "sign in and open System → Domain & Network to configure Cloudflare"
    log ""
  fi

  log "  ${BOLD}This server only${NC}  ${DIM}(not public; tunnel/cloudflared uses loopback)${NC}"
  print_endpoint_row "Loopback UI" "http://${listen}"
  print_endpoint_row "Health" "${HEALTHZ}"
  if [[ -n "${pub}" ]]; then
    print_endpoint_row "Origin IP" "${pub}" "grey-cloud A/AAAA for db + rs in Cloudflare"
  fi
  log ""
  log "  ${DIM}Stable channel: upgrade.sh  ·  Dev channel: install-dev.sh${NC}"
  log ""
}

write_unit() {
  cat >"${UNIT_PATH}" <<'EOF'
[Unit]
Description=Redgres control plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=-/etc/redgres/redgres.env
ExecStart=/opt/redgres/current/redgres serve
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

ensure_env_file() {
  if [[ -f "${ETC_ROOT}/redgres.env" ]]; then
    return 0
  fi
  mkdir -p "${ETC_ROOT}" "${VAR_ROOT}/secrets"
  cat >"${ETC_ROOT}/redgres.env" <<EOF
REDGRES_ENVIRONMENT=development
REDGRES_ADDRESS=127.0.0.1:8790
REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989
REDGRES_BASE_URL=http://127.0.0.1:8989
REDGRES_SQLITE_PATH=/var/lib/redgres/redgres.db
REDGRES_COOKIE_SECURE=false
EOF
  chmod 0640 "${ETC_ROOT}/redgres.env"
}

install_from_dev_release() {
  command -v curl >/dev/null || die "curl is required"
  command -v tar >/dev/null || die "tar is required"
  command -v sha256sum >/dev/null || die "sha256sum is required"

  PREVIOUS=""
  if [[ -L "${OPT_ROOT}/current" ]]; then
    PREVIOUS="$(readlink -f "${OPT_ROOT}/current" || true)"
  fi

  log "  ${CYAN}Fetching dev release (${DEV_TAG})…${NC}"
  JSON="$(curl -fsSL -H 'Accept: application/vnd.github+json' "${API}/releases/tags/${DEV_TAG}")" \
    || die "Dev release not found. Push to master (runs dev-build CI) or trigger Actions → dev-build, then retry."

  TAG="$(printf '%s' "${JSON}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  [[ -n "${TAG}" ]] || die "dev tag_name missing"

  TARBALL_URL="$(printf '%s' "${JSON}" | tr ',' '\n' | sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*redgres_[^"]*_linux_amd64\.tar\.gz\)".*/\1/p' | head -n1)"
  SUMS_URL="$(printf '%s' "${JSON}" | tr ',' '\n' | sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*SHA256SUMS\)".*/\1/p' | head -n1)"
  [[ -n "${TARBALL_URL}" && -n "${SUMS_URL}" ]] || die "dev release assets missing (wait for dev-build workflow)"

  ASSET="${TARBALL_URL##*/}"
  REMOTE_VERSION="${ASSET#redgres_}"
  REMOTE_VERSION="${REMOTE_VERSION%_linux_amd64.tar.gz}"
  CURRENT_VERSION=""
  if [[ -f "${OPT_ROOT}/current/VERSION" ]]; then
    CURRENT_VERSION="$(tr -d '\r\n' <"${OPT_ROOT}/current/VERSION" 2>/dev/null || true)"
  fi
  if [[ -n "${CURRENT_VERSION}" && "${CURRENT_VERSION}" == "${REMOTE_VERSION}" && -x "${OPT_ROOT}/current/redgres" ]]; then
    log "  ${GREEN}Already on ${CURRENT_VERSION}${NC} — restarting service"
    ensure_env_file
    write_unit
    systemctl daemon-reload
    systemctl restart redgres.service
    wait_for_healthz
    print_summary "${CURRENT_VERSION}" unchanged
    return 0
  fi

  WORKDIR="$(mktemp -d /tmp/redgres-dev.XXXXXX)"
  trap 'rm -rf "${WORKDIR}"' EXIT
  cd "${WORKDIR}"

  log "  ${CYAN}Downloading ${ASSET}…${NC}"
  curl -fsSL -o "${ASSET}" "${TARBALL_URL}"
  curl -fsSL -o SHA256SUMS "${SUMS_URL}"
  expected="$(awk -v f="${ASSET}" '$2==f {print $1}' SHA256SUMS)"
  actual="$(sha256sum "${ASSET}" | awk '{print $1}')"
  [[ -n "${expected}" && "${expected}" == "${actual}" ]] || die "dev release checksum verification failed"

  mkdir -p extract
  tar -xzf "${ASSET}" -C extract
  BIN="$(find extract -type f -name redgres | head -n1)"
  VERF="$(find extract -type f -name VERSION | head -n1)"
  [[ -n "${BIN}" && -x "${BIN}" ]] || die "archive missing redgres binary"
  VERSION="$(tr -d '\r\n' <"${VERF}")"

  DEST="${OPT_ROOT}/releases/${VERSION}"
  mkdir -p "${DEST}" "${OPT_ROOT}"
  install -m 0755 "${BIN}" "${DEST}/redgres"
  install -m 0644 "${VERF}" "${DEST}/VERSION"
  ln -sfn "${DEST}" "${OPT_ROOT}/current"

  if [[ -x /usr/local/bin/redgres ]]; then
    log "  ${YELLOW}Replacing legacy /usr/local/bin/redgres with ${OPT_ROOT}/current${NC}"
  fi

  ensure_env_file
  write_unit
  systemctl daemon-reload
  systemctl enable redgres.service >/dev/null 2>&1 || true

  if ! systemctl restart redgres.service; then
    if [[ -n "${PREVIOUS}" && -d "${PREVIOUS}" ]]; then
      ln -sfn "${PREVIOUS}" "${OPT_ROOT}/current"
      systemctl daemon-reload || true
      systemctl restart redgres.service || true
    fi
    die "systemd restart failed; previous release restored when available"
  fi

  log "  ${CYAN}Waiting for healthz…${NC}"
  wait_for_healthz
  print_summary "${VERSION}"
}

install_build_local() {
  log "  ${YELLOW}${BOLD}Local compile mode${NC} ${DIM}(slow; uses CPU/RAM on this server)${NC}"
  command -v git >/dev/null || die "git is required"
  command -v go >/dev/null || die "go is required"
  command -v npm >/dev/null || die "npm is required"
  WORKDIR="$(mktemp -d /tmp/redgres-dev.XXXXXX)"
  trap 'rm -rf "${WORKDIR}"' EXIT
  git clone --depth 1 --branch master "${REPO_URL}" "${WORKDIR}/src"
  cd "${WORKDIR}/src"
  SHA="$(git rev-parse --short HEAD)"
  VERSION="0.0.0-dev.${SHA}"
  (cd web && npm ci && npm run build)
  log "  ${CYAN}Compiling Go (may take several minutes)…${NC}"
  CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o redgres ./cmd/redgres
  DEST="${OPT_ROOT}/releases/${VERSION}"
  mkdir -p "${DEST}"
  install -m 0755 redgres "${DEST}/redgres"
  printf '%s\n' "${VERSION}" >"${DEST}/VERSION"
  ln -sfn "${DEST}" "${OPT_ROOT}/current"
  ensure_env_file
  write_unit
  systemctl daemon-reload
  systemctl enable redgres.service >/dev/null 2>&1 || true
  systemctl restart redgres.service
  wait_for_healthz
  print_summary "${VERSION}"
}

[[ "${EUID}" -eq 0 ]] || die "Run with sudo"

log ""
log "  ${CYAN}${BOLD}Redgres development installer${NC}"
log "  ${DIM}Latest master snapshot via GitHub dev release (CI-built; fast install)${NC}"
log ""

if [[ "${REDGRES_DEV_BUILD_LOCAL:-}" == "1" ]]; then
  install_build_local
else
  install_from_dev_release
fi
