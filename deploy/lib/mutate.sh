#!/usr/bin/env bash
# Live fresh-install mutation (OPS-001 Partial). Existing modes stay exit 2.
# Ubuntu 24.04 (noble) and 26.04 (resolute) use the same PGDG + Ubuntu package names.
# Does not source operator --config. Does not print secrets. Does not apt-upgrade the system.
set -euo pipefail

redgres_require_root() {
  [[ "${EUID}" -eq 0 ]] || redgres_die 'live install requires root'
}

# PGDG current Ubuntu set (wiki.postgresql.org/wiki/Apt): noble (24.04), resolute (26.04).
# Interim 24.10/25.x were removed from apt.postgresql.org.
redgres_assert_pgdg_ubuntu() {
  local os_id="$1"
  local os_version_id="$2"
  local os_codename="$3"
  [[ "${os_id}" == "ubuntu" ]] || redgres_die "unsupported OS ${os_id:-unknown}"
  [[ "${os_codename}" =~ ^[a-z]+$ ]] || redgres_die 'os-release VERSION_CODENAME is invalid'
  case "${os_codename}" in
    noble|resolute) ;;
    *)
      redgres_die "Ubuntu ${os_version_id:-unknown} (${os_codename}) is not a PGDG Ubuntu release (24.04 noble / 26.04 resolute)"
      ;;
  esac
}

redgres_resolve_os_release() {
  local etc="${1:-/etc/os-release}"
  local usr="${2:-/usr/lib/os-release}"
  if [[ -f "${etc}" && ! -L "${etc}" ]]; then
    printf '%s' "${etc}"
    return 0
  fi
  if [[ -L "${etc}" || ! -e "${etc}" ]]; then
    if [[ -f "${usr}" && ! -L "${usr}" ]]; then
      printf '%s' "${usr}"
      return 0
    fi
  fi
  return 1
}

redgres_read_os() {
  local os_file
  os_file="$(redgres_resolve_os_release)" || redgres_die 'os-release is not trusted'
  redgres_validate_trusted_path 'os-release' "${os_file}" file
  # shellcheck disable=SC1090
  . "${os_file}"
  redgres_assert_pgdg_ubuntu "${ID:-}" "${VERSION_ID:-}" "${VERSION_CODENAME:-}"
  REDGRES_OS_ID="${ID}"
  REDGRES_OS_VERSION_ID="${VERSION_ID}"
  REDGRES_OS_CODENAME="${VERSION_CODENAME}"
}

redgres_require_amd64() {
  local arch
  local dpkg_bin='/usr/bin/dpkg'
  [[ -x "${dpkg_bin}" && -f "${dpkg_bin}" && ! -L "${dpkg_bin}" ]] || redgres_die 'dpkg is unavailable'
  arch="$("${dpkg_bin}" --print-architecture)"
  [[ "${arch}" == "amd64" ]] || redgres_die "unsupported architecture ${arch}"
  REDGRES_OS_ARCH="${arch}"
}

redgres_find_ss() {
  local candidate
  for candidate in /usr/bin/ss /usr/sbin/ss /bin/ss; do
    [[ -x "${candidate}" && -f "${candidate}" && ! -L "${candidate}" ]] || continue
    printf '%s' "${candidate}"
    return 0
  done
  return 1
}

redgres_port_free() {
  local port="$1"
  local ss_bin listeners
  ss_bin="$(redgres_find_ss)" || redgres_die 'ss is unavailable'
  listeners="$("${ss_bin}" -H -lnt)" || redgres_die 'ss failed'
  if printf '%s\n' "${listeners}" | /usr/bin/awk -v port="${port}" 'BEGIN { found=0 } $4 ~ (":" port "$") { found=1 } END { exit found ? 0 : 1 }'; then
    redgres_die "port ${port} is already in use"
  fi
}

redgres_live_preflight_ports() {
  redgres_port_free 5432
  redgres_port_free 6380
  if [[ "${pgbouncer_mode}" == "fresh" ]]; then
    redgres_port_free 6432
  fi
}

redgres_pgbouncer_ini() {
  local userlist="${1:-${REDGRES_PGBOUNCER_USERLIST:-/etc/pgbouncer/userlist.txt}}"
  cat <<EOF
[databases]
* = host=127.0.0.1 port=5432

[pgbouncer]
logfile = /var/log/postgresql/pgbouncer.log
pidfile = /var/run/postgresql/pgbouncer.pid
unix_socket_dir = /var/run/postgresql
listen_addr = 127.0.0.1
listen_port = 6432
auth_type = scram-sha-256
auth_file = ${userlist}
auth_user = redgres_admin
auth_dbname = postgres
auth_query = SELECT usename, passwd FROM pgbouncer.user_search(\$1)
admin_users = redgres_admin
pool_mode = transaction
max_client_conn = 200
default_pool_size = 20
ignore_startup_parameters = extra_float_digits
server_tls_sslmode = require
client_tls_sslmode = require
client_tls_cert_file = /etc/ssl/certs/ssl-cert-snakeoil.pem
client_tls_key_file = /etc/ssl/private/ssl-cert-snakeoil.key
EOF
}

redgres_pgbouncer_userlist_line() {
  local user="$1" pass="$2"
  pass="${pass//\\/\\\\}"
  pass="${pass//\"/\\\"}"
  printf '"%s" "%s"\n' "${user}" "${pass}"
}

redgres_pgbouncer_auth_sql() {
  cat <<'EOSQL'
CREATE SCHEMA IF NOT EXISTS pgbouncer;
REVOKE ALL ON SCHEMA pgbouncer FROM PUBLIC;
GRANT USAGE ON SCHEMA pgbouncer TO redgres_admin;
CREATE OR REPLACE FUNCTION pgbouncer.user_search(uname text)
RETURNS TABLE(usename name, passwd text)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
  SELECT usename, passwd FROM pg_catalog.pg_shadow WHERE usename = uname;
$$;
REVOKE ALL ON FUNCTION pgbouncer.user_search(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION pgbouncer.user_search(text) TO redgres_admin;
EOSQL
}

redgres_apply_pgbouncer_auth_sql() {
  local log
  log="$(/usr/bin/mktemp /tmp/redgres-cmd.XXXXXX)" || return 1
  # shellcheck disable=SC2064
  trap "rm -f $(printf '%q' "${log}")" RETURN
  if /usr/sbin/runuser -u postgres -- /usr/bin/psql -q -d postgres -v ON_ERROR_STOP=1 >"${log}" 2>&1 <<EOSQL
$(redgres_pgbouncer_auth_sql)
EOSQL
  then
    /usr/bin/rm -f "${log}"
    return 0
  fi
  redgres_cmd_log_safe "$(/usr/bin/tail -n 50 "${log}")" >&2
  /usr/bin/rm -f "${log}"
  return 1
}

redgres_pgbouncer_health() {
  local i=0
  while ! /usr/bin/pg_isready -h 127.0.0.1 -p 6432 >/dev/null 2>&1; do
    i=$((i + 1))
    (( i < 30 )) || redgres_die 'pgbouncer did not become ready on 127.0.0.1:6432'
    /usr/bin/sleep 1
  done
}

redgres_configure_fresh_pgbouncer() {
  local ini userlist passfile pass
  [[ "${pgbouncer_mode}" == "fresh" ]] || return 0
  ini="${REDGRES_PGBOUNCER_INI:-/etc/pgbouncer/pgbouncer.ini}"
  userlist="${REDGRES_PGBOUNCER_USERLIST:-/etc/pgbouncer/userlist.txt}"
  passfile="${REDGRES_POSTGRES_PASSFILE:-/etc/redgres/postgres.pass}"
  [[ -f "${passfile}" && ! -L "${passfile}" ]] || redgres_die 'postgres passfile is not trusted'
  pass="$(/usr/bin/cat "${passfile}")"
  [[ -n "${pass}" ]] || redgres_die 'postgres passfile is empty'
  if [[ "${REDGRES_PGBOUNCER_SKIP_HOST:-}" != "1" ]]; then
    redgres_apply_pgbouncer_auth_sql
  fi
  /usr/bin/mkdir -p "$(/usr/bin/dirname "${ini}")" "$(/usr/bin/dirname "${userlist}")"
  umask 077
  redgres_pgbouncer_ini "${userlist}" >"${ini}"
  /usr/bin/chmod 644 "${ini}"
  redgres_pgbouncer_userlist_line redgres_admin "${pass}" >"${userlist}"
  /usr/bin/chmod 600 "${userlist}"
  if [[ "${REDGRES_PGBOUNCER_SKIP_HOST:-}" != "1" ]]; then
    if /usr/bin/getent passwd postgres >/dev/null; then
      /usr/bin/chown postgres:postgres "${ini}" "${userlist}"
    fi
    /usr/sbin/usermod -aG ssl-cert postgres
    if /usr/bin/getent passwd pgbouncer >/dev/null; then
      /usr/sbin/usermod -aG ssl-cert pgbouncer
    fi
    redgres_run_quiet 'pgbouncer enable' /usr/bin/systemctl enable pgbouncer || redgres_die 'pgbouncer enable failed'
    redgres_run_quiet 'pgbouncer restart' /usr/bin/systemctl restart pgbouncer || redgres_die 'pgbouncer restart failed'
    redgres_pgbouncer_health
  fi
  redgres_pgbouncer_listen=1
  redgres_log 'pgbouncer listening on 127.0.0.1:6432 (password not logged)'
}

redgres_refuse_existing_postgres_datadir() {
  local datadir="/var/lib/postgresql/${postgres_version}/main"
  if [[ -e "${datadir}" ]]; then
    redgres_die "refusing fresh-postgres: ${datadir} already exists"
  fi
}

# Capture apt-get. Success is quiet; failure dumps a secret-safe tail.
# NEEDRESTART_SUSPEND skips the post-install process scan that floods SSH.
redgres_apt_handle_log() {
  local log="$1" rc="$2"
  if [[ "${rc}" -ne 0 ]]; then
    redgres_log 'apt-get failed'
    redgres_cmd_log_safe "$(/usr/bin/tail -n 50 "${log}")" >&2
    return 1
  fi
  if /usr/bin/grep -q 'NO_PUBKEY' "${log}"; then
    redgres_log 'apt update: using cached PGDG index (signature key missing)'
  fi
  return 0
}

redgres_apt_bin() {
  printf '%s' "${REDGRES_APT_GET:-/usr/bin/apt-get}"
}

redgres_apt_get() {
  local log rc=0 apt
  apt="$(redgres_apt_bin)"
  if [[ "${REDGRES_INSTALL_VERBOSE:-}" == '1' ]]; then
    NEEDRESTART_SUSPEND=1 NEEDRESTART_MODE=l DEBIAN_FRONTEND=noninteractive APT_LISTCHANGES_FRONTEND=none \
      redgres_run_filtered "${apt}" -o Dpkg::Use-Pty=0 "$@" </dev/null
    return $?
  fi
  log="$(/usr/bin/mktemp /tmp/redgres-cmd.XXXXXX)" || return 1
  # shellcheck disable=SC2064
  trap "rm -f $(printf '%q' "${log}")" RETURN
  if ! NEEDRESTART_SUSPEND=1 NEEDRESTART_MODE=l DEBIAN_FRONTEND=noninteractive APT_LISTCHANGES_FRONTEND=none \
    "${apt}" -o Dpkg::Use-Pty=0 "$@" </dev/null >"${log}" 2>&1; then
    rc=$?
  fi
  if ! redgres_apt_handle_log "${log}" "${rc}"; then
    /usr/bin/rm -f "${log}"
    return 1
  fi
  /usr/bin/rm -f "${log}"
}

# Parse `apt-cache policy` stdout. Candidate is pinned before apt-get install pkg=ver.
redgres_apt_candidate_from_policy() {
  local policy="$1"
  local candidate
  candidate="$(printf '%s\n' "${policy}" | /usr/bin/awk '/^[[:space:]]*Candidate:[[:space:]]+/ { print $2; exit }')"
  [[ -n "${candidate}" ]] || return 1
  [[ "${candidate}" != '(none)' ]] || return 1
  [[ "${candidate}" =~ ^[0-9][A-Za-z0-9.+:~-]*$ ]] || return 1
  printf '%s\n' "${candidate}"
}

redgres_apt_candidate() {
  local pkg="$1" policy
  [[ "${pkg}" =~ ^[a-z0-9][a-z0-9+.-]*$ ]] || return 1
  policy="$(/usr/bin/apt-cache policy "${pkg}")" || return 1
  redgres_apt_candidate_from_policy "${policy}"
}

redgres_apt_install() {
  local pkg="$1" status ver candidate
  [[ "${pkg}" =~ ^[a-z0-9][a-z0-9+.-]*$ ]] || redgres_die "invalid package name ${pkg}"
  status="$(/usr/bin/dpkg-query -W -f '${Status}' "${pkg}" 2>/dev/null || true)"
  if [[ "${status}" == *"install ok installed"* ]]; then
    ver="$(/usr/bin/dpkg-query -W -f '${Version}' "${pkg}")"
    redgres_log "package already-correct ${pkg}=${ver}"
    return 0
  fi
  candidate="$(redgres_apt_candidate "${pkg}")" || return 1
  redgres_log "pinning ${pkg}=${candidate}"
  redgres_apt_get install -y --no-install-recommends "${pkg}=${candidate}" || return 1
  ver="$(/usr/bin/dpkg-query -W -f '${Version}' "${pkg}")"
  [[ "${ver}" == "${candidate}" ]] || return 1
  redgres_log "installed ${pkg}=${ver}"
}

redgres_assert_redis_pong() {
  local out="$1"
  printf '%s\n' "${out}" | /usr/bin/awk 'BEGIN { found=0 } $0=="PONG" { found=1 } END { exit found ? 0 : 1 }'
}

# Official redis image logs/config must never print requirepass. Keep installer stderr secret-safe.
redgres_redis_logs_safe() {
  redgres_cmd_log_safe "$1"
}

redgres_redis_container_status() {
  /usr/bin/docker inspect -f 'status={{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}}' redgres-redis 2>/dev/null || printf 'status=missing'
}

redgres_redis_health_report() {
  local status logs
  status="$(redgres_redis_container_status)"
  redgres_log "redis health failed (${status})"
  logs="$(/usr/bin/docker logs --tail 20 redgres-redis 2>&1 || true)"
  redgres_redis_logs_safe "${logs}" >&2
}

redgres_postgres_health() {
  local i=0
  while ! /usr/bin/pg_isready -q -h 127.0.0.1 -p 5432; do
    i=$((i + 1))
    (( i < 30 )) || redgres_die 'postgres did not become ready'
    /usr/bin/sleep 1
  done
  redgres_log 'postgres health=accepting'
}

redgres_wait_redis_container() {
  local i=0 status
  while :; do
    status="$(/usr/bin/docker inspect -f '{{.State.Status}}' redgres-redis 2>/dev/null || true)"
    if [[ "${status}" == "running" ]]; then
      redgres_log 'redis container=running'
      return 0
    fi
    i=$((i + 1))
    if (( i >= 45 )); then
      redgres_redis_health_report
      redgres_die "redis container is not running (${status:-missing})"
    fi
    /usr/bin/sleep 1
  done
}

redgres_redis_health() {
  local pass out i=0
  [[ -f /etc/redgres/redis.pass && ! -L /etc/redgres/redis.pass ]] || redgres_die 'redis passfile is not trusted'
  pass="$(/usr/bin/cat /etc/redgres/redis.pass)"
  [[ -n "${pass}" ]] || redgres_die 'redis passfile is empty'
  while :; do
    out="$(
      /usr/bin/docker exec -i redgres-redis redis-cli -h 127.0.0.1 --raw --no-auth-warning 2>/dev/null <<EOF
AUTH "${pass}"
PING
EOF
    )" || true
    if redgres_assert_redis_pong "${out}"; then
      redgres_log 'redis health=PONG'
      return 0
    fi
    i=$((i + 1))
    if (( i >= 30 )); then
      redgres_redis_health_report
      redgres_die 'redis did not become ready'
    fi
    /usr/bin/sleep 1
  done
}

redgres_disable_auto_cluster() {
  local conf='/etc/postgresql-common/createcluster.conf'
  /usr/bin/mkdir -p /etc/postgresql-common
  if [[ -f "${conf}" ]]; then
    if /usr/bin/grep -qE '^[[:space:]]*create_main_cluster[[:space:]]*=' "${conf}"; then
      /usr/bin/sed -i 's/^[[:space:]]*create_main_cluster[[:space:]]*=.*/create_main_cluster = false/' "${conf}"
    else
      printf '\n%s\n' 'create_main_cluster = false' >>"${conf}"
    fi
  else
    printf '%s\n' 'create_main_cluster = false' >"${conf}"
  fi
  /usr/bin/chmod 644 "${conf}"
}

redgres_enable_pgdg() {
  local key='/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc'
  local src='/etc/apt/sources.list.d/pgdg.sources'
  local expected
  [[ -f "${key}" && ! -L "${key}" ]] || redgres_die 'postgresql-common did not provide the PGDG signing key'
  expected="$(cat <<EOF
Types: deb
URIs: https://apt.postgresql.org/pub/repos/apt
Suites: ${REDGRES_OS_CODENAME}-pgdg
Architectures: ${REDGRES_OS_ARCH}
Components: main
Signed-By: ${key}
EOF
)"
  if [[ -f "${src}" ]] && [[ "$(/usr/bin/cat "${src}")" == "${expected}" ]]; then
    redgres_log 'pgdg.sources already-correct'
    return 0
  fi
  printf '%s\n' "${expected}" >"${src}"
  /usr/bin/chmod 644 "${src}"
  redgres_log "enabled PGDG suite ${REDGRES_OS_CODENAME}-pgdg"
}

redgres_ensure_identity() {
  /usr/bin/getent group redgres >/dev/null || /usr/sbin/groupadd --system redgres
  /usr/bin/getent passwd redgres >/dev/null || /usr/sbin/useradd --system --gid redgres --home-dir /var/lib/redgres --shell /usr/sbin/nologin redgres
  /usr/bin/mkdir -p /var/lib/redgres /var/lib/redgres/secrets /var/lib/redgres/redis/data /etc/redgres
  /usr/bin/chown redgres:redgres /var/lib/redgres/secrets
  /usr/bin/chmod 700 /var/lib/redgres/secrets
  /usr/bin/chown redgres:redgres /var/lib/redgres
  /usr/bin/chmod 750 /var/lib/redgres
  /usr/bin/chown root:root /var/lib/redgres/redis
  /usr/bin/chmod 755 /var/lib/redgres/redis
  # Official redis image runs as UID 999; do not chown Redis data to redgres.
  /usr/bin/chown 999:999 /var/lib/redgres/redis/data
  /usr/bin/chmod 750 /var/lib/redgres/redis/data
  /usr/bin/chown root:redgres /etc/redgres
  /usr/bin/chmod 750 /etc/redgres
}

# Host redis.conf is mounted :ro into the official image (UID 999). Mode 0600
# root:root is unreadable in the container; the entrypoint then fails to chmod
# a read-only mount. Own the conf as 999:999 and skip entrypoint perm fixes.
redgres_apply_redis_conf_perms() {
  local conf='/etc/redgres/redis.conf' passfile='/etc/redgres/redis.pass'
  [[ -f "${conf}" && ! -L "${conf}" ]] || redgres_die 'redis conf is not trusted'
  [[ -f "${passfile}" && ! -L "${passfile}" ]] || redgres_die 'redis passfile is not trusted'
  /usr/bin/chown 999:999 "${conf}"
  /usr/bin/chmod 600 "${conf}"
  /usr/bin/chown root:root "${passfile}"
  /usr/bin/chmod 600 "${passfile}"
}

redgres_redis_lowmem_conf() {
  local kb
  kb="$(/usr/bin/awk '/^MemTotal:/ { print $2 }' /proc/meminfo)"
  [[ -n "${kb}" ]] || return 0
  if (( kb < 2000000 )); then
    printf '%s\n' 'maxmemory 64mb' 'maxmemory-policy allkeys-lru'
  fi
}

redgres_write_redis_conf() {
  local conf='/etc/redgres/redis.conf' passfile='/etc/redgres/redis.pass' pass extra
  if [[ -f "${conf}" && -f "${passfile}" ]]; then
    redgres_apply_redis_conf_perms
    redgres_log 'redis auth files already-correct'
    return 0
  fi
  if [[ -e "${conf}" || -e "${passfile}" ]]; then
    redgres_die 'redis auth files are inconsistent'
  fi
  pass="$(/usr/bin/dd if=/dev/urandom bs=24 count=1 status=none | /usr/bin/base64 | /usr/bin/tr -d '\n')"
  extra="$(redgres_redis_lowmem_conf)"
  umask 077
  printf '%s\n' "${pass}" >"${passfile}"
  /usr/bin/cat >"${conf}" <<EOF
bind 0.0.0.0
port 6379
protected-mode yes
appendonly yes
dir /data
requirepass "${pass}"
${extra}
EOF
  redgres_apply_redis_conf_perms
  redgres_log 'redis password written to /etc/redgres/redis.pass (not logged)'
}

redgres_redis_compose_yaml() {
  local image="$1"
  /usr/bin/cat <<EOF
services:
  redis:
    image: ${image}
    container_name: redgres-redis
    restart: unless-stopped
    environment:
      SKIP_FIX_PERMS: "1"
    command: ["redis-server", "/usr/local/etc/redis/redis.conf"]
    ports:
      - "127.0.0.1:6380:6379"
    volumes:
      - /var/lib/redgres/redis/data:/data
      - /etc/redgres/redis.conf:/usr/local/etc/redis/redis.conf:ro
EOF
}

redgres_write_redis_compose() {
  local image="$1"
  redgres_redis_compose_yaml "${image}" >'/etc/redgres/redis-compose.yml'
  /usr/bin/chmod 644 /etc/redgres/redis-compose.yml
}

redgres_wait_docker() {
  local i=0
  while ! /usr/bin/docker info >/dev/null 2>&1; do
    i=$((i + 1))
    (( i < 30 )) || redgres_die 'docker did not become ready'
    /usr/bin/sleep 1
  done
}

redgres_low_memory_postgres() {
  local kb confdir snippet
  kb="$(/usr/bin/awk '/^MemTotal:/ { print $2 }' /proc/meminfo)"
  [[ -n "${kb}" ]] || return 0
  if (( kb < 2000000 )); then
    confdir="/etc/postgresql/${postgres_version}/main/conf.d"
    snippet="${confdir}/redgres-lowmem.conf"
    /usr/bin/mkdir -p "${confdir}"
    printf '%s\n' 'shared_buffers = 32MB' 'work_mem = 1MB' >"${snippet}"
    /usr/bin/chmod 644 "${snippet}"
    redgres_log 'applied low-memory PostgreSQL snippet (host has under 2GiB RAM)'
  fi
}

redgres_record_resolved_packages() {
  local pkg
  for pkg in "postgresql-${postgres_version}" "postgresql-client-${postgres_version}" docker.io docker-compose-v2; do
    redgres_log "resolved ${pkg}=$(/usr/bin/dpkg-query -W -f '${Version}' "${pkg}")"
  done
  if [[ "${pgbouncer_mode}" == "fresh" ]]; then
    redgres_log "resolved pgbouncer=$(/usr/bin/dpkg-query -W -f '${Version}' pgbouncer)"
  fi
}

redgres_redis_url_encode() {
  printf '%s' "$1" | /usr/bin/sed 's/%/%25/g; s/+/%2B/g; s/\//%2F/g; s/=/%3D/g'
}

redgres_write_redis_admin_url() {
  local passfile='/etc/redgres/redis.pass' urlfile='/etc/redgres/redis.url' pass encoded
  [[ -f "${passfile}" && ! -L "${passfile}" ]] || redgres_die 'redis passfile is not trusted'
  pass="$(/usr/bin/cat "${passfile}")"
  [[ -n "${pass}" ]] || redgres_die 'redis passfile is empty'
  encoded="$(redgres_redis_url_encode "${pass}")"
  umask 077
  printf 'redis://default:%s@127.0.0.1:6380/0\n' "${encoded}" >"${urlfile}"
  /usr/bin/chown redgres:redgres "${urlfile}"
  /usr/bin/chmod 600 "${urlfile}"
}

redgres_enable_postgres_loopback_ssl() {
  local confdir="/etc/postgresql/${postgres_version}/main/conf.d"
  if [[ ! -f /etc/ssl/certs/ssl-cert-snakeoil.pem || ! -f /etc/ssl/private/ssl-cert-snakeoil.key ]]; then
    redgres_apt_install ssl-cert || redgres_die 'apt-get failed'
    /usr/sbin/make-ssl-cert generate-default-snakeoil --force-overwrite >/dev/null
  fi
  /usr/sbin/usermod -aG ssl-cert postgres
  /usr/bin/mkdir -p "${confdir}"
  printf '%s\n' "listen_addresses = '127.0.0.1'" >"${confdir}/redgres-listen.conf"
  printf '%s\n' "ssl = on" "ssl_cert_file = '/etc/ssl/certs/ssl-cert-snakeoil.pem'" "ssl_key_file = '/etc/ssl/private/ssl-cert-snakeoil.key'" >"${confdir}/redgres-ssl.conf"
  /usr/bin/chmod 644 "${confdir}/redgres-listen.conf" "${confdir}/redgres-ssl.conf"
  redgres_run_quiet 'postgres restart' /usr/bin/pg_ctlcluster "${postgres_version}" main restart || redgres_die 'postgres restart failed'
  redgres_log 'postgres loopback TLS enabled (snakeoil; sslmode=require)'
}

redgres_create_postgres_admin() {
  local passfile='/etc/redgres/postgres.pass' pass
  if [[ ! -f "${passfile}" ]]; then
    pass="$(/usr/bin/dd if=/dev/urandom bs=24 count=1 status=none | /usr/bin/base64 | /usr/bin/tr -d '\n')"
    umask 077
    printf '%s\n' "${pass}" >"${passfile}"
  fi
  [[ -f "${passfile}" && ! -L "${passfile}" ]] || redgres_die 'postgres passfile is not trusted'
  /usr/bin/chown redgres:redgres "${passfile}"
  /usr/bin/chmod 600 "${passfile}"
  pass="$(/usr/bin/cat "${passfile}")"
  [[ -n "${pass}" ]] || redgres_die 'postgres passfile is empty'
  local log
  log="$(/usr/bin/mktemp /tmp/redgres-cmd.XXXXXX)" || redgres_die 'postgres admin role failed'
  # shellcheck disable=SC2064
  trap "rm -f $(printf '%q' "${log}")" RETURN
  if /usr/sbin/runuser -u postgres -- /usr/bin/psql -q -d postgres -v ON_ERROR_STOP=1 >"${log}" 2>&1 <<EOSQL
DO \$\$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'redgres_admin') THEN
    ALTER ROLE redgres_admin WITH LOGIN PASSWORD \$redgres\$${pass}\$redgres\$;
  ELSE
    CREATE ROLE redgres_admin LOGIN PASSWORD \$redgres\$${pass}\$redgres\$;
  END IF;
END
\$\$;
GRANT CONNECT ON DATABASE postgres TO redgres_admin;
EOSQL
  then
    /usr/bin/rm -f "${log}"
    redgres_log 'postgres admin role redgres_admin ready (password not logged)'
    return 0
  fi
  redgres_cmd_log_safe "$(/usr/bin/tail -n 50 "${log}")" >&2
  /usr/bin/rm -f "${log}"
  redgres_die 'postgres admin role failed'
}

redgres_live_install() {
  local pg_pkg
  [[ "${mode}" == "fresh-postgres" ]] || redgres_not_implemented 'live existing-postgres is not implemented'
  [[ "${redis_mode}" == "fresh" ]] || redgres_not_implemented 'live existing redis is not implemented'
  case "${pgbouncer_mode}" in
    disabled|fresh) ;;
    *) redgres_not_implemented 'live existing pgbouncer is not implemented' ;;
  esac

  redgres_require_root
  redgres_read_os
  redgres_require_amd64
  redgres_refuse_existing_postgres_datadir
  redgres_live_preflight_ports

  pg_pkg="postgresql-${postgres_version}"
  if redgres_color_ok; then
    printf '\n  \033[0;36m\033[1mRedgres install\033[0m\n'
    printf '  \033[2m%s %s (%s)  postgres=%s  redis=%s  pgbouncer=%s\033[0m\n' \
      "${REDGRES_OS_ID}" "${REDGRES_OS_VERSION_ID}" "${REDGRES_OS_CODENAME}" \
      "${postgres_version}" "${redis_version}" "${pgbouncer_mode}"
  else
    printf '\n  Redgres install\n'
    printf '  %s %s (%s)  postgres=%s  redis=%s  pgbouncer=%s\n' \
      "${REDGRES_OS_ID}" "${REDGRES_OS_VERSION_ID}" "${REDGRES_OS_CODENAME}" \
      "${postgres_version}" "${redis_version}" "${pgbouncer_mode}"
  fi
  REDGRES_LOG_PREFIX='    '

  redgres_section 1 8 'Preflight'
  redgres_log "os=${REDGRES_OS_ID} ${REDGRES_OS_VERSION_ID} (${REDGRES_OS_CODENAME}) postgres=${postgres_version}"

  redgres_section 2 8 'Packages'
  trap 'rm -f /tmp/redgres-cmd.*' EXIT
  trap 'rm -f /tmp/redgres-cmd.*; exit 130' INT
  trap 'rm -f /tmp/redgres-cmd.*; exit 143' TERM
  redgres_apt_get update || redgres_die 'apt-get update failed'
  redgres_apt_install ca-certificates || redgres_die 'apt-get failed'
  redgres_apt_install postgresql-common || redgres_die 'apt-get failed'
  redgres_disable_auto_cluster
  redgres_enable_pgdg
  redgres_apt_get update || redgres_die 'apt-get update failed'
  redgres_apt_install "postgresql-client-${postgres_version}" || redgres_die 'apt-get failed'
  redgres_apt_install "${pg_pkg}" || redgres_die 'apt-get failed'
  redgres_apt_install docker.io || redgres_die 'apt-get failed'
  redgres_apt_install docker-compose-v2 || redgres_die 'apt-get failed'
  if [[ "${pgbouncer_mode}" == "fresh" ]]; then
    redgres_apt_install pgbouncer || redgres_die 'apt-get failed'
  fi
  redgres_ensure_identity

  redgres_section 3 8 'Redis'
  redgres_write_redis_conf
  redgres_write_redis_compose "$(redgres_redis_image_pin "${redis_version}")"
  redgres_run_quiet 'docker enable' /usr/bin/systemctl enable --now docker || redgres_die 'docker enable failed'
  redgres_wait_docker
  redgres_run_quiet 'redis compose' /usr/bin/docker compose -f /etc/redgres/redis-compose.yml up -d || redgres_die 'redis compose failed'
  redgres_wait_redis_container

  redgres_section 4 8 'PostgreSQL'
  if /usr/bin/pg_lsclusters -h | /usr/bin/awk -v v="${postgres_version}" '$1==v { found=1 } END { exit found ? 0 : 1 }'; then
    redgres_die "refusing fresh-postgres: a PostgreSQL ${postgres_version} cluster already exists"
  fi
  redgres_run_quiet 'PostgreSQL cluster' /usr/bin/pg_createcluster --start "${postgres_version}" main || redgres_die 'PostgreSQL cluster create failed'
  redgres_low_memory_postgres
  redgres_enable_postgres_loopback_ssl
  redgres_postgres_health
  redgres_redis_health
  redgres_create_postgres_admin

  redgres_section 5 8 'PgBouncer'
  if [[ "${pgbouncer_mode}" == "fresh" ]]; then
    redgres_configure_fresh_pgbouncer
  else
    redgres_log 'skipped (disabled)'
  fi
  redgres_write_redis_admin_url
  redgres_write_default_env

  redgres_section 6 8 'Firewall'
  redgres_install_bootstrap_firewall

  redgres_section 7 8 'Domain runtime'
  redgres_install_domain_runtime || redgres_log 'domain runtime: units/packages not fully applied'

  redgres_section 8 8 'Application'
  release_path="$(redgres_download_latest_release)"
  REDGRES_DOMAIN_RUNTIME_OPTIONAL=1
  redgres_update_apply "${release_path}"
  /usr/bin/rm -rf "$(/usr/bin/dirname "${release_path}")"
  redgres_owner_bootstrap "$(redgres_opt_current_link)/redgres"
  redgres_chown_app_state
  redgres_record_resolved_packages
  redgres_log "live install result=partial mode=${mode} postgres=${postgres_version} redis=${redis_version} pgbouncer=${pgbouncer_mode} os=${REDGRES_OS_CODENAME}"
  redgres_log 'listeners bound to 127.0.0.1 only; UFW public DB ports not opened'
  REDGRES_LOG_PREFIX=''
  redgres_finish_report
}
