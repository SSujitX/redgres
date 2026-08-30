#!/bin/bash
# certbot renew deploy hook. Copies the renewed lineage onto PostgreSQL and reloads.
# Never prints private keys or the Cloudflare credentials file.
set -euo pipefail
umask 077
PATH=/usr/sbin:/usr/bin:/sbin:/bin

LINEAGE="${RENEWED_LINEAGE:-}"
[[ -n "${LINEAGE}" && -f "${LINEAGE}/fullchain.pem" && -f "${LINEAGE}/privkey.pem" ]] || exit 0

CERT_DIR="${REDGRES_TLS_CERT_DIR:-/etc/redgres/tls}"
/usr/bin/mkdir -p "${CERT_DIR}"
/usr/bin/install -m 0644 "${LINEAGE}/fullchain.pem" "${CERT_DIR}/fullchain.pem"
/usr/bin/install -m 0640 "${LINEAGE}/privkey.pem" "${CERT_DIR}/privkey.pem"
if /usr/bin/getent group ssl-cert >/dev/null 2>&1; then
  /usr/bin/chown root:ssl-cert "${CERT_DIR}/privkey.pem" 2>/dev/null || true
elif /usr/bin/getent group postgres >/dev/null 2>&1; then
  /usr/bin/chown root:postgres "${CERT_DIR}/privkey.pem" 2>/dev/null || true
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
