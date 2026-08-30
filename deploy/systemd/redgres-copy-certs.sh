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
/usr/bin/chmod 0755 "${CERT_DIR}"
/usr/bin/install -m 0644 "${LINEAGE}/fullchain.pem" "${CERT_DIR}/fullchain.pem"
/usr/bin/install -m 0640 "${LINEAGE}/privkey.pem" "${CERT_DIR}/privkey.pem"

redgres_tls_readable_as() {
  local user="$1" dest_key="$2"
  /usr/bin/getent passwd "${user}" >/dev/null 2>&1 || return 1
  DEST_KEY="${dest_key}" su -s /bin/sh "${user}" -c '/usr/bin/test -r "$DEST_KEY"'
}

redgres_tls_install_owned() {
  local user="$1" dest_dir="$2"
  /usr/bin/install -o "${user}" -g "${user}" -m 0644 "${CERT_DIR}/fullchain.pem" "${dest_dir}/redgres-fullchain.pem"
  /usr/bin/install -o "${user}" -g "${user}" -m 0600 "${CERT_DIR}/privkey.pem" "${dest_dir}/redgres-privkey.pem"
  redgres_tls_readable_as "${user}" "${dest_dir}/redgres-privkey.pem"
}

pgb_chain="${CERT_DIR}/fullchain.pem"
pgb_key="${CERT_DIR}/privkey.pem"
if [[ -d /etc/pgbouncer ]]; then
  pgb_user=pgbouncer
  if ! /usr/bin/getent passwd pgbouncer >/dev/null 2>&1; then
    pgb_user=postgres
  fi
  if /usr/bin/getent passwd "${pgb_user}" >/dev/null 2>&1; then
    redgres_tls_install_owned "${pgb_user}" /etc/pgbouncer
    pgb_chain=/etc/pgbouncer/redgres-fullchain.pem
    pgb_key=/etc/pgbouncer/redgres-privkey.pem
  fi
fi
if [[ -f "${PGBOUNCER_INI}" ]]; then
  tmp="$(/usr/bin/mktemp "${PGBOUNCER_INI}.XXXXXX")"
  /usr/bin/awk -v c="${pgb_chain}" -v k="${pgb_key}" '
    /^client_tls_cert_file[[:space:]]*=/ { print "client_tls_cert_file = " c; next }
    /^client_tls_key_file[[:space:]]*=/ { print "client_tls_key_file = " k; next }
    { print }
  ' "${PGBOUNCER_INI}" >"${tmp}"
  if ! grep -q '^client_tls_cert_file[[:space:]]*=' "${tmp}"; then
    printf '%s\n' "client_tls_cert_file = ${pgb_chain}" "client_tls_key_file = ${pgb_key}" >>"${tmp}"
  fi
  /usr/bin/chmod --reference="${PGBOUNCER_INI}" "${tmp}" 2>/dev/null || /usr/bin/chmod 640 "${tmp}"
  /usr/bin/mv "${tmp}" "${PGBOUNCER_INI}"
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet pgbouncer; then
    systemctl reload pgbouncer >/dev/null 2>&1 || systemctl restart pgbouncer >/dev/null 2>&1
  fi
fi

maindir=""
for maindir in /etc/postgresql/*/main; do
  [[ -d "${maindir}/conf.d" ]] || continue
  pg_chain="${CERT_DIR}/fullchain.pem"
  pg_key="${CERT_DIR}/privkey.pem"
  if /usr/bin/getent passwd postgres >/dev/null 2>&1; then
    redgres_tls_install_owned postgres "${maindir}"
    pg_chain="${maindir}/redgres-fullchain.pem"
    pg_key="${maindir}/redgres-privkey.pem"
  fi
  printf '%s\n' "ssl = on" "ssl_cert_file = '${pg_chain}'" "ssl_key_file = '${pg_key}'" >"${maindir}/conf.d/redgres-ssl.conf"
  /usr/bin/chmod 644 "${maindir}/conf.d/redgres-ssl.conf"
  ver="$(printf '%s' "${maindir}" | /usr/bin/awk -F/ '{print $4}')"
  if [[ -n "${ver}" ]] && command -v pg_ctlcluster >/dev/null 2>&1; then
    pg_ctlcluster "${ver}" main reload >/dev/null 2>&1 || exit 1
  fi
done
exit 0
