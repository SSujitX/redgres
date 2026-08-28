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

print_summary() {
  local version="$1" listen bootstrap pub health_ok="" svc=""
  listen="$(env_value REDGRES_ADDRESS)"
  bootstrap="$(env_value REDGRES_BOOTSTRAP_ADDRESS)"
  [[ -n "${listen}" ]] || listen="127.0.0.1:8790"
  pub="$(public_ipv4)"
  if curl -fsS --max-time 3 "${HEALTHZ}" >/dev/null 2>&1; then
    health_ok="yes"
  fi
  if systemctl is-active --quiet redgres.service 2>/dev/null; then
    svc="active"
  else
    svc="inactive"
  fi

  log ""
  log "  ${GREEN}${BOLD}Development install complete${NC} → ${version}"
  log "  ${BOLD}Binary${NC}      ${OPT_ROOT}/current/redgres"
  log "  ${BOLD}Config${NC}      ${ETC_ROOT}/redgres.env"
  log "  ${BOLD}Service${NC}     redgres.service → ${svc}"
  log ""
  log "  ${BOLD}On this server${NC}"
  log "    Loopback UI   http://${listen}  ${DIM}(tunnel only)${NC}"
  if [[ "${health_ok}" == "yes" ]]; then
    log "    Health        ${HEALTHZ}  ${GREEN}OK${NC}"
  else
    log "    Health        ${HEALTHZ}  ${YELLOW}not ready${NC}"
  fi
  if [[ -n "${pub}" ]]; then
    log "    Public IP     ${pub}"
  fi
  log ""
  log "  ${BOLD}Browser${NC}     open your console hostname (e.g. console.redgres.com) via Cloudflare Access"
  log "  ${DIM}Stable releases (no dev channel): upgrade.sh${NC}"
  log ""
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
