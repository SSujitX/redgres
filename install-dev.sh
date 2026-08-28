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

ensure_build_tools() {
  local missing=()
  command -v git >/dev/null || missing+=("git")
  command -v go >/dev/null || missing+=("go")
  command -v npm >/dev/null || missing+=("npm")
  command -v tar >/dev/null || missing+=("tar")
  if ((${#missing[@]} == 0)); then
    return 0
  fi
  if ! command -v apt-get >/dev/null; then
    die "Missing build tools (${missing[*]}). Install git, Go (see go.mod), Node ${NODE_MAJOR}.x, and tar, then re-run."
  fi
  log "  ${CYAN}Installing build tools (${missing[*]})…${NC}"
  log "  ${DIM}Note: release install.sh/upgrade.sh use pre-built binaries and do not need Go/Node.${NC}"
  log "  ${DIM}install-dev builds from Git master on this machine.${NC}"
  export DEBIAN_FRONTEND=noninteractive
  export NEEDRESTART_MODE=a
  export UCF_FORCE_CONFFOLD=1
  apt-get update -qq
  apt-get install -y -qq --no-upgrade \
    -o Dpkg::Options::=--force-confold \
    ca-certificates curl git tar
  if ! command -v npm >/dev/null; then
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
    apt-get install -y -qq --no-upgrade \
      -o Dpkg::Options::=--force-confold \
      nodejs
  fi
  if ! command -v go >/dev/null; then
    local go_ver="${GO_VERSION}"
    local arch="amd64"
    case "$(uname -m)" in
      aarch64|arm64) arch="arm64" ;;
      x86_64|amd64) arch="amd64" ;;
      *) die "Unsupported CPU architecture for Go bootstrap: $(uname -m)" ;;
    esac
    local tarball="go${go_ver}.linux-${arch}.tar.gz"
    local url="https://go.dev/dl/${tarball}"
    log "  ${CYAN}Downloading Go ${go_ver}…${NC}"
    curl -fsSL "${url}" -o "/tmp/${tarball}" || die "Could not download Go ${go_ver} from ${url}"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"
    export PATH="/usr/local/go/bin:${PATH}"
    if [[ ! -f /etc/profile.d/redgres-go.sh ]]; then
      printf '%s\n' 'export PATH="/usr/local/go/bin:$PATH"' >/etc/profile.d/redgres-go.sh
      chmod 0644 /etc/profile.d/redgres-go.sh
    fi
  fi
  export PATH="/usr/local/go/bin:${PATH}"
  command -v git >/dev/null || die "git is required"
  command -v go >/dev/null || die "go is required (expected /usr/local/go/bin/go)"
  command -v npm >/dev/null || die "npm is required"
  command -v tar >/dev/null || die "tar is required"
  log "  ${GREEN}Build tools ready:${NC} go $(go version | awk '{print $3}'), node $(node -v), npm $(npm -v)"
}

GO_VERSION="$(curl -fsSL "https://raw.githubusercontent.com/SSujitX/Redgres/master/go.mod" 2>/dev/null | awk '/^go / {print $2; exit}')"
GO_VERSION="${GO_VERSION:-1.27.0}"
NODE_MAJOR="24"

[[ "${EUID}" -eq 0 ]] || die "Run with sudo"

log ""
log "  ${CYAN}${BOLD}Redgres development installer${NC}"
log "  ${YELLOW}${BOLD}Warning: installs latest master (unreleased).${NC}"
log ""

ensure_build_tools
export PATH="/usr/local/go/bin:${PATH}"

log "  ${CYAN}Cloning master and building…${NC}"
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
