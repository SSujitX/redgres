#!/usr/bin/env bash
# Redgres upgrade (DockLift-parity UX). Always targets GitHub latest release.
# Preserves /etc/redgres, /var/lib/redgres, PostgreSQL, and Redis data.
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
log "  ${DIM}Preserves config and data; installs latest GitHub release only${NC}"
log ""

JSON="$(curl -fsSL -H 'Accept: application/vnd.github+json' "${API}/releases/latest")" || die "GitHub latest release not found"
TAG="$(printf '%s' "${JSON}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
VERSION="${TAG#v}"
[[ -n "${VERSION}" ]] || die "latest tag_name missing"
ASSET="redgres_${VERSION}_linux_amd64.tar.gz"
TARBALL_URL="$(printf '%s' "${JSON}" | tr ',' '\n' | sed -n "s/.*\"browser_download_url\"[[:space:]]*:[[:space:]]*\"\\([^\"]*${ASSET}\\)\".*/\\1/p" | head -n1)"
SUMS_URL="$(printf '%s' "${JSON}" | tr ',' '\n' | sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*SHA256SUMS\)".*/\1/p' | head -n1)"
[[ -n "${TARBALL_URL}" && -n "${SUMS_URL}" ]] || die "latest release assets missing (${ASSET} / SHA256SUMS)"

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

systemctl daemon-reload
if ! systemctl restart redgres.service; then
  if [[ -n "${PREVIOUS}" && -d "${PREVIOUS}" ]]; then
    ln -sfn "${PREVIOUS}" "${OPT_ROOT}/current"
    systemctl daemon-reload || true
    systemctl restart redgres.service || true
  fi
  die "systemd restart failed; previous release restored when available"
fi

ok=0
for _ in $(seq 1 15); do
  if curl -fsS --connect-timeout 2 --max-time 5 "${HEALTHZ}" >/dev/null 2>&1; then
    ok=1
    break
  fi
  sleep 1
done
if [[ "${ok}" -ne 1 ]]; then
  if [[ -n "${PREVIOUS}" && -d "${PREVIOUS}" ]]; then
    ln -sfn "${PREVIOUS}" "${OPT_ROOT}/current"
    systemctl daemon-reload || true
    systemctl restart redgres.service || true
  fi
  die "healthz failed after upgrade; previous release restored when available"
fi

log ""
log "  ${GREEN}${BOLD}Upgrade complete${NC} → ${VERSION}"
log "  Data preserved: /etc/redgres, /var/lib/redgres, PostgreSQL, Redis"
log "  Rollback app only: deploy/install.sh rollback --non-interactive --to PRIOR_VERSION"
log ""
