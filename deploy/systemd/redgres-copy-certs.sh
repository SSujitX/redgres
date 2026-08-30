#!/bin/bash
# certbot renew deploy hook. Copies the renewed lineage onto PostgreSQL and reloads.
# Never prints private keys or the Cloudflare credentials file.
set -euo pipefail
umask 077
PATH=/usr/sbin:/usr/bin:/sbin:/bin

LINEAGE="${RENEWED_LINEAGE:-}"
[[ -n "${LINEAGE}" && -f "${LINEAGE}/fullchain.pem" && -f "${LINEAGE}/privkey.pem" ]] || exit 0

CERT_DIR="${REDGRES_TLS_CERT_DIR:-/etc/ssl/redgres}"
PGBOUNCER_INI="${REDGRES_PGBOUNCER_INI:-/etc/pgbouncer/pgbouncer.ini}"
/usr/bin/mkdir -p "${CERT_DIR}"
if /usr/bin/getent group ssl-cert >/dev/null 2>&1; then
  /usr/bin/chown root:ssl-cert "${CERT_DIR}" 2>/dev/null || true
  /usr/bin/chmod 0750 "${CERT_DIR}"
elif /usr/bin/getent group postgres >/dev/null 2>&1; then
  /usr/bin/chown root:postgres "${CERT_DIR}" 2>/dev/null || true
  /usr/bin/chmod 0750 "${CERT_DIR}"
else
  /usr/bin/chmod 0755 "${CERT_DIR}"
fi
/usr/bin/install -m 0644 "${LINEAGE}/fullchain.pem" "${CERT_DIR}/fullchain.pem"
/usr/bin/install -m 0640 "${LINEAGE}/privkey.pem" "${CERT_DIR}/privkey.pem"
if /usr/bin/getent group ssl-cert >/dev/null 2>&1; then
  /usr/bin/chown root:ssl-cert "${CERT_DIR}/privkey.pem" 2>/dev/null || true
elif /usr/bin/getent group postgres >/dev/null 2>&1; then
  /usr/bin/chown root:postgres "${CERT_DIR}/privkey.pem" 2>/dev/null || true
fi
if [[ -f "${PGBOUNCER_INI}" ]]; then
  tmp="$(/usr/bin/mktemp "${PGBOUNCER_INI}.XXXXXX")"
  /usr/bin/awk -v c="${CERT_DIR}/fullchain.pem" -v k="${CERT_DIR}/privkey.pem" '
    /^client_tls_cert_file[[:space:]]*=/ { print "client_tls_cert_file = " c; next }
    /^client_tls_key_file[[:space:]]*=/ { print "client_tls_key_file = " k; next }
    { print }
  ' "${PGBOUNCER_INI}" >"${tmp}"
  /usr/bin/chmod --reference="${PGBOUNCER_INI}" "${tmp}" 2>/dev/null || /usr/bin/chmod 640 "${tmp}"
  /usr/bin/mv "${tmp}" "${PGBOUNCER_INI}"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl reload pgbouncer >/dev/null 2>&1 || systemctl restart pgbouncer >/dev/null 2>&1 || true
  fi
fi

confdir=""
for confdir in /etc/postgresql/*/main/conf.d; do
  [[ -d "${confdir}" ]] || continue
  printf '%s\n' "ssl = on" "ssl_cert_file = '${CERT_DIR}/fullchain.pem'" "ssl_key_file = '${CERT_DIR}/privkey.pem'" >"${confdir}/redgres-ssl.conf"
  /usr/bin/chmod 644 "${confdir}/redgres-ssl.conf"
  ver="$(printf '%s' "${confdir}" | /usr/bin/awk -F/ '{print $4}')"
  if [[ -n "${ver}" ]] && command -v pg_ctlcluster >/dev/null 2>&1; then
    pg_ctlcluster "${ver}" main reload >/dev/null 2>&1 || exit 1
  fi
done
exit 0
