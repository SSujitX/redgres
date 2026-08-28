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

run_go_build() {
  log "  ${CYAN}[4/5] Compiling redgres binary (Go)…${NC}"
  log "  ${DIM}First build on this server: downloads modules, then compiles (often 3–10 min).${NC}"
  log "  ${DIM}No output during compile is normal — do not interrupt.${NC}"
  local started=$SECONDS pid
  CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o redgres ./cmd/redgres &
  pid=$!
  while kill -0 "${pid}" 2>/dev/null; do
    if (( SECONDS - started >= 30 && (SECONDS - started) % 30 == 0 )); then
      log "  ${DIM}… still compiling ($((SECONDS - started))s)${NC}"
    fi
    sleep 5
  done
  wait "${pid}"
  log "  ${GREEN}Go build finished${NC} ($((SECONDS - started))s)"
}

print_install_summary() {
  local version="$1" listen bootstrap pub health_ok="" svc=""
  listen="$(env_value REDGRES_ADDRESS)"
  bootstrap="$(env_value REDGRES_BOOTSTRAP_ADDRESS)"
  [[ -n "${listen}" ]] || listen="127.0.0.1:8790"
  [[ -n "${bootstrap}" ]] || bootstrap="0.0.0.0:8989"
  pub="$(public_ipv4)"
  if curl -fsS --max-time 3 "http://${listen}/api/v1/healthz" >/dev/null 2>&1; then
    health_ok="yes"
  fi
  if systemctl is-active --quiet redgres.service 2>/dev/null; then
    svc="active"
  else
    svc="inactive"
  fi

  log ""
  log "  ${GREEN}${BOLD}Development build installed${NC}"
  log "  ${BOLD}Version${NC}     ${VERSION}  ${DIM}(git ${SHA} on master)${NC}"
  log "  ${BOLD}Binary${NC}      ${OPT_ROOT}/current/redgres"
  log "  ${BOLD}Config${NC}      ${ETC_ROOT}/redgres.env"
  log "  ${BOLD}Service${NC}     redgres.service → ${svc}"
  log ""
  log "  ${BOLD}Endpoints on this server${NC}"
  log "    ${CYAN}Loopback UI${NC}   http://${listen}  ${DIM}(cloudflared / tunnel only; not public)${NC}"
  if [[ "${health_ok}" == "yes" ]]; then
    log "    ${CYAN}Health${NC}        http://${listen}/api/v1/healthz  ${GREEN}OK${NC}"
  else
    log "    ${CYAN}Health${NC}        http://${listen}/api/v1/healthz  ${YELLOW}not ready yet${NC}"
  fi
  if [[ -n "${bootstrap}" && "${bootstrap}" != "0.0.0.0:0" && "${bootstrap}" != ":0" ]]; then
    log "    ${CYAN}Bootstrap${NC}     http://${bootstrap/#0.0.0.0/${pub:-127.0.0.1}}  ${DIM}(temporary setup UI; firewall-restricted)${NC}"
  fi
  if [[ -n "${pub}" ]]; then
    log "    ${CYAN}Public IP${NC}     ${pub}"
  fi
  log ""
  log "  ${BOLD}If domain is already configured${NC}"
  log "    Open your console hostname through Cloudflare Access (e.g. console.redgres.com)."
  log "    Domain & Network → endpoint cards (console, db, rs, pgadmin, redis Insight)."
  log ""
  log "  ${BOLD}Commands${NC}"
  log "    systemctl status redgres.service"
  log "    /opt/redgres/current/redgres version"
  log "    ${DIM}Stable updates (no Go/Node on server): upgrade.sh${NC}"
  log ""
}

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

log "  ${CYAN}[1/5] Cloning master…${NC}"
WORKDIR="$(mktemp -d /tmp/redgres-dev.XXXXXX)"
trap 'rm -rf "${WORKDIR}"' EXIT
git clone --depth 1 --branch master "${REPO_URL}" "${WORKDIR}/src"
cd "${WORKDIR}/src"
SHA="$(git rev-parse --short HEAD)"
VERSION="0.0.0-dev.${SHA}"
log "  ${GREEN}Source${NC} ${SHA} from master"

log "  ${CYAN}[2/5] Web dependencies (npm ci)…${NC}"
(cd web && npm ci)
log "  ${CYAN}[3/5] Web production build…${NC}"
(cd web && npm run build)
log "  ${GREEN}Web UI built${NC}"

run_go_build
printf '%s\n' "${VERSION}" >VERSION

log "  ${CYAN}[5/5] Installing under ${OPT_ROOT} and restarting service…${NC}"

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
if systemctl restart redgres.service; then
  log "  ${GREEN}redgres.service restarted${NC}"
else
  log "  ${YELLOW}Service restart deferred (check journalctl -u redgres).${NC}"
fi

print_install_summary "${VERSION}"
