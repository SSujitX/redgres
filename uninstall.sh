#!/usr/bin/env bash
# Redgres uninstaller (DockLift-parity UX).
# Default: remove every Redgres-owned resource so the host is clean for a fresh install.
#
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash -s -- -y
#
# App binary only (preserve PostgreSQL, Redis, config, and data):
#   curl ... | sudo bash -s -- -y --app-only [--purge-config] [--purge-state]
set -uo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

OPT_ROOT="/opt/redgres"
ETC_ROOT="/etc/redgres"
VAR_ROOT="/var/lib/redgres"
BACKUP_ROOT="/var/backups/redgres"
UNIT_PATH="/etc/systemd/system/redgres.service"
LIBEXEC_ROOT="/usr/libexec/redgres"

# Docker resources Redgres may create (never touch unrelated workloads).
DOCKER_NAME_RE='^(redgres|redgres-|redis-insight|redisinsight|pgadmin-redgres)'
DOCKER_PROJECT_RE='^redgres'

FORCE=0
APP_ONLY=0
PURGE_CONFIG=0
PURGE_STATE=0

for arg in "$@"; do
  case "${arg}" in
    -y|--force) FORCE=1 ;;
    --app-only) APP_ONLY=1 ;;
    --purge-config) PURGE_CONFIG=1 ;;
    --purge-state) PURGE_STATE=1 ;;
    --help|-h)
      cat <<EOF
Usage: uninstall.sh [-y|--force] [--app-only] [--purge-config] [--purge-state]

Default (no --app-only): full purge — application, config, SQLite state, tunnel
units, bootstrap firewall rule, Redgres Docker workloads, PostgreSQL clusters,
Redis, and PgBouncer installed for this host.

--app-only: remove only /opt/redgres and systemd units; databases and /etc|/var
            state are preserved unless --purge-config / --purge-state are set.
EOF
      exit 0
      ;;
    *)
      printf '%b\n' "${RED}Unknown arg: ${arg}${NC}" >&2
      exit 1
      ;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { printf '%b\n' "${RED}Error: Run with sudo${NC}" >&2; exit 1; }

step() { printf '%b' "$1"; }
step_done() { printf '%b\n' "${GREEN}done${NC}"; }
step_skip() { printf '%b\n' "${DIM}none${NC}"; }
count_lines() { [[ -z "${1:-}" ]] && { echo 0; return; }; grep -c . <<<"${1}" || echo 0; }

if [[ "${APP_ONLY}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}${BOLD}This removes the Redgres application binary and systemd unit.${NC}"
  printf '%b\n' "${DIM}PostgreSQL and Redis are left alone unless you pass --purge-config / --purge-state.${NC}"
else
  printf '%b\n' ""
  printf '%b\n' "${RED}${BOLD}  WARNING: This permanently deletes ALL Redgres data on this host.${NC}"
  printf '%b\n' "${RED}${BOLD}  PostgreSQL clusters, Redis, PgBouncer, config, SQLite state, and backups under ${BACKUP_ROOT} will be destroyed.${NC}"
  printf '%b\n' ""
  printf '%b\n' "${YELLOW}${BOLD}  Back up before continuing:${NC}"
  printf '%b\n' "${YELLOW}    · PostgreSQL  pg_dumpall or your usual backup tool${NC}"
  printf '%b\n' "${YELLOW}    · Redis       RDB/AOF copy or redis-cli SAVE + archive data dir${NC}"
  printf '%b\n' "${YELLOW}    · Redgres     copy ${VAR_ROOT} and ${ETC_ROOT}${NC}"
  printf '%b\n' "${DIM}  There is no undo. This script does not create or verify backups.${NC}"
  printf '%b\n' ""
  printf '%b\n' "${DIM}  Also removes: tunnel units, bootstrap :8989 firewall rule, Redgres Docker workloads.${NC}"
  printf '%b\n' "${DIM}  Docker Engine stays installed.${NC}"
  printf '%b\n' ""
fi

if [[ "${APP_ONLY}" -eq 1 && "${PURGE_CONFIG}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}--purge-config: will delete ${ETC_ROOT}${NC}"
fi
if [[ "${APP_ONLY}" -eq 1 && "${PURGE_STATE}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}--purge-state: will delete ${VAR_ROOT}${NC}"
fi

if [[ "${FORCE}" -ne 1 ]]; then
  printf '%b\n' "${YELLOW}${BOLD}Type yes to confirm uninstall:${NC}"
  read -r response || true
  if [[ ! "${response}" =~ ^([yY][eE][sS]|[yY])$ ]]; then
    echo "Aborted."
    exit 1
  fi
else
  printf '%b\n' "${YELLOW}Force mode (-y): warnings shown above; confirmation skipped.${NC}"
fi

redgres_docker_containers() {
  command -v docker >/dev/null 2>&1 || return 0
  docker ps -a --format '{{.ID}}|{{.Names}}|{{.Label "com.docker.compose.project"}}' 2>/dev/null |
    awk -F'|' -v n="${DOCKER_NAME_RE}" -v p="${DOCKER_PROJECT_RE}" \
      '$2 ~ n || $3 ~ p {print $1}'
}

redgres_docker_images() {
  command -v docker >/dev/null 2>&1 || return 0
  docker images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null |
    awk '$0 ~ /^(redgres|redis-insight|redisinsight|pgadmin-redgres)/ && $0 !~ / /'
}

redgres_docker_volumes() {
  command -v docker >/dev/null 2>&1 || return 0
  docker volume ls --format '{{.Name}}|{{.Labels}}' 2>/dev/null |
    awk -F'|' -v n="${DOCKER_NAME_RE}" \
      '$1 ~ n || $2 ~ /com\.docker\.compose\.project=redgres/ {print $1}'
}

redgres_docker_networks() {
  command -v docker >/dev/null 2>&1 || return 0
  docker network ls --format '{{.Name}}|{{.Labels}}' 2>/dev/null |
    awk -F'|' -v n="${DOCKER_NAME_RE}" \
      '$1 ~ n || $2 ~ /com\.docker\.compose\.project=redgres/ {print $1}'
}

stop_systemd_unit() {
  local unit="$1"
  systemctl stop "${unit}" 2>/dev/null || true
  systemctl disable "${unit}" 2>/dev/null || true
}

remove_bootstrap_firewall() {
  if [[ -x "${LIBEXEC_ROOT}/bootstrap-ufw-remove.sh" ]]; then
    "${LIBEXEC_ROOT}/bootstrap-ufw-remove.sh" 2>/dev/null || true
    return 0
  fi
  command -v ufw >/dev/null 2>&1 || return 0
  local i rule
  for ((i = 0; i < 20; i++)); do
    rule="$(ufw status numbered 2>/dev/null | grep -E '8989' | head -1 | grep -oE '[[:space:]]*[0-9]+' | tr -d ' ' || true)"
    [[ -n "${rule}" ]] || break
    ufw --force delete "${rule}" 2>/dev/null || break
  done
  ufw delete allow 8989/tcp 2>/dev/null || true
}

purge_postgresql() {
  if command -v pg_lsclusters >/dev/null 2>&1; then
    while read -r ver name _rest; do
      [[ -n "${ver}" && -n "${name}" ]] || continue
      pg_dropcluster --stop "${ver}" "${name}" 2>/dev/null || true
    done < <(pg_lsclusters --no-header 2>/dev/null || true)
  fi
  stop_systemd_unit postgresql.service
  stop_systemd_unit postgresql@.service
  if command -v apt-get >/dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get purge -y postgresql postgresql-* 2>/dev/null || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf remove -y postgresql\* 2>/dev/null || true
  elif command -v yum >/dev/null 2>&1; then
    yum remove -y postgresql\* 2>/dev/null || true
  fi
  rm -rf /var/lib/postgresql 2>/dev/null || true
}

purge_redis_native() {
  stop_systemd_unit redis-server.service
  stop_systemd_unit redis.service
  if command -v apt-get >/dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get purge -y redis-server redis 2>/dev/null || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf remove -y redis 2>/dev/null || true
  elif command -v yum >/dev/null 2>&1; then
    yum remove -y redis 2>/dev/null || true
  fi
}

purge_pgbouncer() {
  stop_systemd_unit pgbouncer.service
  if command -v apt-get >/dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get purge -y pgbouncer 2>/dev/null || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf remove -y pgbouncer 2>/dev/null || true
  elif command -v yum >/dev/null 2>&1; then
    yum remove -y pgbouncer 2>/dev/null || true
  fi
}

# ── 1. Stop Redgres + tunnel units ───────────────────────────────────────────
if command -v systemctl >/dev/null 2>&1; then
  step "  ${CYAN}[1/7]${NC} Stopping Redgres and tunnel services... "
  stop_systemd_unit redgres.service
  stop_systemd_unit cloudflared-redgres.service
  stop_systemd_unit cloudflared-redgres.path
  stop_systemd_unit cloudflared-redgres-restart.service
  step_done
else
  step "  ${CYAN}[1/7]${NC} systemd not found "
  step_skip
fi

# ── 2. Bootstrap firewall ────────────────────────────────────────────────────
step "  ${CYAN}[2/7]${NC} Removing bootstrap firewall rule (8989)... "
remove_bootstrap_firewall
step_done

# ── 3. Docker workloads ──────────────────────────────────────────────────────
if [[ "${APP_ONLY}" -eq 0 ]]; then
  step "  ${CYAN}[3/7]${NC} Removing Redgres Docker containers... "
  if command -v docker >/dev/null 2>&1; then
    if [[ -f "${VAR_ROOT}/redis/docker-compose.yml" ]]; then
      (cd "${VAR_ROOT}/redis" && docker compose down --volumes --remove-orphans 2>/dev/null) || true
    fi
    CONTAINERS="$(redgres_docker_containers || true)"
    if [[ -n "${CONTAINERS}" ]]; then
      docker stop ${CONTAINERS} 2>/dev/null || true
      docker rm -f -v ${CONTAINERS} 2>/dev/null || true
      step_done
      printf '%b\n' "         ${DIM}removed $(count_lines "${CONTAINERS}") container(s)${NC}"
    else
      step_skip
    fi
    IMAGES="$(redgres_docker_images || true)"
    [[ -n "${IMAGES}" ]] && docker rmi -f ${IMAGES} 2>/dev/null || true
    VOLUMES="$(redgres_docker_volumes || true)"
    [[ -n "${VOLUMES}" ]] && docker volume rm -f ${VOLUMES} 2>/dev/null || true
    NETWORKS="$(redgres_docker_networks || true)"
    [[ -n "${NETWORKS}" ]] && docker network rm ${NETWORKS} 2>/dev/null || true
  else
    step_skip
  fi
else
  step "  ${CYAN}[3/7]${NC} Docker cleanup "
  step_skip
fi

# ── 4. PostgreSQL / Redis / PgBouncer ───────────────────────────────────────
if [[ "${APP_ONLY}" -eq 0 ]]; then
  step "  ${CYAN}[4/7]${NC} Removing PostgreSQL clusters and packages... "
  purge_postgresql
  step_done

  step "  ${CYAN}[5/7]${NC} Removing Redis and PgBouncer... "
  purge_redis_native
  purge_pgbouncer
  step_done
else
  step "  ${CYAN}[4/7]${NC} PostgreSQL/Redis/PgBouncer "
  step_skip
  step "  ${CYAN}[5/7]${NC} (skipped — --app-only) "
  step_skip
fi

# ── 6. Systemd units + libexec ───────────────────────────────────────────────
step "  ${CYAN}[6/7]${NC} Removing systemd units and helpers... "
rm -f "${UNIT_PATH}" \
  /etc/systemd/system/cloudflared-redgres.service \
  /etc/systemd/system/cloudflared-redgres.path \
  /etc/systemd/system/cloudflared-redgres-restart.service \
  2>/dev/null || true
rm -rf "${LIBEXEC_ROOT}" 2>/dev/null || true
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload 2>/dev/null || true
  systemctl reset-failed 2>/dev/null || true
fi
step_done

# ── 7. Filesystem ────────────────────────────────────────────────────────────
step "  ${CYAN}[7/7]${NC} Removing Redgres files... "
rm -rf "${OPT_ROOT}" 2>/dev/null || true
rm -f /usr/local/bin/redgres 2>/dev/null || true

if [[ "${APP_ONLY}" -eq 0 || "${PURGE_CONFIG}" -eq 1 ]]; then
  rm -rf "${ETC_ROOT}" 2>/dev/null || true
fi
if [[ "${APP_ONLY}" -eq 0 || "${PURGE_STATE}" -eq 1 ]]; then
  rm -rf "${VAR_ROOT}" 2>/dev/null || true
fi
if [[ "${APP_ONLY}" -eq 0 ]]; then
  rm -rf "${BACKUP_ROOT}" /var/log/redgres 2>/dev/null || true
fi
step_done

if [[ "${APP_ONLY}" -eq 0 ]] && command -v apt-get >/dev/null 2>&1; then
  DEBIAN_FRONTEND=noninteractive apt-get autoremove -y 2>/dev/null || true
fi

if id redgres >/dev/null 2>&1; then
  userdel redgres 2>/dev/null || true
fi
if getent group redgres >/dev/null 2>&1; then
  groupdel redgres 2>/dev/null || true
fi

BUSY=""
if command -v ss >/dev/null 2>&1; then
  BUSY="$(ss -tlnH 2>/dev/null | awk '{print $4}' | grep -oE ':(8790|8989|5432|6379|6380|6432|5540|5050)$' | sort -u | tr '\n' ' ' || true)"
fi

log=""
if [[ "${APP_ONLY}" -eq 1 ]]; then
  log="${GREEN}${BOLD}Redgres application uninstalled.${NC}"
else
  log="${GREEN}${BOLD}Redgres fully removed. Host is clean for a fresh install.${NC}"
fi
printf '%b\n' ""
printf '%b\n' "  ${log}"
if [[ -n "${BUSY}" ]]; then
  printf '%b\n' "  ${YELLOW}Note:${NC} ports still in use (non-Redgres or stale socket): ${BUSY}"
  printf '%b\n' "  ${DIM}Check with: ss -tlnp${NC}"
else
  printf '%b\n' "  ${DIM}Redgres ports (8790, 8989, 5432, 6379, 6380, 6432) are free.${NC}"
fi
if [[ "${APP_ONLY}" -eq 0 ]]; then
  printf '%b\n' "  ${DIM}Docker Engine was left installed. Reinstall: curl install-dev.sh | bash${NC}"
else
  printf '%b\n' "  ${DIM}PostgreSQL and Redis were not removed (--app-only).${NC}"
fi
printf '%b\n' ""
