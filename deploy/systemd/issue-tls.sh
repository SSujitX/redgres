#!/bin/bash
# Privileged Let's Encrypt DNS-01 for grey-cloud db/rs (OPS-009).
# Reads hostnames from the request file. Never prints credential file contents.
set -euo pipefail
umask 077
PATH=/usr/sbin:/usr/bin:/sbin:/bin

REQUEST="${REDGRES_TLS_ISSUE_REQUEST_FILE:-/var/lib/redgres/tls-issue.request}"
RESULT="${REDGRES_TLS_ISSUE_RESULT_FILE:-/var/lib/redgres/tls-issue.result}"
CREDS="${REDGRES_CERTBOT_DNS_TOKEN_FILE:-/var/lib/redgres/secrets/certbot-dns.ini}"
LIVE_ROOT="${REDGRES_CERT_LIVE_DIR:-/etc/letsencrypt/live}"
# postgres/pgbouncer are not in group redgres; /etc/redgres is 0750 and unreadable to them.
CERT_DIR="${REDGRES_TLS_CERT_DIR:-/etc/ssl/redgres}"
CERTBOT="${REDGRES_CERTBOT_BIN:-certbot}"
PGBOUNCER_INI="${REDGRES_PGBOUNCER_INI:-/etc/pgbouncer/pgbouncer.ini}"

redgres_tls_prepare_dest() {
  local dir="$1"
  /usr/bin/mkdir -p "${dir}" || return 1
  /usr/bin/chmod 0755 "${dir}" || return 1
}

# 0640 root:ssl-cert is not readable by postgres on this host even when id lists ssl-cert.
# Reload re-reads certs as the service user, so copies must be owned by that user.
redgres_tls_readable_as() {
  local user="$1" dest_key="$2"
  /usr/bin/getent passwd "${user}" >/dev/null 2>&1 || return 1
  DEST_KEY="${dest_key}" su -s /bin/sh "${user}" -c '/usr/bin/test -r "$DEST_KEY"'
}

redgres_tls_install_owned() {
  local user="$1" dest_dir="$2" src_chain="$3" src_key="$4"
  /usr/bin/install -o "${user}" -g "${user}" -m 0644 "${src_chain}" "${dest_dir}/redgres-fullchain.pem" || return 1
  /usr/bin/install -o "${user}" -g "${user}" -m 0600 "${src_key}" "${dest_dir}/redgres-privkey.pem" || return 1
  redgres_tls_readable_as "${user}" "${dest_dir}/redgres-privkey.pem"
}

redgres_tls_apply_pgbouncer() {
  local dest_chain="$1" dest_key="$2" tmp
  [[ -f "${PGBOUNCER_INI}" ]] || return 0
  tmp="$(/usr/bin/mktemp "${PGBOUNCER_INI}.XXXXXX")"
  /usr/bin/awk -v c="${dest_chain}" -v k="${dest_key}" '
    /^client_tls_cert_file[[:space:]]*=/ { print "client_tls_cert_file = " c; next }
    /^client_tls_key_file[[:space:]]*=/ { print "client_tls_key_file = " k; next }
    { print }
  ' "${PGBOUNCER_INI}" >"${tmp}"
  if ! grep -q '^client_tls_cert_file[[:space:]]*=' "${tmp}"; then
    printf '%s\n' "client_tls_cert_file = ${dest_chain}" "client_tls_key_file = ${dest_key}" >>"${tmp}"
  fi
  /usr/bin/chmod --reference="${PGBOUNCER_INI}" "${tmp}" 2>/dev/null || /usr/bin/chmod 640 "${tmp}"
  /usr/bin/mv "${tmp}" "${PGBOUNCER_INI}"
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet pgbouncer; then
    systemctl reload pgbouncer >/dev/null 2>&1 || systemctl restart pgbouncer >/dev/null 2>&1 || return 1
  fi
}

redgres_tls_valid_hostname() {
  local h="$1" label
  [[ -n "${h}" && "${#h}" -le 253 && "${h}" == *.* ]] || return 1
  case "${h}" in
    *[[:space:]]*|*/*|*:*|*@*|*[\\]*|*[?]*|*[%]*|*[#]*|*[*]*) return 1 ;;
  esac
  IFS='.' read -r -a labels <<< "${h}"
  for label in "${labels[@]}"; do
    [[ -n "${label}" && "${#label}" -le 63 ]] || return 1
    [[ "${label}" != -* && "${label}" != *- ]] || return 1
    [[ "${label}" =~ ^[a-z0-9-]+$ ]] || return 1
  done
  return 0
}

redgres_tls_copy_certs() {
  local primary="$1" live dest_chain dest_key
  live="${LIVE_ROOT}/${primary}"
  [[ -f "${live}/fullchain.pem" && -f "${live}/privkey.pem" ]] || return 1
  redgres_tls_prepare_dest "${CERT_DIR}"
  dest_chain="${CERT_DIR}/fullchain.pem"
  dest_key="${CERT_DIR}/privkey.pem"
  /usr/bin/install -m 0644 "${live}/fullchain.pem" "${dest_chain}"
  /usr/bin/install -m 0640 "${live}/privkey.pem" "${dest_key}"
  local pgb_chain="${dest_chain}" pgb_key="${dest_key}"
  if [[ "${REDGRES_TLS_APPLY_PGBOUNCER:-${REDGRES_TLS_APPLY_POSTGRES_SSL:-1}}" == "1" ]]; then
    if [[ -d /etc/pgbouncer ]]; then
      local pgb_user=pgbouncer
      if ! /usr/bin/getent passwd pgbouncer >/dev/null 2>&1; then
        pgb_user=postgres
      fi
      if /usr/bin/getent passwd "${pgb_user}" >/dev/null 2>&1; then
        redgres_tls_install_owned "${pgb_user}" /etc/pgbouncer "${dest_chain}" "${dest_key}" || return 1
        pgb_chain=/etc/pgbouncer/redgres-fullchain.pem
        pgb_key=/etc/pgbouncer/redgres-privkey.pem
      fi
    fi
    redgres_tls_apply_pgbouncer "${pgb_chain}" "${pgb_key}" || return 1
  fi
  if [[ "${REDGRES_TLS_APPLY_POSTGRES_SSL:-1}" != "1" ]]; then
    return 0
  fi
  local maindir mains=() pg_chain pg_key
  shopt -s nullglob
  mains=(/etc/postgresql/*/main)
  shopt -u nullglob
  for maindir in "${mains[@]}"; do
    [[ -d "${maindir}/conf.d" ]] || continue
    pg_chain="${CERT_DIR}/fullchain.pem"
    pg_key="${CERT_DIR}/privkey.pem"
    if /usr/bin/getent passwd postgres >/dev/null 2>&1; then
      redgres_tls_install_owned postgres "${maindir}" "${dest_chain}" "${dest_key}" || return 1
      pg_chain="${maindir}/redgres-fullchain.pem"
      pg_key="${maindir}/redgres-privkey.pem"
    fi
    printf '%s\n' "ssl = on" "ssl_cert_file = '${pg_chain}'" "ssl_key_file = '${pg_key}'" >"${maindir}/conf.d/redgres-ssl.conf"
    /usr/bin/chmod 644 "${maindir}/conf.d/redgres-ssl.conf"
    local ver
    ver="$(printf '%s' "${maindir}" | /usr/bin/awk -F/ '{print $4}')"
    if [[ -n "${ver}" ]] && command -v pg_ctlcluster >/dev/null 2>&1; then
      pg_ctlcluster "${ver}" main reload >/dev/null 2>&1 || return 1
    fi
  done
}

fail() {
  printf '%s\n' 'failed' >"${RESULT}"
  /usr/bin/chmod 644 "${RESULT}" 2>/dev/null || true
  rm -f "${REQUEST}"
  exit 1
}

# Missing request is a PathChanged-on-delete race; do not write failed.
[[ -f "${REQUEST}" ]] || exit 0
[[ -f "${CREDS}" ]] || fail

mapfile -t hosts < <(/usr/bin/tr -d '\r' <"${REQUEST}" | /usr/bin/awk 'NF && $0 !~ /^#/')
[[ "${#hosts[@]}" -ge 2 ]] || fail
primary=""
args=()
for host in "${hosts[@]}"; do
  host="$(printf '%s' "${host}" | /usr/bin/tr '[:upper:]' '[:lower:]')"
  redgres_tls_valid_hostname "${host}" || fail
  [[ -n "${primary}" ]] || primary="${host}"
  args+=(-d "${host}")
done

if ! "${CERTBOT}" certonly \
  --non-interactive \
  --agree-tos \
  --register-unsafely-without-email \
  --dns-cloudflare \
  --dns-cloudflare-credentials "${CREDS}" \
  --dns-cloudflare-propagation-seconds 60 \
  "${args[@]}"; then
  fail
fi

redgres_tls_copy_certs "${primary}" || fail
{
  printf '%s\n' 'issued'
  printf '%s\n' "${hosts[@]}"
} >"${RESULT}"
/usr/bin/chmod 644 "${RESULT}" 2>/dev/null || true
rm -f "${REQUEST}"
exit 0
