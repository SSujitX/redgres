#!/usr/bin/env bash
# Redgres uninstaller (DockLift-parity UX).
# Default: remove every Redgres-owned resource so the host is clean for a fresh install.
#
# Interactive (type yes at prompt — works with curl | bash via /dev/tty):
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash
# Non-interactive:
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
KEEP_REMOTE=0

for arg in "$@"; do
  case "${arg}" in
    -y|--force) FORCE=1 ;;
    --app-only) APP_ONLY=1 ;;
    --purge-config) PURGE_CONFIG=1 ;;
    --purge-state) PURGE_STATE=1 ;;
    --keep-remote) KEEP_REMOTE=1 ;;
    --help|-h)
      cat <<EOF
Usage: uninstall.sh [-y|--force] [--app-only] [--keep-remote] [--purge-config] [--purge-state]

Default (no --app-only): full purge — application, config, SQLite state, Cloudflare
DNS/tunnel/Access (via stored API token), certbot db/rs certificates, tunnel units,
bootstrap firewall rule, Redgres Docker workloads, PostgreSQL, Redis, PgBouncer.

--keep-remote: local purge only; skip Cloudflare API and certbot delete.
--app-only: remove only /opt/redgres and systemd units; databases preserved unless
            --purge-config / --purge-state are set.
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

printf '%b\n' ""
printf '%b\n' "  ${CYAN}${BOLD}Redgres uninstaller${NC}"
printf '%b\n' "  ${DIM}Removes Redgres from this host. Full purge is destructive.${NC}"
printf '%b\n' ""

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
  printf '%b\n' "${DIM}  Also removes: Cloudflare DNS/tunnel/Access (API), certbot db/rs certs, tunnel units,${NC}"
  printf '%b\n' "${DIM}  bootstrap :8989 firewall rule, Redgres Docker workloads, cloudflared package.${NC}"
  printf '%b\n' "${DIM}  Docker Engine stays installed. Use --keep-remote to skip Cloudflare/certbot.${NC}"
  printf '%b\n' ""
fi

if [[ "${APP_ONLY}" -eq 1 && "${PURGE_CONFIG}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}--purge-config: will delete ${ETC_ROOT}${NC}"
fi
if [[ "${APP_ONLY}" -eq 1 && "${PURGE_STATE}" -eq 1 ]]; then
  printf '%b\n' "${YELLOW}--purge-state: will delete ${VAR_ROOT}${NC}"
fi

confirm_uninstall() {
  local response=""
  if [[ -t 0 ]]; then
    :
  elif [[ -e /dev/tty ]]; then
    # curl | bash (and curl | sudo bash) leaves stdin on the pipe — attach the real terminal.
    exec 0</dev/tty
  else
    printf '%b\n' "${RED}Error: No terminal for confirmation.${NC}" >&2
    printf '%b\n' "${DIM}Use: curl -fsSL .../uninstall.sh | sudo bash -s -- -y${NC}" >&2
    printf '%b\n' "${DIM}Or:  curl -fsSL .../uninstall.sh -o uninstall.sh && sudo bash uninstall.sh${NC}" >&2
    exit 1
  fi
  printf '%b\n' "${YELLOW}${BOLD}Uninstall this server?${NC}  ${DIM}yes / y to continue  ·  no / n to abort${NC}"
  printf '%b' "${YELLOW}${BOLD}Choice [y/N]: ${NC}"
  read -r response || true
  case "${response}" in
    [yY]|[yY][eE][sS]) return 0 ;;
    [nN]|[nN][oO]|"")
      printf '%b\n' "${DIM}Aborted.${NC}"
      exit 1
      ;;
    *)
      printf '%b\n' "${DIM}Aborted.${NC}"
      exit 1
      ;;
  esac
}

if [[ "${FORCE}" -ne 1 ]]; then
  confirm_uninstall
  printf '%b\n' ""
else
  printf '%b\n' "${YELLOW}Force mode (-y): warnings shown above; confirmation skipped.${NC}"
  printf '%b\n' ""
fi

env_value_from_file() {
  local key="$1" file="${2:-${ETC_ROOT}/redgres.env}" line
  [[ -f "${file}" ]] || return 0
  line="$(grep -E "^${key}=" "${file}" 2>/dev/null | tail -n1 || true)"
  printf '%s' "${line#*=}"
}

DOMAIN_SNAPSHOT="$(mktemp /tmp/redgres-uninstall-domain.XXXXXX)"
CF_API_STATUS="unknown"
trap 'rm -f "${DOMAIN_SNAPSHOT}"' EXIT

write_domain_snapshot() {
  local sqlite env_file
  : >"${DOMAIN_SNAPSHOT}"
  env_file="${ETC_ROOT}/redgres.env"
  sqlite="$(env_value_from_file REDGRES_SQLITE_PATH "${env_file}")"
  [[ -n "${sqlite}" ]] || sqlite="${VAR_ROOT}/redgres.db"
  [[ -f "${sqlite}" ]] || return 0
  command -v python3 >/dev/null 2>&1 || return 0
  python3 - "${env_file}" "${sqlite}" "${DOMAIN_SNAPSHOT}" <<'PY' || true
import json, os, sqlite3, sys

def q(value: str) -> str:
    return value.replace("\\", "\\\\").replace('"', '\\"')

env_path, db_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]

def load_env(path):
    env = {}
    if os.path.isfile(path):
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                line = line.strip()
                if not line or line.startswith("#") or "=" not in line:
                    continue
                key, value = line.split("=", 1)
                env[key.strip()] = value.strip()
    return env

env = load_env(env_path)
sqlite = env.get("REDGRES_SQLITE_PATH", db_path)
if not os.path.isfile(sqlite):
    raise SystemExit(0)
try:
    con = sqlite3.connect(f"file:{sqlite}?mode=ro", uri=True)
    row = con.execute("SELECT payload FROM domain_deployment WHERE id = 1").fetchone()
    if not row:
        raise SystemExit(0)
    dep = json.loads(row[0])
except Exception:
    raise SystemExit(0)

zone = (dep.get("zone_name") or "").strip()
console = (dep.get("console_hostname") or dep.get("hostname") or "").strip()
db_host = (dep.get("db_hostname") or "").strip()
rs = (dep.get("rs_hostname") or dep.get("redis_hostname") or "").strip()
pgadmin = (dep.get("pgadmin_hostname") or "").strip()
insight = (dep.get("redis_insight_hostname") or "").strip()
if not console and zone:
    console = f"console.{zone}"
if not db_host and zone:
    db_host = f"db.{zone}"
if not rs and zone:
    rs = f"rs.{zone}"
if not pgadmin and zone:
    pgadmin = f"pgadmin.{zone}"
if not insight and zone:
    insight = f"redis.{zone}"
tunnel_id = (dep.get("tunnel_id") or "").strip()
tunnel_name = (dep.get("tunnel_name") or "").strip()
dns_provider = (dep.get("dns_provider") or "").strip().lower()

lines = [
    f'ZONE="{q(zone)}"',
    f'CONSOLE="{q(console)}"',
    f'DB="{q(db_host)}"',
    f'RS="{q(rs)}"',
    f'PGADMIN="{q(pgadmin)}"',
    f'INSIGHT="{q(insight)}"',
    f'TUNNEL_ID="{q(tunnel_id)}"',
    f'TUNNEL_NAME="{q(tunnel_name)}"',
    f'DNS_PROVIDER="{q(dns_provider)}"',
    "CONFIGURED=1",
]
with open(out_path, "w", encoding="utf-8") as fh:
    fh.write("\n".join(lines) + "\n")
PY
}

write_domain_snapshot

print_cloudflare_followup() {
  local snap="${DOMAIN_SNAPSHOT}" need_manual=1
  [[ "${APP_ONLY}" -eq 1 ]] && return 0
  [[ "${KEEP_REMOTE}" -eq 1 ]] && need_manual=1
  [[ "${CF_API_STATUS}" == "api_ok" ]] && need_manual=0

  if [[ -f "${snap}" ]] && grep -q '^CONFIGURED=1' "${snap}" 2>/dev/null; then
    # shellcheck disable=SC1090
    source "${snap}"
    printf '%b\n' ""
    if [[ "${need_manual}" -eq 0 ]]; then
      printf '%b\n' "  ${BOLD}Cloudflare${NC}  ${GREEN}API cleanup attempted${NC} ${DIM}(zone ${ZONE:-unknown})${NC}"
      printf '%b\n' "  ${DIM}Verify nothing remains — dashboard links below if needed.${NC}"
    else
      printf '%b\n' "  ${YELLOW}${BOLD}Cloudflare — check dashboard and remove anything left:${NC}"
    fi
    printf '%b\n' "    ${BOLD}DNS${NC}       https://dash.cloudflare.com/  →  ${ZONE:-your zone}  →  DNS"
    if [[ -n "${CONSOLE:-}" ]]; then
      printf '%b\n' "              remove: ${CONSOLE}"
      [[ -n "${DB:-}" ]] && printf '%b\n' "                        ${DB}  ${DIM}(grey-cloud A/AAAA)${NC}"
      [[ -n "${RS:-}" ]] && printf '%b\n' "                        ${RS}  ${DIM}(grey-cloud A/AAAA)${NC}"
      [[ -n "${PGADMIN:-}" ]] && printf '%b\n' "                        ${PGADMIN}  ${DIM}(proxied CNAME)${NC}"
      [[ -n "${INSIGHT:-}" ]] && printf '%b\n' "                        ${INSIGHT}  ${DIM}(proxied CNAME)${NC}"
    fi
    printf '%b\n' "    ${BOLD}Access${NC}    https://one.dash.cloudflare.com/access/applications"
    if [[ -n "${CONSOLE:-}" ]]; then
      printf '%b\n' "              delete apps for: ${CONSOLE}"
    else
      printf '%b\n' "              delete console/pgadmin/redis UI apps"
    fi
    [[ -n "${PGADMIN:-}" ]] && printf '%b\n' "                        ${PGADMIN}"
    [[ -n "${INSIGHT:-}" ]] && printf '%b\n' "                        ${INSIGHT}"
    printf '%b\n' "    ${BOLD}Tunnels${NC}   https://one.dash.cloudflare.com/networks/tunnels"
    if [[ -n "${TUNNEL_NAME:-}" ]]; then
      printf '%b\n' "              delete tunnel: ${TUNNEL_NAME}"
    elif [[ -n "${TUNNEL_ID:-}" ]]; then
      printf '%b\n' "              delete tunnel id: ${TUNNEL_ID}"
    else
      printf '%b\n' "              delete Redgres tunnel for this host"
    fi
    if [[ -n "${DB:-}" ]]; then
      printf '%b\n' "    ${BOLD}TLS (host)${NC}  certbot certificates  ${DIM}→ delete ${DB} if still listed${NC}"
    fi
    return 0
  fi

  if [[ "${CF_API_STATUS}" == "no_state" || "${CF_API_STATUS}" == "no_token" || "${KEEP_REMOTE}" -eq 1 ]]; then
    printf '%b\n' ""
    printf '%b\n' "  ${YELLOW}${BOLD}Cloudflare — no local domain state; check dashboard manually:${NC}"
    printf '%b\n' "    DNS      https://dash.cloudflare.com/"
    printf '%b\n' "    Access   https://one.dash.cloudflare.com/access/applications"
    printf '%b\n' "    Tunnels  https://one.dash.cloudflare.com/networks/tunnels"
  fi
}

remote_cloudflare_disconnect() {
  local sqlite env_file cf_out
  [[ "${KEEP_REMOTE}" -eq 1 ]] && {
    CF_API_STATUS="skipped_keep_remote"
    return 0
  }
  env_file="${ETC_ROOT}/redgres.env"
  sqlite="$(env_value_from_file REDGRES_SQLITE_PATH "${env_file}")"
  [[ -n "${sqlite}" ]] || sqlite="${VAR_ROOT}/redgres.db"
  [[ -f "${sqlite}" ]] || {
    CF_API_STATUS="no_state"
    printf '%b\n' "         ${YELLOW}no sqlite state — use Cloudflare dashboard${NC}"
    return 0
  }
  command -v python3 >/dev/null 2>&1 || {
    printf '%b\n' "         ${YELLOW}skipped — python3 required for Cloudflare API${NC}"
    CF_API_STATUS="no_python"
    return 0
  }
  cf_out="$(python3 - "${env_file}" "${sqlite}" <<'PY' 2>&1 || true
import json, os, sqlite3, sys, urllib.error, urllib.request

def load_env(path):
    env = {}
    if not os.path.isfile(path):
        return env
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            env[key.strip()] = value.strip()
    return env

def read_token(env):
    token_path = env.get("REDGRES_CLOUDFLARE_TOKEN_FILE", "/var/lib/redgres/secrets/cloudflare-api-token")
    if token_path and os.path.isfile(token_path):
        token = open(token_path, encoding="utf-8").read().strip()
        if token:
            return token
    oauth_path = env.get("REDGRES_CLOUDFLARE_OAUTH_TOKEN_FILE", "")
    if oauth_path and os.path.isfile(oauth_path):
        try:
            payload = json.load(open(oauth_path, encoding="utf-8"))
            token = (payload.get("access_token") or "").strip()
            if token:
                return token
        except (OSError, json.JSONDecodeError):
            pass
    return ""

def cf_delete(path, token):
    url = "https://api.cloudflare.com/client/v4" + path
    req = urllib.request.Request(
        url,
        method="DELETE",
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status in (200, 404)
    except urllib.error.HTTPError as err:
        if err.code == 404:
            return True
        print(f"         warn: Cloudflare DELETE failed ({err.code})", file=sys.stderr)
        return False

env_path, db_path = sys.argv[1], sys.argv[2]
env = load_env(env_path)
sqlite = env.get("REDGRES_SQLITE_PATH", db_path)
if not os.path.isfile(sqlite):
    print("         no domain state (sqlite missing)")
    print("STATUS:no_state")
    raise SystemExit(0)
con = sqlite3.connect(f"file:{sqlite}?mode=ro", uri=True)
row = con.execute("SELECT payload FROM domain_deployment WHERE id = 1").fetchone()
if not row:
    print("         no domain configured")
    print("STATUS:no_domain")
    raise SystemExit(0)
dep = json.loads(row[0])
if (dep.get("dns_provider") or "").strip().lower() == "manual":
    print("         manual DNS — Cloudflare API skipped")
    print("STATUS:manual_dns")
    raise SystemExit(0)
token = read_token(env)
if not token:
    print("         warn: no Cloudflare token — remove DNS/tunnel/Access in dashboard", file=sys.stderr)
    print("STATUS:no_token")
    raise SystemExit(0)
account = dep.get("account_id") or ""
zone = dep.get("zone_id") or ""
apps = dep.get("access_apps") or []
if not apps and dep.get("access_app_id"):
    apps = [{
        "app_id": dep.get("access_app_id"),
        "policy_id": dep.get("access_policy_id") or "",
    }]
deleted_apps = set()
failed = False
for binding in apps:
    app_id = binding.get("app_id") or ""
    policy_id = binding.get("policy_id") or ""
    if app_id and policy_id and not cf_delete(f"/accounts/{account}/access/apps/{app_id}/policies/{policy_id}", token):
        failed = True
    if app_id and app_id not in deleted_apps:
        if not cf_delete(f"/accounts/{account}/access/apps/{app_id}", token):
            failed = True
        deleted_apps.add(app_id)
for rec in dep.get("records") or []:
    rec_id = rec.get("id") or ""
    if rec_id and zone and not cf_delete(f"/zones/{zone}/dns_records/{rec_id}", token):
        failed = True
tunnel_id = dep.get("tunnel_id") or ""
if tunnel_id and account and not cf_delete(f"/accounts/{account}/cfd_tunnel/{tunnel_id}", token):
    failed = True
print("         Cloudflare disconnect attempted (DNS records, tunnel, Access)")
print("STATUS:api_partial" if failed else "STATUS:api_ok")
PY
)"
  printf '%s\n' "${cf_out}" | grep -v '^STATUS:' || true
  CF_API_STATUS="$(printf '%s\n' "${cf_out}" | grep '^STATUS:' | tail -n1 | cut -d: -f2-)"
  [[ -n "${CF_API_STATUS}" ]] || CF_API_STATUS="unknown"
}

purge_tls_certs() {
  local sqlite certbot_bin primary
  [[ "${KEEP_REMOTE}" -eq 1 ]] && return 0
  sqlite="$(env_value_from_file REDGRES_SQLITE_PATH)"
  [[ -n "${sqlite}" ]] || sqlite="${VAR_ROOT}/redgres.db"
  [[ -f "${sqlite}" ]] || return 0
  certbot_bin="$(env_value_from_file REDGRES_CERTBOT_BIN)"
  [[ -n "${certbot_bin}" ]] || certbot_bin="certbot"
  command -v "${certbot_bin}" >/dev/null 2>&1 || return 0
  primary="$(python3 - "${sqlite}" <<'PY' || true
import json, sqlite3, sys
db = sys.argv[1]
try:
    con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    row = con.execute("SELECT payload FROM domain_deployment WHERE id = 1").fetchone()
    if not row:
        raise SystemExit(0)
    dep = json.loads(row[0])
except Exception:
    raise SystemExit(0)
db_host = (dep.get("db_hostname") or "").strip()
if not db_host:
    zone = (dep.get("zone_name") or "").strip()
    if zone:
        db_host = f"db.{zone}"
if db_host:
    print(db_host)
PY
)"
  [[ -n "${primary}" ]] || return 0
  "${certbot_bin}" delete --non-interactive --cert-name "${primary}" 2>/dev/null || true
  printf '%b\n' "         certbot delete attempted for ${primary}"
}

purge_cloudflared_package() {
  [[ "${APP_ONLY}" -eq 1 ]] && return 0
  if command -v apt-get >/dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get purge -y cloudflared 2>/dev/null || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf remove -y cloudflared 2>/dev/null || true
  elif command -v yum >/dev/null 2>&1; then
    yum remove -y cloudflared 2>/dev/null || true
  fi
}

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

# ── 0. Cloudflare + TLS (before local state is deleted) ─────────────────────
if [[ "${APP_ONLY}" -eq 0 && "${KEEP_REMOTE}" -eq 0 ]]; then
  step "  ${CYAN}[0/8]${NC} Cloudflare + TLS cleanup... "
  remote_cloudflare_disconnect
  purge_tls_certs
  step_done
elif [[ "${APP_ONLY}" -eq 0 && "${KEEP_REMOTE}" -eq 1 ]]; then
  step "  ${CYAN}[0/8]${NC} Cloudflare + TLS "
  step_skip
fi

# ── 1. Stop Redgres + tunnel units ───────────────────────────────────────────
if command -v systemctl >/dev/null 2>&1; then
  step "  ${CYAN}[1/8]${NC} Stopping Redgres and tunnel services... "
  stop_systemd_unit redgres.service
  stop_systemd_unit cloudflared-redgres.service
  stop_systemd_unit cloudflared-redgres.path
  stop_systemd_unit cloudflared-redgres-restart.service
  step_done
else
  step "  ${CYAN}[1/8]${NC} systemd not found "
  step_skip
fi

# ── 2. Bootstrap firewall ────────────────────────────────────────────────────
step "  ${CYAN}[2/8]${NC} Removing bootstrap firewall rule (8989)... "
remove_bootstrap_firewall
step_done

# ── 3. Docker workloads ──────────────────────────────────────────────────────
if [[ "${APP_ONLY}" -eq 0 ]]; then
  step "  ${CYAN}[3/8]${NC} Removing Redgres Docker containers... "
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
  step "  ${CYAN}[3/8]${NC} Docker cleanup "
  step_skip
fi

# ── 4. PostgreSQL / Redis / PgBouncer ───────────────────────────────────────
if [[ "${APP_ONLY}" -eq 0 ]]; then
  step "  ${CYAN}[4/8]${NC} Removing PostgreSQL clusters and packages... "
  purge_postgresql
  step_done

  step "  ${CYAN}[5/8]${NC} Removing Redis, PgBouncer, and cloudflared... "
  purge_redis_native
  purge_pgbouncer
  purge_cloudflared_package
  step_done
else
  step "  ${CYAN}[4/8]${NC} PostgreSQL/Redis/PgBouncer "
  step_skip
  step "  ${CYAN}[5/8]${NC} (skipped — --app-only) "
  step_skip
fi

# ── 6. Systemd units + libexec ───────────────────────────────────────────────
step "  ${CYAN}[6/8]${NC} Removing systemd units and helpers... "
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
step "  ${CYAN}[7/8]${NC} Removing Redgres files... "
rm -f \
  "${VAR_ROOT}/secrets/cloudflare-api-token" \
  "${VAR_ROOT}/secrets/cloudflared-tunnel-token" \
  "${VAR_ROOT}/secrets/certbot-dns.ini" \
  "${VAR_ROOT}/secrets/cloudflare-oauth-client.json" \
  "${VAR_ROOT}/secrets/cloudflare-oauth-token.json" \
  "${ETC_ROOT}/secrets/cloudflare-oauth-token" \
  "${ETC_ROOT}/secrets/certbot-dns-token" \
  2>/dev/null || true
rmdir "${VAR_ROOT}/secrets" "${ETC_ROOT}/secrets" 2>/dev/null || true
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
  print_cloudflare_followup
  printf '%b\n' "  ${DIM}Reinstall: curl -fsSL .../install-dev.sh | sudo bash${NC}"
else
  printf '%b\n' "  ${DIM}PostgreSQL and Redis were not removed (--app-only).${NC}"
fi
printf '%b\n' ""
