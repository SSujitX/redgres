#!/usr/bin/env bash
# App-release + owner bootstrap + finish report for the live fresh install (OPS-005 Partial).
# Owner password is captured through a named pipe and printed once in the finish box
# on a TTY. It is never written to installer logs. Redis/Postgres secrets stay in files.
set -euo pipefail

# Domain wizard secret *paths* only. Files are created later by the UI/API
# (0600, never logged). Empty path disables token paste / apply.
redgres_domain_secret_env_defaults() {
  cat <<'EOF'
REDGRES_CLOUDFLARE_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-api-token
REDGRES_TUNNEL_TOKEN_FILE=/var/lib/redgres/secrets/cloudflared-tunnel-token
REDGRES_CLOUDFLARE_OAUTH_CLIENT_FILE=/var/lib/redgres/secrets/cloudflare-oauth-client.json
REDGRES_CLOUDFLARE_OAUTH_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-oauth-token.json
REDGRES_CERTBOT_DNS_TOKEN_FILE=/var/lib/redgres/secrets/certbot-dns.ini
REDGRES_TLS_ISSUE_REQUEST_FILE=/var/lib/redgres/tls-issue.request
REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres-tls/issue.result
EOF
}

# Append missing KEY=value lines from stdin. Never overwrite an existing key.
redgres_env_ensure_lines() {
  local env_file="$1"
  local line key added=0
  [[ -f "${env_file}" ]] || return 1
  while IFS= read -r line || [[ -n "${line}" ]]; do
    [[ -n "${line}" ]] || continue
    key="${line%%=*}"
    [[ -n "${key}" && "${key}" != "${line}" ]] || continue
    if grep -qE "^${key}=" "${env_file}"; then
      continue
    fi
    printf '%s\n' "${line}" >>"${env_file}"
    added=1
  done
  [[ "${added}" -eq 1 ]]
}

redgres_migrate_tls_result_path() {
  local env_file="$1" tmp
  grep -q '^REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres/tls-issue.result$' "${env_file}" || return 1
  tmp="$(/usr/bin/mktemp "${env_file}.XXXXXX")" || return 1
  /usr/bin/awk '{
    if ($0 == "REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres/tls-issue.result") {
      print "REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres-tls/issue.result"
    } else {
      print
    }
  }' "${env_file}" >"${tmp}"
  /usr/bin/chmod --reference="${env_file}" "${tmp}" 2>/dev/null || /usr/bin/chmod 600 "${tmp}"
  /usr/bin/chown --reference="${env_file}" "${tmp}" 2>/dev/null || true
  /usr/bin/mv -fT "${tmp}" "${env_file}"
}

redgres_ensure_secrets_dir() {
  local dir="${REDGRES_SECRETS_DIR:-/var/lib/redgres/secrets}"
  /usr/bin/mkdir -p "${dir}"
  if /usr/bin/getent passwd redgres >/dev/null 2>&1; then
    /usr/bin/chown redgres:redgres "${dir}" 2>/dev/null || true
  fi
  /usr/bin/chmod 700 "${dir}"
}

redgres_ensure_domain_secret_env() {
  local env_file="${1:-/etc/redgres/redgres.env}" changed=0
  redgres_ensure_secrets_dir
  [[ -f "${env_file}" ]] || return 1
  redgres_migrate_tls_result_path "${env_file}" && changed=1
  redgres_domain_secret_env_defaults | redgres_env_ensure_lines "${env_file}" && changed=1
  [[ "${changed}" -eq 1 ]]
}

redgres_pgbouncer_env_lines() {
  if [[ "${redgres_pgbouncer_listen:-}" == "1" ]]; then
    printf 'REDGRES_POSTGRES_POOLED_PORT=6432\n'
  fi
}

redgres_ensure_pgbouncer_env() {
  local env_file="${1:-/etc/redgres/redgres.env}"
  [[ -f "${env_file}" ]] || return 1
  if redgres_pgbouncer_env_lines | redgres_env_ensure_lines "${env_file}"; then
    return 0
  fi
  return 1
}

redgres_expert_tool_env_defaults() {
  cat <<'EOF'
REDGRES_PGADMIN_EMAIL=admin@redgres.com
REDGRES_PGADMIN_PASSWORD_FILE=/var/lib/redgres/secrets/pgadmin.pass
REDGRES_PGADMIN_MASTER_PASSWORD_FILE=/var/lib/redgres/secrets/pgadmin.master
REDGRES_TOOL_GATE_PGADMIN_LISTEN=127.0.0.1:5050
REDGRES_TOOL_GATE_PGADMIN_UPSTREAM=http://127.0.0.1:5052
REDGRES_TOOL_GATE_REDISINSIGHT_LISTEN=127.0.0.1:5540
REDGRES_TOOL_GATE_REDISINSIGHT_UPSTREAM=http://127.0.0.1:5542
EOF
}

redgres_ensure_expert_tool_env() {
  local env_file="${1:-/etc/redgres/redgres.env}"
  redgres_ensure_secrets_dir
  [[ -f "${env_file}" ]] || return 1
  if redgres_expert_tool_env_defaults | redgres_env_ensure_lines "${env_file}"; then
    return 0
  fi
  return 1
}

redgres_write_default_env() {
  local env_file='/etc/redgres/redgres.env' base added=0
  /usr/bin/getent group redgres >/dev/null || redgres_die 'redgres group is missing'
  redgres_ensure_secrets_dir
  if [[ -f "${env_file}" ]]; then
    if redgres_ensure_domain_secret_env "${env_file}"; then
      added=1
    fi
    if redgres_ensure_pgbouncer_env "${env_file}"; then
      added=1
    fi
    if redgres_ensure_expert_tool_env "${env_file}"; then
      added=1
    fi
    if [[ "${added}" -eq 1 ]]; then
      /usr/bin/chmod 660 "${env_file}"
      /usr/bin/chown root:redgres "${env_file}"
      redgres_log 'redgres.env missing keys appended'
    else
      redgres_log 'redgres.env already-correct'
    fi
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
  redgres_domain_secret_env_defaults >>"${env_file}"
  redgres_pgbouncer_env_lines >>"${env_file}"
  redgres_expert_tool_env_defaults >>"${env_file}"
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

redgres_ip_from_who_line() {
  local line="$1" host
  [[ -n "${line}" ]] || return 1
  case "${line}" in
    *'('*')') ;;
    *) return 1 ;;
  esac
  host="${line##*(}"
  host="${host%)}"
  host="${host#[}"
  host="${host%]}"
  redgres_assert_bootstrap_allow_ip "${host}" || return 1
  printf '%s' "${host}"
}

redgres_ssh_client_ip_from_environ_stream() {
  local key raw conn="" client=""
  while IFS= read -r -d '' key; do
    case "${key}" in
      SSH_CONNECTION=*)
        raw="${key#SSH_CONNECTION=}"
        conn="${raw%% *}"
        ;;
      SSH_CLIENT=*)
        raw="${key#SSH_CLIENT=}"
        client="${raw%% *}"
        ;;
    esac
  done
  if redgres_assert_bootstrap_allow_ip "${conn}"; then
    printf '%s' "${conn}"
    return 0
  fi
  if redgres_assert_bootstrap_allow_ip "${client}"; then
    printf '%s' "${client}"
    return 0
  fi
  return 1
}

redgres_proc_ppid() {
  local line pid=""
  [[ -r "$1" ]] || return 1
  while IFS= read -r line; do
    case "${line}" in
      PPid:*)
        pid="${line#PPid:}"
        pid="${pid#"${pid%%[![:space:]]*}"}"
        printf '%s' "${pid}"
        return 0
        ;;
    esac
  done <"$1"
  return 1
}

redgres_ssh_client_ip_from_ancestors() {
  local envf pid i found
  if [[ -n "${REDGRES_BOOTSTRAP_PROC_ENVIRON+x}" ]]; then
    envf="${REDGRES_BOOTSTRAP_PROC_ENVIRON}"
    [[ -n "${envf}" && -r "${envf}" ]] || return 1
    redgres_ssh_client_ip_from_environ_stream <"${envf}"
    return
  fi
  pid="${PPID:-}"
  i=0
  while [[ "${i}" -lt 8 && "${pid}" =~ ^[1-9][0-9]*$ ]]; do
    envf="/proc/${pid}/environ"
    if [[ -r "${envf}" ]]; then
      found="$(redgres_ssh_client_ip_from_environ_stream <"${envf}" || true)"
      if redgres_assert_bootstrap_allow_ip "${found}"; then
        printf '%s' "${found}"
        return 0
      fi
    fi
    pid="$(redgres_proc_ppid "/proc/${pid}/status" || true)"
    i=$((i + 1))
  done
  return 1
}

redgres_ssh_client_ip_from_who() {
  local line
  if [[ -n "${REDGRES_BOOTSTRAP_WHO_LINE+x}" ]]; then
    line="${REDGRES_BOOTSTRAP_WHO_LINE}"
  else
    line="$(who -m 2>/dev/null || true)"
    [[ -n "${line}" ]] || line="$(who am i 2>/dev/null || true)"
  fi
  redgres_ip_from_who_line "${line}"
}

redgres_bootstrap_allow_from() {
  local raw="${REDGRES_BOOTSTRAP_ALLOW_FROM:-}"
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  raw=""
  if [[ -n "${SSH_CONNECTION:-}" ]]; then
    raw="${SSH_CONNECTION%% *}"
  fi
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  raw=""
  if [[ -n "${SSH_CLIENT:-}" ]]; then
    raw="${SSH_CLIENT%% *}"
  fi
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  raw="$(redgres_ssh_client_ip_from_ancestors || true)"
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  raw="$(redgres_ssh_client_ip_from_who || true)"
  if redgres_assert_bootstrap_allow_ip "${raw}"; then
    printf '%s' "${raw}"
    return 0
  fi
  return 1
}

redgres_prompt_say() {
  if [[ -w /dev/tty ]]; then
    printf '%s\n' "$1" >/dev/tty
  else
    printf '%s\n' "$1" >&2
  fi
}

redgres_prompt_bootstrap_allow_ip() {
  local input="${REDGRES_BOOTSTRAP_ALLOW_TTY:-/dev/tty}" answer i
  [[ -r "${input}" ]] || return 1
  for i in 1 2 3; do
    redgres_prompt_say 'First-run console (port 8989) opens only for your current public IP.'
    redgres_prompt_say 'It closes after Domain & Network confirm. Not opened to the internet.'
    if [[ -w /dev/tty ]]; then
      printf '%s' 'Public IP you will browse from: ' >/dev/tty
    else
      printf '%s' 'Public IP you will browse from: ' >&2
    fi
    IFS= read -r answer <"${input}" || return 1
    answer="${answer//[$' \t\r\n']/}"
    if redgres_assert_bootstrap_allow_ip "${answer}"; then
      printf '%s' "${answer}"
      return 0
    fi
    redgres_prompt_say 'That is not a single operator IP (no 0.0.0.0, no CIDR).'
  done
  return 1
}

redgres_resolve_bootstrap_allow_from() {
  local from
  from="$(redgres_bootstrap_allow_from || true)"
  if [[ -n "${from}" ]]; then
    printf '%s' "${from}"
    return 0
  fi
  from="$(redgres_prompt_bootstrap_allow_ip || true)"
  if redgres_assert_bootstrap_allow_ip "${from}"; then
    REDGRES_BOOTSTRAP_ALLOW_FROM="${from}"
    printf '%s' "${from}"
    return 0
  fi
  return 1
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
$(redgres_app_unit_runtime_lines "${binary_path}" '/etc/redgres/secrets/legacy-vault-secret' '/etc/redgres/secrets/legacy-vault-secret.managed')
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
  redgres_ensure_secrets_dir
  if [[ -d /var/lib/redgres/secrets ]]; then
    /usr/bin/chown -R redgres:redgres /var/lib/redgres/secrets
  fi
  if declare -F redgres_restore_pgadmin_master_ownership >/dev/null 2>&1; then
    redgres_restore_pgadmin_master_ownership
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
  from="$(redgres_resolve_bootstrap_allow_from || true)"
  if [[ -z "${from}" ]]; then
    redgres_fw_note 'ufw: operator source IP unknown; not opening 8989 to the world'
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

# Official stable suite from https://pkg.cloudflare.com/ (Debian/Ubuntu including 24.04 and 26.04).
redgres_cloudflared_apt_source_line() {
  printf '%s\n' 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared any main'
}

redgres_domain_unit_src() {
  printf '%s' "${REDGRES_DOMAIN_UNIT_SRC:-${script_dir}/systemd}"
}

redgres_libexec_dir() {
  printf '%s' "${REDGRES_LIBEXEC_DIR:-/usr/libexec/redgres}"
}

redgres_systemd_unit_dir() {
  printf '%s' "${REDGRES_SYSTEMD_UNIT_DIR:-/etc/systemd/system}"
}

redgres_domain_runtime_is_managed() {
  local libexec units hook
  libexec="$(redgres_libexec_dir)"
  units="$(redgres_systemd_unit_dir)"
  hook="${REDGRES_CERTBOT_DEPLOY_HOOK_DIR:-/etc/letsencrypt/renewal-hooks/deploy}"
  [[ -e "${libexec}/issue-tls.sh" ||
    -e "${units}/redgres-tls-issue.service" ||
    -e "${units}/redgres-tls-issue.path" ||
    -e "${units}/cloudflared-redgres.service" ||
    -e "${hook}/redgres-copy-certs.sh" ]]
}

redgres_install_file() {
  local src="$1" dest="$2" mode="$3"
  [[ -f "${src}" ]] || return 1
  /usr/bin/mkdir -p "$(/usr/bin/dirname "${dest}")"
  /usr/bin/install -m "${mode}" "${src}" "${dest}"
}

redgres_install_cloudflared_units() {
  local src dest_lib dest_sys
  src="$(redgres_domain_unit_src)"
  dest_lib="$(redgres_libexec_dir)"
  dest_sys="$(redgres_systemd_unit_dir)"
  /usr/bin/mkdir -p "${dest_lib}" "${dest_sys}"
  redgres_install_file "${src}/cloudflared-run.sh" "${dest_lib}/cloudflared-run.sh" 0755
  redgres_install_file "${src}/cloudflared-redgres.service" "${dest_sys}/cloudflared-redgres.service" 0644
  redgres_install_file "${src}/cloudflared-redgres-restart.service" "${dest_sys}/cloudflared-redgres-restart.service" 0644
  redgres_install_file "${src}/cloudflared-redgres.path" "${dest_sys}/cloudflared-redgres.path" 0644
}

redgres_install_tls_issue_helper() {
  local src dest_lib dest_sys hook
  src="$(redgres_domain_unit_src)"
  dest_lib="$(redgres_libexec_dir)"
  dest_sys="$(redgres_systemd_unit_dir)"
  hook="${REDGRES_CERTBOT_DEPLOY_HOOK_DIR:-/etc/letsencrypt/renewal-hooks/deploy}"
  /usr/bin/mkdir -p "${dest_lib}" "${dest_sys}"
  redgres_install_file "${src}/issue-tls.sh" "${dest_lib}/issue-tls.sh" 0755
  redgres_install_file "${src}/redgres-tls-issue.service" "${dest_sys}/redgres-tls-issue.service" 0644
  redgres_install_file "${src}/redgres-tls-issue.path" "${dest_sys}/redgres-tls-issue.path" 0644
  if [[ "${REDGRES_SKIP_CERTBOT_HOOK:-}" != "1" ]]; then
    /usr/bin/mkdir -p "${hook}"
    redgres_install_file "${src}/redgres-copy-certs.sh" "${hook}/redgres-copy-certs.sh" 0755
  fi
}

redgres_enable_cloudflared_apt() {
  local keyring='/usr/share/keyrings/cloudflare-main.gpg' list='/etc/apt/sources.list.d/cloudflared.list'
  /usr/bin/mkdir -p /usr/share/keyrings
  if [[ ! -f "${keyring}" ]]; then
    /usr/bin/curl -fsSL --max-time 30 -o "${keyring}" https://pkg.cloudflare.com/cloudflare-main.gpg || return 1
    /usr/bin/chmod 644 "${keyring}"
  fi
  redgres_cloudflared_apt_source_line >"${list}"
  /usr/bin/chmod 644 "${list}"
}

redgres_install_domain_packages() {
  redgres_enable_cloudflared_apt || return 1
  if declare -F redgres_apt_get >/dev/null 2>&1; then
    redgres_apt_get update || return 1
  else
    DEBIAN_FRONTEND=noninteractive /usr/bin/apt-get update || return 1
  fi
  redgres_apt_install cloudflared || return 1
  redgres_apt_install certbot || return 1
  redgres_apt_install python3-certbot-dns-cloudflare || return 1
}

redgres_install_domain_runtime() {
  if [[ "${REDGRES_DOMAIN_RUNTIME_IF_MANAGED:-}" == "1" ]] && ! redgres_domain_runtime_is_managed; then
    redgres_log 'domain runtime: not managed on this host; skipped'
    return 0
  fi
  redgres_install_cloudflared_units || return 1
  redgres_install_tls_issue_helper || return 1
  if [[ "${EUID}" -eq 0 && "${REDGRES_OPT_ROOT:-/opt/redgres}" == "/opt/redgres" && -x /usr/bin/apt-get && "${REDGRES_SKIP_DOMAIN_PACKAGES:-}" != "1" ]]; then
    if declare -F redgres_apt_install >/dev/null 2>&1; then
      if ! redgres_install_domain_packages; then
        if [[ "${REDGRES_DOMAIN_PACKAGES_OPTIONAL:-}" == "1" ]]; then
          redgres_log 'domain runtime: package refresh not applied; installed helper files remain version-matched'
        else
          return 1
        fi
      fi
    fi
  fi
  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || return 1
    systemctl disable --now cloudflared.service >/dev/null 2>&1 || true
    systemctl enable cloudflared-redgres.service >/dev/null 2>&1 || return 1
    # ConditionPathExists skips this when the token is not written yet.
    systemctl start cloudflared-redgres.service >/dev/null 2>&1 || true
    systemctl enable --now cloudflared-redgres.path >/dev/null 2>&1 || return 1
    systemctl enable --now redgres-tls-issue.path >/dev/null 2>&1 || return 1
    systemctl enable --now certbot.timer >/dev/null 2>&1 || true
  fi
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

# GitHub releases/latest can lag installer scripts in a git clone.
redgres_owner_has_password_fifo() {
  local bin="$1" help
  help="$("${bin}" create-owner -h 2>&1 || true)"
  [[ "${help}" == *'-password-fifo'* ]]
}

redgres_owner_bootstrap() {
  local bin="$1" db='/var/lib/redgres/redgres.db'
  local fifo parent reader rc=0
  REDGRES_FINISH_OWNER_PASSWORD=''
  if ! redgres_have_owner_tty; then
    redgres_log "Owner not created here (no controlling terminal). Run: ${bin} create-owner --username admin --sqlite-path ${db}"
    return 0
  fi
  if ! redgres_owner_has_password_fifo "${bin}"; then
    redgres_log 'create-owner has no --password-fifo (downloaded release older than installer); password prints once on this TTY'
    "${bin}" create-owner --generate --username admin --sqlite-path "${db}" || redgres_die "create-owner --generate failed"
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
    printf '%s' 'first-run console not opened'
    return 0
  fi
  printf '8989/tcp from %s only' "${from}"
}

redgres_finish_show_owner_password() {
  [[ -n "${REDGRES_FINISH_OWNER_PASSWORD}" ]] || return 1
  redgres_finish_show_secret
}

redgres_finish_show_secret() {
  if [[ -t 1 || "${REDGRES_FINISH_SHOW_PASSWORD:-}" == "1" ]]; then
    return 0
  fi
  return 1
}

redgres_read_oneline_secret() {
  local passfile="$1" value
  [[ -f "${passfile}" && ! -L "${passfile}" ]] || return 1
  value="$(/usr/bin/tr -d '\n\r' <"${passfile}")"
  [[ -n "${value}" ]] || return 1
  printf '%s' "${value}"
}

redgres_pgadmin_login_file() {
  printf '%s' "${REDGRES_PGADMIN_PASSWORD_FILE:-/var/lib/redgres/secrets/pgadmin.pass}"
}

redgres_pgadmin_master_file() {
  printf '%s' "${REDGRES_PGADMIN_MASTER_PASSWORD_FILE:-/var/lib/redgres/secrets/pgadmin.master}"
}

redgres_finish_pgadmin_email() {
  printf '%s' "${REDGRES_PGADMIN_EMAIL:-admin@redgres.com}"
}

redgres_finish_write_tty_secret() {
  local line="$1"
  if [[ -n "${REDGRES_FINISH_TTY:-}" ]]; then
    printf '%s\n' "${line}" >>"${REDGRES_FINISH_TTY}"
    return 0
  fi
  if redgres_have_owner_tty; then
    printf '%s\n' "${line}" >/dev/tty
    return 0
  fi
  return 1
}

redgres_finish_can_write_tty_secret() {
  [[ -n "${REDGRES_FINISH_TTY:-}" ]] && return 0
  redgres_have_owner_tty && return 0
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
  local origin="$1" login_line="$2" pg_login_line="$3" pg_master_line="$4"
  local app_ver pg_ver docker_ver pgb_ver pgb_line ufw_line ufw_boot os_line
  app_ver="$(redgres_installed_app_version)"
  pg_ver="$(redgres_pkg_version "postgresql-${postgres_version:-unknown}")"
  docker_ver="$(redgres_pkg_version docker.io)"
  ufw_line="$(redgres_ufw_on_off)"
  ufw_boot="$(redgres_ufw_bootstrap_note)"
  os_line="${REDGRES_OS_ID:-unknown} ${REDGRES_OS_VERSION_ID:-} (${REDGRES_OS_CODENAME:-})"
  if [[ "${pgbouncer_mode:-}" == "fresh" ]]; then
    pgb_ver="$(redgres_pkg_version pgbouncer)"
    if [[ "${redgres_pgbouncer_listen:-}" == "1" ]]; then
      pgb_line="fresh  ${pgb_ver}  127.0.0.1:6432"
    else
      pgb_line="fresh  ${pgb_ver}  (listen not configured)"
    fi
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
    "pgAdmin        127.0.0.1:5052  (loopback)" \
    "pgAdmin login  ${pg_login_line}" \
    "pgAdmin master ${pg_master_line}" \
    "PgBouncer      ${pgb_line}" \
    "Docker         ${docker_ver}" \
    "Redgres        ${app_ver}" \
    "UFW            ${ufw_line}" \
    "UFW bootstrap  ${ufw_boot}" \
    "Public DB      ports not opened" \
    "Next           sign in, then Domain & Network for TLS"
}

redgres_finish_report() {
  local origin login_line pg_login_line pg_master_line pg_login_secret pg_master_secret email
  origin="$(redgres_bootstrap_base_url)"
  origin="${origin%$'\n'}"
  if redgres_finish_show_owner_password; then
    login_line="admin / ${REDGRES_FINISH_OWNER_PASSWORD}"
  elif [[ -n "${REDGRES_FINISH_OWNER_PASSWORD}" ]]; then
    if redgres_finish_can_write_tty_secret; then
      login_line='admin / (shown on this terminal only)'
    else
      login_line='admin / not shown (no TTY)'
    fi
  else
    login_line='run create-owner --username admin (no TTY here)'
  fi
  email="$(redgres_finish_pgadmin_email)"
  if pg_login_secret="$(redgres_read_oneline_secret "$(redgres_pgadmin_login_file)")"; then
    if redgres_finish_show_secret; then
      pg_login_line="${email} / ${pg_login_secret}"
    elif redgres_finish_can_write_tty_secret; then
      pg_login_line="${email} / (shown on this terminal only)"
    else
      pg_login_line="${email} / Reveal in Expert tools"
    fi
  else
    pg_login_line='not configured'
    pg_login_secret=''
  fi
  if pg_master_secret="$(redgres_read_oneline_secret "$(redgres_pgadmin_master_file)")"; then
    if redgres_finish_show_secret; then
      pg_master_line="${pg_master_secret}"
    elif redgres_finish_can_write_tty_secret; then
      pg_master_line='(shown on this terminal only)'
    else
      pg_master_line='Reveal in Expert tools'
    fi
  else
    pg_master_line='not configured'
    pg_master_secret=''
  fi
  printf '\n'
  redgres_finish_box_rows "${origin}" "${login_line}" "${pg_login_line}" "${pg_master_line}"
  if ! redgres_finish_show_owner_password && [[ -n "${REDGRES_FINISH_OWNER_PASSWORD}" ]]; then
    redgres_finish_write_tty_secret "Owner login    admin / ${REDGRES_FINISH_OWNER_PASSWORD}" || true
  fi
  if ! redgres_finish_show_secret && [[ -n "${pg_login_secret}" ]]; then
    redgres_finish_write_tty_secret "pgAdmin login  ${email} / ${pg_login_secret}" || true
  fi
  if ! redgres_finish_show_secret && [[ -n "${pg_master_secret}" ]]; then
    redgres_finish_write_tty_secret "pgAdmin master ${pg_master_secret}" || true
  fi
  printf '\n'
  REDGRES_FINISH_OWNER_PASSWORD=''
  pg_login_secret=''
  pg_master_secret=''
}
