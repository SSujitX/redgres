#!/usr/bin/env bash
# Redgres public installer (DockLift-parity UX).
# Install latest GitHub release, or pin with v= / REDGRES_VERSION=.
#
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install.sh | sudo bash
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install.sh | sudo bash -s -- v=0.1.0
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install.sh | sudo REDGRES_VERSION=0.1.0 bash
#
# Pinning/downgrading with install.sh replaces the application release and does
# not run upgrade.sh's preserve-data path. Prefer upgrade.sh to move forward;
# backup before installing an older tag.
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

REPO="SSujitX/redgres"
OPT_ROOT="/opt/redgres"
ETC_ROOT="/etc/redgres"
VAR_ROOT="/var/lib/redgres"
UNIT_PATH="/etc/systemd/system/redgres.service"
API="https://api.github.com/repos/${REPO}"

die() { printf '%b\n' "${RED}Error: $*${NC}" >&2; exit 1; }
log() { printf '%b\n' "$*"; }

[[ "${EUID}" -eq 0 ]] || die "Run with sudo"

REQUESTED_VERSION="${REDGRES_VERSION:-}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    v=*|version=*|--version=*) REQUESTED_VERSION="${1#*=}" ;;
    --version|-v) shift; REQUESTED_VERSION="${1:-}" ;;
    latest|LATEST) REQUESTED_VERSION="" ;;
    v[0-9]*|[0-9]*.[0-9]*) REQUESTED_VERSION="$1" ;;
    --force) FORCE=1 ;;
    *) die "unknown install arg: $1 (use v=0.1.0 or REDGRES_VERSION=0.1.0)" ;;
  esac
  shift || true
done
FORCE="${FORCE:-0}"

normalize_tag() {
  local raw="$1"
  raw="$(printf '%s' "${raw}" | tr -d '[:space:]')"
  [[ -z "${raw}" || "${raw}" == "latest" || "${raw}" == "LATEST" ]] && { printf ''; return 0; }
  case "${raw}" in
    v*) printf '%s' "${raw}" ;;
    *) printf 'v%s' "${raw}" ;;
  esac
}

TAG="$(normalize_tag "${REQUESTED_VERSION}")"

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
  if [[ -z "${tag}" ]]; then
    url="${API}/releases/latest"
  else
    url="${API}/releases/tags/${tag}"
  fi
  body="$(curl -fsSL -H 'Accept: application/vnd.github+json' "${url}")" || die "GitHub release not found"
  printf '%s' "${body}"
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
}

log ""
log "  ${CYAN}${BOLD}Redgres installer${NC}"
log "  ${DIM}One secure control plane for PostgreSQL and Redis${NC}"
log ""

RELEASE_JSON="$(resolve_release "${TAG}")"
pick_assets "${RELEASE_JSON}"

WORKDIR="$(mktemp -d /tmp/redgres-install.XXXXXX)"
trap 'rm -rf "${WORKDIR}"' EXIT
cd "${WORKDIR}"

log "  Downloading ${ASSET_NAME}..."
curl -fsSL -o "${ASSET_NAME}" "${TARBALL_URL}"
curl -fsSL -o SHA256SUMS "${SUMS_URL}"

grep -E "^[0-9a-f]{64}  ${ASSET_NAME}\$" SHA256SUMS >/dev/null || die "SHA256SUMS missing exact entry for ${ASSET_NAME}"
sha256sum -c SHA256SUMS --ignore-missing >/dev/null 2>&1 || {
  # Strict: verify our asset line only
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
install -m 0755 "${BIN}" "${DEST}/redgres"
install -m 0644 "${VERF}" "${DEST}/VERSION"
ln -sfn "${DEST}" "${OPT_ROOT}/current"

if [[ ! -f "${ETC_ROOT}/redgres.env" ]]; then
  cat >"${ETC_ROOT}/redgres.env" <<EOF
REDGRES_ENVIRONMENT=production
REDGRES_ADDRESS=127.0.0.1:8790
REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989
REDGRES_BASE_URL=http://127.0.0.1:8989
REDGRES_SQLITE_PATH=/var/lib/redgres/redgres.db
REDGRES_COOKIE_SECURE=false
EOF
  chmod 0640 "${ETC_ROOT}/redgres.env"
fi

cat >"${UNIT_PATH}" <<EOF
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

systemctl daemon-reload
systemctl enable redgres.service >/dev/null
systemctl restart redgres.service || log "  ${YELLOW}Service start deferred (create an owner first if needed).${NC}"

log ""
log "  ${GREEN}${BOLD}Redgres application installed${NC} (${VERSION_FILE})"
log "  Binary: ${OPT_ROOT}/current/redgres"
log "  Config: ${ETC_ROOT}/redgres.env"
log ""
log "  Next:"
log "    1. Wire PostgreSQL/Redis in ${ETC_ROOT}/redgres.env (or use deploy/install.sh for services)."
log "    2. Create owner: /opt/redgres/current/redgres create-owner --username admin --sqlite-path /var/lib/redgres/redgres.db"
log "    3. Open bootstrap UI (source-restricted): http://SERVER_IP:8989"
log "    4. Prefer upgrade.sh for future application updates."
log ""
