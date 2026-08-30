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
  /usr/bin/mkdir -p "${dir}"
  if /usr/bin/getent group ssl-cert >/dev/null 2>&1; then
    /usr/bin/chown root:ssl-cert "${dir}" 2>/dev/null || true
    /usr/bin/chmod 0750 "${dir}"
  elif /usr/bin/getent group postgres >/dev/null 2>&1; then
    /usr/bin/chown root:postgres "${dir}" 2>/dev/null || true
    /usr/bin/chmod 0750 "${dir}"
  else
    /usr/bin/chmod 0755 "${dir}"
  fi
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
  /usr/bin/chmod --reference="${PGBOUNCER_INI}" "${tmp}" 2>/dev/null || /usr/bin/chmod 640 "${tmp}"
  /usr/bin/mv "${tmp}" "${PGBOUNCER_INI}"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl reload pgbouncer >/dev/null 2>&1 || systemctl restart pgbouncer >/dev/null 2>&1 || true
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
  if /usr/bin/getent group ssl-cert >/dev/null 2>&1; then
    /usr/bin/chown root:ssl-cert "${dest_key}" 2>/dev/null || true
  elif /usr/bin/getent group postgres >/dev/null 2>&1; then
    /usr/bin/chown root:postgres "${dest_key}" 2>/dev/null || true
  fi
  if [[ "${REDGRES_TLS_APPLY_PGBOUNCER:-${REDGRES_TLS_APPLY_POSTGRES_SSL:-1}}" == "1" ]]; then
    redgres_tls_apply_pgbouncer "${dest_chain}" "${dest_key}"
  fi
  if [[ "${REDGRES_TLS_APPLY_POSTGRES_SSL:-1}" != "1" ]]; then
    return 0
  fi
  local confdir confdirs=()
  shopt -s nullglob
  confdirs=(/etc/postgresql/*/main/conf.d)
  shopt -u nullglob
  for confdir in "${confdirs[@]}"; do
    [[ -d "${confdir}" ]] || continue
    printf '%s\n' "ssl = on" "ssl_cert_file = '${dest_chain}'" "ssl_key_file = '${dest_key}'" >"${confdir}/redgres-ssl.conf"
    /usr/bin/chmod 644 "${confdir}/redgres-ssl.conf"
    local ver
    ver="$(printf '%s' "${confdir}" | /usr/bin/awk -F/ '{print $4}')"
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
