#!/usr/bin/env bash
# Redgres development installer (DockLift-parity UX).
# Builds unreleased code from master and installs under /opt/redgres.
#
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install-dev.sh | sudo bash
#
# Warning: installs unreleased code from master. Prefer install.sh for stable deployments.
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

REPO_URL="https://github.com/SSujitX/redgres.git"
OPT_ROOT="/opt/redgres"
ETC_ROOT="/etc/redgres"
VAR_ROOT="/var/lib/redgres"
UNIT_PATH="/etc/systemd/system/redgres.service"

die() { printf '%b\n' "${RED}Error: $*${NC}" >&2; exit 1; }
log() { printf '%b\n' "$*"; }

[[ "${EUID}" -eq 0 ]] || die "Run with sudo"

log ""
log "  ${CYAN}${BOLD}Redgres development installer${NC}"
log "  ${YELLOW}${BOLD}Warning: installs latest master (unreleased).${NC}"
log ""

command -v git >/dev/null || die "git is required"
command -v go >/dev/null || die "go is required"
command -v npm >/dev/null || die "npm is required"
command -v tar >/dev/null || die "tar is required"

WORKDIR="$(mktemp -d /tmp/redgres-dev.XXXXXX)"
trap 'rm -rf "${WORKDIR}"' EXIT
git clone --depth 1 --branch master "${REPO_URL}" "${WORKDIR}/src"
cd "${WORKDIR}/src"
SHA="$(git rev-parse --short HEAD)"
VERSION="0.0.0-dev.${SHA}"

(cd web && npm ci && npm run build)
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o redgres ./cmd/redgres
printf '%s\n' "${VERSION}" >VERSION

DEST="${OPT_ROOT}/releases/${VERSION}"
mkdir -p "${DEST}" "${ETC_ROOT}" "${VAR_ROOT}/secrets"
install -m 0755 redgres "${DEST}/redgres"
install -m 0644 VERSION "${DEST}/VERSION"
ln -sfn "${DEST}" "${OPT_ROOT}/current"

if [[ ! -f "${ETC_ROOT}/redgres.env" ]]; then
  cat >"${ETC_ROOT}/redgres.env" <<EOF
REDGRES_ENVIRONMENT=development
REDGRES_ADDRESS=127.0.0.1:8790
REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989
REDGRES_BASE_URL=http://127.0.0.1:8989
REDGRES_SQLITE_PATH=/var/lib/redgres/redgres.db
REDGRES_COOKIE_SECURE=false
EOF
  chmod 0640 "${ETC_ROOT}/redgres.env"
fi

cat >"${UNIT_PATH}" <<'EOF'
[Unit]
Description=Redgres control plane (development build)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=-/etc/redgres/redgres.env
ExecStart=/opt/redgres/current/redgres serve
Restart=on-failure
RestartSec=3
ReadWritePaths=/var/lib/redgres /etc/redgres

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable redgres.service >/dev/null
systemctl restart redgres.service || log "  ${YELLOW}Service start deferred.${NC}"

log ""
log "  ${GREEN}${BOLD}Development build installed${NC} (${VERSION})"
log "  Prefer install.sh / upgrade.sh for release channels."
log ""
