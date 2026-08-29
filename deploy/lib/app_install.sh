#!/usr/bin/env bash
# App-release + owner bootstrap + finish report for the live fresh install (OPS-005 Partial).
# Owner password is captured through a named pipe and printed once in the finish box
# on a TTY. It is never written to installer logs. Redis/Postgres secrets stay in files.
set -euo pipefail

redgres_write_default_env() {
  local env_file='/etc/redgres/redgres.env' base
  /usr/bin/getent group redgres >/dev/null || redgres_die 'redgres group is missing'
  if [[ -f "${env_file}" ]]; then
    redgres_log 'redgres.env already-correct'
    return 0
  fi
  /usr/bin/mkdir -p /etc/redgres
  /usr/bin/chown root:redgres /etc/redgres
  /usr/bin/chmod 750 /etc/redgres
  base="$(redgres_bootstrap_base_url)"
  umask 077
  /usr/bin/cat >"${env_file}" <<EOF
REDGRES_ENVIRONMENT=production
REDGRES_ADDRESS=127.0.0.1:8790
REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989
REDGRES_BASE_URL=${base}
REDGRES_SQLITE_PATH=/var/lib/redgres/redgres.db
REDGRES_COOKIE_SECURE=false
REDGRES_BOOTSTRAP_UFW_REMOVE_CMD=/usr/libexec/redgres/bootstrap-ufw-remove.sh
REDGRES_POSTGRES_HOST=127.0.0.1
REDGRES_POSTGRES_PORT=5432
REDGRES_POSTGRES_DATABASE=postgres
REDGRES_POSTGRES_USER=redgres_admin
REDGRES_POSTGRES_PASSWORD_FILE=/etc/redgres/postgres.pass
REDGRES_POSTGRES_SSLMODE=require
REDGRES_POSTGRES_EXPECTED_MAJOR=${postgres_version}
REDGRES_REDIS_ADMIN_URL_FILE=/etc/redgres/redis.url
REDGRES_REDIS_EXPECTED_SERIES=${redis_version}
EOF
  /usr/bin/chmod 660 "${env_file}"
  /usr/bin/chown root:redgres "${env_file}"
  redgres_log 'default /etc/redgres/redgres.env written (bootstrap HTTP; CookieSecure false until domain TLS)'
}

redgres_fw_note() {
  redgres_log "$*"
}

redgres_assert_bootstrap_allow_ip() {
  local ip="$1"
  [[ -n "${ip}" ]] || return 1
  case "${ip}" in
    */*|*' '*|0.0.0.0|::|::1) return 1 ;;
  esac
  if [[ "${ip}" == *:* ]]; then
    [[ "${ip}" =~ ^[0-9a-fA-F:]+$ ]] || return 1
    return 0
  fi
  [[ "${ip}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || return 1
  case "${ip}" in
    127.*) return 1 ;;
  esac
  return 0
}

redgres_bootstrap_allow_from() {
  local raw="${REDGRES_BOOTSTRAP_ALLOW_FROM:-}"
  if [[ -z "${raw}" && -n "${SSH_CONNECTION:-}" ]]; then
    raw="${SSH_CONNECTION%% *}"
  fi
  if [[ -z "${raw}" && -n "${SSH_CLIENT:-}" ]]; then
    raw="${SSH_CLIENT%% *}"
  fi
  redgres_assert_bootstrap_allow_ip "${raw}" || return 1
  printf '%s' "${raw}"
}

redgres_ufw_bootstrap_allow_argv() {
  local from="$1"
  printf 'allow from %s to any port 8989 proto tcp comment redgres-bootstrap' "${from}"
}

redgres_app_unit_body() {
  local binary_path="$1"
  cat <<EOF
[Unit]
Description=Redgres control plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=redgres
Group=redgres
UMask=0077
EnvironmentFile=-/etc/redgres/redgres.env
ExecStart=${binary_path} serve
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

redgres_chown_app_state() {
  local db='/var/lib/redgres/redgres.db'
  /usr/bin/chown redgres:redgres /var/lib/redgres 2>/dev/null || true
  if [[ -e "${db}" ]]; then
    /usr/bin/chown redgres:redgres "${db}" 2>/dev/null || true
    /usr/bin/chown redgres:redgres "${db}-wal" "${db}-shm" 2>/dev/null || true
  fi
  if [[ -d /var/lib/redgres/secrets ]]; then
    /usr/bin/chown -R redgres:redgres /var/lib/redgres/secrets
  fi
  if [[ -f /etc/redgres/redgres.env ]]; then
    /usr/bin/chown root:redgres /etc/redgres/redgres.env
    /usr/bin/chmod 660 /etc/redgres/redgres.env
  fi
}

redgres_install_bootstrap_ufw_helper() {
  /usr/bin/mkdir -p /usr/libexec/redgres /etc/systemd/system
  /usr/bin/cat >/usr/libexec/redgres/bootstrap-ufw-remove.sh <<'EOF'
#!/bin/bash
set -euo pipefail
umask 077
PATH=/usr/sbin:/usr/bin:/sbin:/bin
if command -v ufw >/dev/null 2>&1; then
  i=0
  while [[ "${i}" -lt 20 ]]; do
    rule="$(ufw status numbered 2>/dev/null | grep -E '8989' | head -1 | grep -oE '[[:space:]]*[0-9]+' | tr -d ' ' || true)"
    [[ -n "${rule}" ]] || break
    ufw --force delete "${rule}" 2>/dev/null || break
    i=$((i + 1))
  done
  ufw delete allow 8989/tcp 2>/dev/null || true
fi
rm -f /var/lib/redgres/bootstrap-ufw-remove.requested
EOF
  /usr/bin/chmod 0755 /usr/libexec/redgres/bootstrap-ufw-remove.sh
  /usr/bin/chown root:root /usr/libexec/redgres/bootstrap-ufw-remove.sh 2>/dev/null || true
  /usr/bin/cat >/etc/systemd/system/redgres-bootstrap-ufw-remove.service <<'EOF'
[Unit]
Description=Remove Redgres bootstrap :8989 UFW rule

[Service]
Type=oneshot
User=root
ExecStart=/usr/libexec/redgres/bootstrap-ufw-remove.sh
EOF
  /usr/bin/cat >/etc/systemd/system/redgres-bootstrap-ufw-remove.path <<'EOF'
[Unit]
Description=Remove Redgres bootstrap UFW rule when requested

[Path]
PathExists=/var/lib/redgres/bootstrap-ufw-remove.requested
Unit=redgres-bootstrap-ufw-remove.service

[Install]
WantedBy=multi-user.target
EOF
  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable --now redgres-bootstrap-ufw-remove.path >/dev/null 2>&1 || true
  fi
}

redgres_ufw_restrict_bootstrap() {
  local from=""
  if ! command -v ufw >/dev/null 2>&1; then
    redgres_fw_note 'ufw: not installed; bootstrap :8989 is not source-restricted'
    return 0
  fi
  from="$(redgres_bootstrap_allow_from 2>/dev/null || true)"
  if [[ -z "${from}" ]]; then
    redgres_fw_note 'ufw: operator source IP unknown; set REDGRES_BOOTSTRAP_ALLOW_FROM; not opening 8989 to the world'
    return 0
  fi
  if ufw allow from "${from}" to any port 8989 proto tcp comment 'redgres-bootstrap'; then
    if ufw status 2>/dev/null | grep -q '^Status: active'; then
      redgres_fw_note "ufw: allow 8989/tcp from ${from} only"
    else
      redgres_fw_note "ufw: inactive; queued allow from ${from} to 8989 (enable UFW or a cloud firewall to enforce)"
    fi
  else
    redgres_fw_note 'ufw: failed to add source-restricted 8989 rule'
    return 1
  fi
}

redgres_install_bootstrap_firewall() {
  redgres_install_bootstrap_ufw_helper
  redgres_ufw_restrict_bootstrap || true
}

redgres_bootstrap_base_url() {
  local ip candidate
  for candidate in $(/usr/bin/hostname -I 2>/dev/null); do
    candidate="${candidate// /}"
    [[ -n "${candidate}" ]] || continue
    case "${candidate}" in
      127.*|10.*|192.168.*|172.1[6-9].*|172.2[0-9].*|172.3[0-1].*|169.254.*) continue ;;
    esac
    ip="${candidate}"
    break
  done
  if [[ ! "${ip}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    ip="$(/usr/bin/curl -fsS --max-time 3 https://api.ipify.org 2>/dev/null || true)"
  fi
  if [[ "${ip}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf 'http://%s:8989\n' "${ip}"
  else
    printf '%s\n' 'http://127.0.0.1:8989'
  fi
}

redgres_assert_github_release_url() {
  local url="$1"
  local version="$2"
  local name="$3"
  [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || redgres_die 'release version is not X.Y.Z'
  [[ "${url}" == "https://github.com/SSujitX/redgres/releases/download/v${version}/${name}" ]] || redgres_die 'release URL is not the expected GitHub asset'
}

redgres_release_urls_from_json() {
  local json="$1" version asset tarball_url sums_url
  version="$(printf '%s' "${json}" | /usr/bin/sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | /usr/bin/head -n1)"
  version="${version#v}"
  [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || redgres_die 'release tag_name missing or not X.Y.Z'
  asset="redgres_${version}_linux_amd64.tar.gz"
  tarball_url="$(printf '%s' "${json}" | /usr/bin/tr ',' '\n' | /usr/bin/sed -n "s/.*\"browser_download_url\"[[:space:]]*:[[:space:]]*\"\\([^\"]*${asset}\\)\".*/\\1/p" | /usr/bin/head -n1)"
  sums_url="$(printf '%s' "${json}" | /usr/bin/tr ',' '\n' | /usr/bin/sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*SHA256SUMS\)".*/\1/p' | /usr/bin/head -n1)"
  [[ -n "${tarball_url}" && -n "${sums_url}" ]] || redgres_die "release assets missing (${asset} / SHA256SUMS)"
  redgres_assert_github_release_url "${tarball_url}" "${version}" "${asset}"
  redgres_assert_github_release_url "${sums_url}" "${version}" 'SHA256SUMS'
  printf '%s\n%s\n' "${tarball_url}" "${sums_url}"
}

# /tmp is 1777; trusted-path rejects world-writable ancestors. Keep downloads
# under a root-owned 0700 jail so live update can open the tarball.
redgres_release_download_root() {
  printf '%s' '/var/lib/redgres-release'
}

redgres_prepare_release_download_dir() {
  local root workdir
  root="$(redgres_release_download_root)"
  /usr/bin/mkdir -p "${root}"
  if [[ "${EUID}" -eq 0 ]]; then
    /usr/bin/chown root:root "${root}"
  fi
  /usr/bin/chmod 700 "${root}"
  workdir="$(/usr/bin/mktemp -d "${root}/XXXXXX")"
  /usr/bin/chmod 700 "${workdir}"
  if [[ "${EUID}" -eq 0 ]]; then
    /usr/bin/chown root:root "${workdir}"
  fi
  printf '%s' "${workdir}"
}

redgres_download_latest_release() {
  local api='https://api.github.com/repos/SSujitX/redgres'
  local json urls tarball_url sums_url asset workdir
  json="$(/usr/bin/curl -fsSL -H 'Accept: application/vnd.github+json' "${api}/releases/latest")" || redgres_die 'GitHub latest release not found'
  urls="$(redgres_release_urls_from_json "${json}")"
  tarball_url="$(printf '%s\n' "${urls}" | /usr/bin/sed -n '1p')"
  sums_url="$(printf '%s\n' "${urls}" | /usr/bin/sed -n '2p')"
  asset="${tarball_url##*/}"
  workdir="$(redgres_prepare_release_download_dir)"
  /usr/bin/curl -fsSL -o "${workdir}/${asset}" "${tarball_url}"
  /usr/bin/curl -fsSL -o "${workdir}/SHA256SUMS" "${sums_url}"
  /usr/bin/chmod 600 "${workdir}/${asset}" "${workdir}/SHA256SUMS"
  if [[ "${EUID}" -eq 0 ]]; then
    /usr/bin/chown root:root "${workdir}/${asset}" "${workdir}/SHA256SUMS"
  fi
  printf '%s' "${workdir}/${asset}"
}

redgres_have_owner_tty() {
  { : >/dev/tty; } 2>/dev/null
}

redgres_owner_password_fifo() {
  printf '%s' "${REDGRES_OWNER_PASSWORD_FIFO:-/var/lib/redgres/owner-pass.fifo}"
}

redgres_read_owner_password_fifo() {
  local fifo="$1"
  if [[ -x /usr/bin/timeout ]]; then
    /usr/bin/timeout 30 /usr/bin/cat "${fifo}"
  else
    /usr/bin/cat "${fifo}"
  fi
}

# Generated owner password for the finish box. Never Redis/Postgres secrets.
REDGRES_FINISH_OWNER_PASSWORD=''

redgres_owner_bootstrap() {
  local bin="$1" db='/var/lib/redgres/redgres.db'
  local fifo parent reader rc=0
  REDGRES_FINISH_OWNER_PASSWORD=''
  if ! redgres_have_owner_tty; then
    redgres_log "Owner not created here (no controlling terminal). Run: ${bin} create-owner --username admin --sqlite-path ${db}"
    return 0
  fi
  fifo="$(redgres_owner_password_fifo)"
  parent="$(/usr/bin/dirname "${fifo}")"
  /usr/bin/mkdir -p "${parent}"
  /usr/bin/rm -f "${fifo}"
  /usr/bin/mkfifo -m 600 "${fifo}"
  if [[ "${EUID}" -eq 0 ]]; then
    /usr/bin/chown root:root "${fifo}"
  fi
  REDGRES_FINISH_OWNER_PASSWORD="$(
    redgres_read_owner_password_fifo "${fifo}" &
    reader=$!
    if ! "${bin}" create-owner --generate --username admin --sqlite-path "${db}" --password-fifo "${fifo}"; then
      /usr/bin/kill "${reader}" 2>/dev/null || true
      wait "${reader}" 2>/dev/null || true
      exit 1
    fi
    wait "${reader}"
  )" || rc=$?
  /usr/bin/rm -f "${fifo}"
  if [[ "${rc}" -ne 0 ]]; then
    REDGRES_FINISH_OWNER_PASSWORD=''
    redgres_die "create-owner --generate failed"
  fi
  REDGRES_FINISH_OWNER_PASSWORD="${REDGRES_FINISH_OWNER_PASSWORD%$'\n'}"
}

redgres_pkg_version() {
  local pkg="$1"
  if [[ -x /usr/bin/dpkg-query ]]; then
    /usr/bin/dpkg-query -W -f '${Version}' "${pkg}" 2>/dev/null || printf '%s' 'not-installed'
  else
    printf '%s' 'not-on-this-host'
  fi
}

redgres_installed_app_version() {
  local f='/opt/redgres/current/VERSION'
  if declare -F redgres_opt_current_link >/dev/null 2>&1; then
    f="$(redgres_opt_current_link)/VERSION"
  fi
  if [[ -f "${f}" && ! -L "${f}" ]]; then
    /usr/bin/tr -d '[:space:]' <"${f}"
    return 0
  fi
  printf '%s' 'unknown'
}

redgres_ufw_on_off() {
  if ! command -v ufw >/dev/null 2>&1; then
    printf '%s' 'not installed'
    return 0
  fi
  if ufw status 2>/dev/null | grep -q '^Status: active'; then
    printf '%s' 'on'
  else
    printf '%s' 'off'
  fi
}

redgres_ufw_bootstrap_note() {
  local from
  from="$(redgres_bootstrap_allow_from 2>/dev/null || true)"
  if [[ -z "${from}" ]]; then
    printf '%s' '8989 not world-opened (set REDGRES_BOOTSTRAP_ALLOW_FROM)'
    return 0
  fi
  printf '8989/tcp from %s only' "${from}"
}

redgres_finish_show_owner_password() {
  [[ -n "${REDGRES_FINISH_OWNER_PASSWORD}" ]] || return 1
  if [[ -t 1 || "${REDGRES_FINISH_SHOW_PASSWORD:-}" == "1" ]]; then
    return 0
  fi
  return 1
}

redgres_print_finish_box() {
  local -a rows=("$@")
  local w=0 r rule
  for r in "${rows[@]}"; do
    if ((${#r} > w)); then
      w=${#r}
    fi
  done
  if ((w < 62)); then
    w=62
  fi
  rule="$(printf '%*s' "$((w + 2))" '' | /usr/bin/tr ' ' '-')"
  printf '+%s+\n' "${rule}"
  for r in "${rows[@]}"; do
    printf '| %-*s |\n' "${w}" "${r}"
  done
  printf '+%s+\n' "${rule}"
}

redgres_finish_box_rows() {
  local origin="$1" login_line="$2"
  local app_ver pg_ver docker_ver pgb_ver pgb_line ufw_line ufw_boot os_line
  app_ver="$(redgres_installed_app_version)"
  pg_ver="$(redgres_pkg_version "postgresql-${postgres_version:-unknown}")"
  docker_ver="$(redgres_pkg_version docker.io)"
  ufw_line="$(redgres_ufw_on_off)"
  ufw_boot="$(redgres_ufw_bootstrap_note)"
  os_line="${REDGRES_OS_ID:-unknown} ${REDGRES_OS_VERSION_ID:-} (${REDGRES_OS_CODENAME:-})"
  if [[ "${pgbouncer_mode:-}" == "fresh" ]]; then
    pgb_ver="$(redgres_pkg_version pgbouncer)"
    pgb_line="fresh  ${pgb_ver}  (listen not configured)"
  else
    pgb_line="${pgbouncer_mode:-skipped}"
  fi
  redgres_print_finish_box \
    "Redgres install complete (Partial)" \
    "Mode           ${mode:-unknown}" \
    "OS             ${os_line}" \
    "Bootstrap UI   ${origin}" \
    "Owner login    ${login_line}" \
    "API origin     127.0.0.1:8790  (loopback, not public)" \
    "PostgreSQL     ${postgres_version:-?}  ${pg_ver}  127.0.0.1:5432" \
    "Redis          ${redis_version:-?}  127.0.0.1:6380  (loopback)" \
    "PgBouncer      ${pgb_line}" \
    "Docker         ${docker_ver}" \
    "Redgres        ${app_ver}" \
    "UFW            ${ufw_line}" \
    "UFW bootstrap  ${ufw_boot}" \
    "Public DB      ports not opened" \
    "Next           sign in, then Domain & Network for TLS"
}

redgres_finish_report() {
  local origin login_line
  origin="$(redgres_bootstrap_base_url)"
  origin="${origin%$'\n'}"
  if redgres_finish_show_owner_password; then
    login_line="admin / ${REDGRES_FINISH_OWNER_PASSWORD}"
  elif [[ -n "${REDGRES_FINISH_OWNER_PASSWORD}" ]]; then
    login_line='admin / (shown on this terminal only)'
  else
    login_line='run create-owner --username admin (no TTY here)'
  fi
  printf '\n'
  redgres_finish_box_rows "${origin}" "${login_line}"
  if ! redgres_finish_show_owner_password && [[ -n "${REDGRES_FINISH_OWNER_PASSWORD}" ]]; then
    if [[ -n "${REDGRES_FINISH_TTY:-}" ]]; then
      printf 'Owner login    admin / %s\n' "${REDGRES_FINISH_OWNER_PASSWORD}" >>"${REDGRES_FINISH_TTY}"
    elif redgres_have_owner_tty; then
      printf 'Owner login    admin / %s\n' "${REDGRES_FINISH_OWNER_PASSWORD}" >/dev/tty
    fi
  fi
  printf '\n'
  REDGRES_FINISH_OWNER_PASSWORD=''
}
