#!/usr/bin/env bash
# Redgres uninstaller (DockLift-parity UX).
# Removes the Redgres application (/opt/redgres + systemd unit).
# Does NOT remove PostgreSQL, Redis, or (by default) /etc/redgres or /var/lib/redgres.
#
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash -s -- -y
# Optional:
#   --purge-config   also remove /etc/redgres
#   --purge-state    also remove /var/lib/redgres (SQLite/control state; not PG/Redis data directories managed outside Redgres)
set -uo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

OPT_ROOT="/opt/redgres"
ETC_ROOT="/etc/redgres"
VAR_ROOT="/var/lib/redgres"
UNIT_PATH="/etc/systemd/system/redgres.service"

FORCE=0
PURGE_CONFIG=0
PURGE_STATE=0

for arg in "$@"; do
  case "${arg}" in
    -y|--force) FORCE=1 ;;
    --purge-config) PURGE_CONFIG=1 ;;
    --purge-state) PURGE_STATE=1 ;;
    --help|-h)
      printf '%s\n' "Usage: uninstall.sh [-y|--force] [--purge-config] [--purge-state]"
      exit 0
      ;;
    *)
      printf '%b\n' "${RED}Unknown arg: ${arg}${NC}" >&2
      exit 1
      ;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { printf '%b\n' "${RED}Error: Run with sudo${NC}" >&2; exit 1; }

printf '%b\n' "${YELLOW}${BOLD}This removes the Redgres application binary and systemd unit.${NC}"
printf '%b\n' "${DIM}PostgreSQL and Redis packages/data are left alone.${NC}"
if [[ "${PURGE_CONFIG}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}--purge-config: will delete ${ETC_ROOT}${NC}"
fi
if [[ "${PURGE_STATE}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}--purge-state: will delete ${VAR_ROOT} (Redgres SQLite/control state)${NC}"
fi

if [[ "${FORCE}" -ne 1 ]]; then
  printf '%b\n' "${YELLOW}Continue? (y/N)${NC}"
  read -r response || true
  if [[ ! "${response}" =~ ^([yY][eE][sS]|[yY])$ ]]; then
    echo "Aborted."
    exit 1
  fi
else
  printf '%b\n' "${YELLOW}Force mode: skipping confirmation.${NC}"
fi

if command -v systemctl >/dev/null 2>&1; then
  printf '%b' "${CYAN}Stopping redgres... ${NC}"
  systemctl stop redgres.service 2>/dev/null || true
  systemctl disable redgres.service 2>/dev/null || true
  systemctl stop cloudflared-redgres.service 2>/dev/null || true
  systemctl disable cloudflared-redgres.service 2>/dev/null || true
  systemctl stop cloudflared-redgres.path 2>/dev/null || true
  systemctl disable cloudflared-redgres.path 2>/dev/null || true
  printf '%b\n' "${GREEN}done${NC}"
fi

rm -f "${UNIT_PATH}" \
  /etc/systemd/system/cloudflared-redgres.service \
  /etc/systemd/system/cloudflared-redgres.path \
  /etc/systemd/system/cloudflared-redgres-restart.service \
  /usr/libexec/redgres/cloudflared-run.sh \
  /usr/libexec/redgres/bootstrap-ufw-remove.sh 2>/dev/null || true
rmdir /usr/libexec/redgres 2>/dev/null || true

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
fi

printf '%b' "${CYAN}Removing ${OPT_ROOT}... ${NC}"
rm -rf "${OPT_ROOT}"
# Legacy ad-hoc path from early live tests
rm -f /usr/local/bin/redgres 2>/dev/null || true
printf '%b\n' "${GREEN}done${NC}"

if [[ "${PURGE_CONFIG}" -eq 1 ]]; then
  printf '%b' "${CYAN}Removing ${ETC_ROOT}... ${NC}"
  rm -rf "${ETC_ROOT}"
  printf '%b\n' "${GREEN}done${NC}"
fi
if [[ "${PURGE_STATE}" -eq 1 ]]; then
  printf '%b' "${CYAN}Removing ${VAR_ROOT}... ${NC}"
  rm -rf "${VAR_ROOT}"
  printf '%b\n' "${GREEN}done${NC}"
fi

printf '%b\n' "${GREEN}${BOLD}Redgres application uninstalled.${NC}"
printf '%b\n' "${DIM}PostgreSQL and Redis were not touched.${NC}"
