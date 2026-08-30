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
TARGETS_FILE="${REDGRES_TLS_TARGETS_FILE:-/etc/redgres/tls-targets}"
LINEAGE_STATE="${REDGRES_TLS_LINEAGE_FILE:-/etc/redgres/tls-lineage}"
POSTGRES_ROOT="${REDGRES_POSTGRES_CONFIG_ROOT:-/etc/postgresql}"
OPENSSL="${REDGRES_OPENSSL_BIN:-openssl}"
TLS_POSTGRES_CLUSTER=""
TLS_PGBOUNCER=0
TLS_PGBOUNCER_USER=""

[[ -f "${LINEAGE_STATE}" && ! -L "${LINEAGE_STATE}" ]] || exit 0
if [[ "${LINEAGE_STATE}" == "/etc/redgres/tls-lineage" ]]; then
  [[ "$(/usr/bin/stat -c '%U:%G:%a' "${LINEAGE_STATE}")" == "root:root:600" ]] || exit 1
fi
EXPECTED_LINEAGE="$(/usr/bin/head -n1 "${LINEAGE_STATE}")"
[[ -n "${EXPECTED_LINEAGE}" && "${LINEAGE}" == "${EXPECTED_LINEAGE}" ]] || exit 0

redgres_tls_load_targets() {
  local key value metadata
  [[ -f "${TARGETS_FILE}" && ! -L "${TARGETS_FILE}" ]] || return 0
  if [[ "${TARGETS_FILE}" == "/etc/redgres/tls-targets" ]]; then
    metadata="$(/usr/bin/stat -c '%U:%G:%a' "${TARGETS_FILE}")" || return 1
    [[ "${metadata}" == "root:root:600" ]] || return 1
  fi
  while IFS='=' read -r key value; do
    case "${key}" in
      postgres_cluster)
        [[ "${value}" =~ ^[0-9]+/[a-z0-9_-]+$ ]] || return 1
        TLS_POSTGRES_CLUSTER="${value}"
        ;;
      pgbouncer)
        [[ "${value}" == "0" || "${value}" == "1" ]] || return 1
        TLS_PGBOUNCER="${value}"
        ;;
      pgbouncer_user)
        [[ "${value}" == "postgres" || "${value}" == "pgbouncer" ]] || return 1
        TLS_PGBOUNCER_USER="${value}"
        ;;
      ''|'#'*) ;;
      *) return 1 ;;
    esac
  done <"${TARGETS_FILE}"
  [[ "${TLS_PGBOUNCER}" == "0" || -n "${TLS_PGBOUNCER_USER}" ]] || return 1
}

redgres_tls_load_targets
if [[ "${TLS_PGBOUNCER}" == "1" ]]; then
  [[ -d /etc/pgbouncer && -f "${PGBOUNCER_INI}" && ! -L "${PGBOUNCER_INI}" ]] || exit 1
fi
/usr/bin/mkdir -p "${CERT_DIR}"
/usr/bin/chmod 0755 "${CERT_DIR}"

redgres_tls_readable_as() {
  local user="$1" dest_key="$2"
  /usr/bin/getent passwd "${user}" >/dev/null 2>&1 || return 1
  DEST_KEY="${dest_key}" su -s /bin/sh "${user}" -c '/usr/bin/test -r "$DEST_KEY"'
}

redgres_tls_atomic_install() {
  local src="$1" dest="$2" mode="$3" owner="${4:-}" tmp
  tmp="$(/usr/bin/mktemp "${dest}.XXXXXX")" || return 1
  if ! /usr/bin/install -m "${mode}" "${src}" "${tmp}"; then
    rm -f "${tmp}"
    return 1
  fi
  if [[ -n "${owner}" ]] && ! /usr/bin/chown "${owner}:${owner}" "${tmp}"; then
    rm -f "${tmp}"
    return 1
  fi
  /usr/bin/mv -fT -- "${tmp}" "${dest}"
}

TLS_TXN_DIR=""
TLS_TXN_COMMITTED=0
TLS_TXN_DESTS=()
TLS_TXN_BACKUPS=()

redgres_tls_txn_backup() {
  local dest="$1" index backup fail_after=0
  [[ ! -L "${dest}" ]] || return 1
  index="${#TLS_TXN_DESTS[@]}"
  backup="${TLS_TXN_DIR}/${index}"
  if [[ "${TARGETS_FILE}" != "/etc/redgres/tls-targets" ]]; then
    fail_after="${REDGRES_TLS_TXN_BACKUP_FAIL_AFTER:-0}"
    [[ "${fail_after}" =~ ^[0-9]+$ ]] || fail_after=0
    [[ "${fail_after}" -eq 0 || "${index}" -ne "${fail_after}" ]] || return 1
  fi
  if [[ -e "${dest}" ]]; then
    [[ -f "${dest}" ]] || return 1
    /usr/bin/cp --preserve=mode,ownership,timestamps -- "${dest}" "${backup}" || return 1
  else
    backup=""
  fi
  # Keep the arrays aligned: no fallible operation may occur between appends.
  TLS_TXN_DESTS+=("${dest}")
  TLS_TXN_BACKUPS+=("${backup}")
}

redgres_tls_txn_restore() {
  local i dest backup tmp
  for ((i=${#TLS_TXN_DESTS[@]}-1; i>=0; i--)); do
    dest="${TLS_TXN_DESTS[$i]}"
    backup="${TLS_TXN_BACKUPS[$i]}"
    if [[ -n "${backup}" ]]; then
      tmp="$(/usr/bin/mktemp "${dest}.restore.XXXXXX")" || continue
      if /usr/bin/cp --preserve=mode,ownership,timestamps -- "${backup}" "${tmp}"; then
        /usr/bin/mv -fT -- "${tmp}" "${dest}" || rm -f -- "${tmp}"
      else
        rm -f -- "${tmp}"
      fi
    else
      rm -f -- "${dest}"
    fi
  done
}

redgres_tls_txn_exit() {
  local rc=$? version cluster
  if [[ "${TLS_TXN_COMMITTED}" -ne 1 && -n "${TLS_TXN_DIR}" ]]; then
    redgres_tls_txn_restore
    if [[ "${TLS_PGBOUNCER}" == "1" ]] && command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet pgbouncer; then
      systemctl reload pgbouncer >/dev/null 2>&1 || true
    fi
    if [[ -n "${TLS_POSTGRES_CLUSTER}" ]] && command -v pg_ctlcluster >/dev/null 2>&1; then
      version="${TLS_POSTGRES_CLUSTER%%/*}"
      cluster="${TLS_POSTGRES_CLUSTER#*/}"
      pg_ctlcluster "${version}" "${cluster}" reload >/dev/null 2>&1 || true
    fi
  fi
  [[ -z "${TLS_TXN_DIR}" ]] || rm -rf -- "${TLS_TXN_DIR}"
  return "${rc}"
}

redgres_tls_pair_matches() {
  local cert_hash key_hash
  cert_hash="$("${OPENSSL}" x509 -in "${LINEAGE}/fullchain.pem" -pubkey -noout 2>/dev/null | "${OPENSSL}" pkey -pubin -outform DER 2>/dev/null | /usr/bin/sha256sum)" || return 1
  key_hash="$("${OPENSSL}" pkey -in "${LINEAGE}/privkey.pem" -pubout -outform DER 2>/dev/null | /usr/bin/sha256sum)" || return 1
  [[ -n "${cert_hash}" && "${cert_hash}" == "${key_hash}" ]]
}

redgres_tls_verify_served_pair() {
  local port="$1" served expected_fp served_fp
  [[ "${TARGETS_FILE}" == "/etc/redgres/tls-targets" ]] || return 0
  served="$(/usr/bin/mktemp "${CERT_DIR}/.served-cert.XXXXXX")" || return 1
  if ! /usr/bin/timeout 15 "${OPENSSL}" s_client -starttls postgres -connect "127.0.0.1:${port}" -showcerts </dev/null 2>/dev/null |
      /usr/bin/awk '/-----BEGIN CERTIFICATE-----/ {capture=1} capture {print} /-----END CERTIFICATE-----/ {exit}' >"${served}"; then
    rm -f -- "${served}"
    return 1
  fi
  [[ -s "${served}" ]] || { rm -f -- "${served}"; return 1; }
  expected_fp="$("${OPENSSL}" x509 -in "${LINEAGE}/fullchain.pem" -noout -sha256 -fingerprint 2>/dev/null)" || { rm -f -- "${served}"; return 1; }
  served_fp="$("${OPENSSL}" x509 -in "${served}" -noout -sha256 -fingerprint 2>/dev/null)" || { rm -f -- "${served}"; return 1; }
  rm -f -- "${served}"
  [[ -n "${expected_fp}" && "${expected_fp}" == "${served_fp}" ]]
}

redgres_tls_install_pair() {
  local src_chain="$1" src_key="$2" dest_chain="$3" dest_key="$4" chain_mode="$5" key_mode="$6" owner="${7:-}"
  redgres_tls_txn_backup "${dest_chain}"
  redgres_tls_txn_backup "${dest_key}"
  redgres_tls_atomic_install "${src_chain}" "${dest_chain}" "${chain_mode}" "${owner}"
  redgres_tls_atomic_install "${src_key}" "${dest_key}" "${key_mode}" "${owner}"
}

redgres_tls_pair_matches
TLS_TXN_DIR="$(/usr/bin/mktemp -d "${CERT_DIR}/.renew-copy.XXXXXX")"
trap redgres_tls_txn_exit EXIT

redgres_tls_install_pair "${LINEAGE}/fullchain.pem" "${LINEAGE}/privkey.pem" "${CERT_DIR}/fullchain.pem" "${CERT_DIR}/privkey.pem" 0644 0640

redgres_tls_install_owned() {
  local user="$1" dest_dir="$2"
  redgres_tls_install_pair "${CERT_DIR}/fullchain.pem" "${CERT_DIR}/privkey.pem" "${dest_dir}/redgres-fullchain.pem" "${dest_dir}/redgres-privkey.pem" 0644 0600 "${user}"
  redgres_tls_readable_as "${user}" "${dest_dir}/redgres-privkey.pem"
}

pgb_chain="${CERT_DIR}/fullchain.pem"
pgb_key="${CERT_DIR}/privkey.pem"
if [[ "${TLS_PGBOUNCER}" == "1" && -d /etc/pgbouncer ]]; then
  pgb_user="${TLS_PGBOUNCER_USER}"
  if /usr/bin/getent passwd "${pgb_user}" >/dev/null 2>&1; then
    redgres_tls_install_owned "${pgb_user}" /etc/pgbouncer
    pgb_chain=/etc/pgbouncer/redgres-fullchain.pem
    pgb_key=/etc/pgbouncer/redgres-privkey.pem
  fi
fi
if [[ "${TLS_PGBOUNCER}" == "1" && -f "${PGBOUNCER_INI}" && ! -L "${PGBOUNCER_INI}" ]]; then
  grep -q '^client_tls_cert_file[[:space:]]*=' "${PGBOUNCER_INI}" || exit 1
  grep -q '^client_tls_key_file[[:space:]]*=' "${PGBOUNCER_INI}" || exit 1
  tmp="$(/usr/bin/mktemp "${PGBOUNCER_INI}.XXXXXX")"
  redgres_tls_txn_backup "${PGBOUNCER_INI}"
  /usr/bin/awk -v c="${pgb_chain}" -v k="${pgb_key}" '
    /^client_tls_cert_file[[:space:]]*=/ { print "client_tls_cert_file = " c; next }
    /^client_tls_key_file[[:space:]]*=/ { print "client_tls_key_file = " k; next }
    { print }
  ' "${PGBOUNCER_INI}" >"${tmp}"
  /usr/bin/chmod --reference="${PGBOUNCER_INI}" "${tmp}" 2>/dev/null || /usr/bin/chmod 640 "${tmp}"
  /usr/bin/chown --reference="${PGBOUNCER_INI}" "${tmp}" 2>/dev/null
  /usr/bin/mv -fT "${tmp}" "${PGBOUNCER_INI}"
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet pgbouncer; then
    if ! systemctl reload pgbouncer >/dev/null 2>&1; then
      exit 1
    fi
  fi
  redgres_tls_verify_served_pair 6432
fi

if [[ -n "${TLS_POSTGRES_CLUSTER}" ]]; then
  version="${TLS_POSTGRES_CLUSTER%%/*}"
  cluster="${TLS_POSTGRES_CLUSTER#*/}"
  maindir="${POSTGRES_ROOT}/${version}/${cluster}"
  [[ -f "${maindir}/conf.d/redgres-ssl.conf" && ! -L "${maindir}/conf.d/redgres-ssl.conf" ]] || exit 1
  pg_chain="${CERT_DIR}/fullchain.pem"
  pg_key="${CERT_DIR}/privkey.pem"
  if /usr/bin/getent passwd postgres >/dev/null 2>&1; then
    redgres_tls_install_owned postgres "${maindir}"
    pg_chain="${maindir}/redgres-fullchain.pem"
    pg_key="${maindir}/redgres-privkey.pem"
  fi
  snippet="${maindir}/conf.d/redgres-ssl.conf"
  redgres_tls_txn_backup "${snippet}"
  next="$(/usr/bin/mktemp "${snippet}.next.XXXXXX")"
  printf '%s\n' "ssl = on" "ssl_cert_file = '${pg_chain}'" "ssl_key_file = '${pg_key}'" >"${next}"
  /usr/bin/chmod --reference="${snippet}" "${next}" 2>/dev/null || /usr/bin/chmod 644 "${next}"
  /usr/bin/chown --reference="${snippet}" "${next}" 2>/dev/null
  /usr/bin/mv -fT "${next}" "${snippet}"
  if command -v pg_ctlcluster >/dev/null 2>&1; then
    if ! pg_ctlcluster "${version}" "${cluster}" reload >/dev/null 2>&1; then
      exit 1
    fi
  fi
  redgres_tls_verify_served_pair 5432
fi
TLS_TXN_COMMITTED=1
exit 0
