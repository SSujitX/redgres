#!/bin/bash
# Privileged Let's Encrypt DNS-01 for grey-cloud db/rs (OPS-009).
# Reads hostnames from the request file. Never prints credential file contents.
set -euo pipefail
umask 077
PATH=/usr/sbin:/usr/bin:/sbin:/bin

REQUEST="${REDGRES_TLS_ISSUE_REQUEST_FILE:-/var/lib/redgres/tls-issue.request}"
RESULT="${REDGRES_TLS_ISSUE_RESULT_FILE:-/var/lib/redgres-tls/issue.result}"
ACTIVE_REQUEST="${REDGRES_TLS_ACTIVE_REQUEST_FILE:-$(/usr/bin/dirname -- "${RESULT}")/active.request}"
CLEANUP_RESULT="${REDGRES_TLS_CLEANUP_RESULT_FILE:-$(/usr/bin/dirname -- "${RESULT}")/cleanup.result}"
CREDS="${REDGRES_CERTBOT_DNS_TOKEN_FILE:-/var/lib/redgres/secrets/certbot-dns.ini}"
LIVE_ROOT="${REDGRES_CERT_LIVE_DIR:-/etc/letsencrypt/live}"
# postgres/pgbouncer are not in group redgres; /etc/redgres is 0750 and unreadable to them.
CERT_DIR="${REDGRES_TLS_CERT_DIR:-/etc/ssl/redgres}"
CERTBOT="${REDGRES_CERTBOT_BIN:-certbot}"
OPENSSL="${REDGRES_OPENSSL_BIN:-openssl}"
PGBOUNCER_INI="${REDGRES_PGBOUNCER_INI:-/etc/pgbouncer/pgbouncer.ini}"
MIN_VALID_SECONDS="${REDGRES_TLS_MIN_VALID_SECONDS:-604800}"
TARGETS_FILE="${REDGRES_TLS_TARGETS_FILE:-/etc/redgres/tls-targets}"
LINEAGE_STATE="${REDGRES_TLS_LINEAGE_FILE:-/etc/redgres/tls-lineage}"
POSTGRES_ROOT="${REDGRES_POSTGRES_CONFIG_ROOT:-/etc/postgresql}"
TLS_POSTGRES_CLUSTER=""
TLS_PGBOUNCER=0
TLS_PGBOUNCER_USER=""
hosts=()
CERTBOT_CREDS="${CREDS}"
CERTBOT_CREDS_SNAPSHOT=""
certbot_output=""

redgres_tls_cleanup() {
  [[ -z "${certbot_output}" ]] || rm -f -- "${certbot_output}"
}
trap redgres_tls_cleanup EXIT

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

redgres_tls_record_lineage() {
  local lineage="$1" dir tmp
  [[ -d "${lineage}" && ! -L "${lineage}" ]] || return 1
  dir="$(/usr/bin/dirname -- "${LINEAGE_STATE}")"
  [[ -d "${dir}" ]] || return 1
  tmp="$(/usr/bin/mktemp "${dir}/.tls-lineage.XXXXXX")" || return 1
  printf '%s\n' "${lineage}" >"${tmp}"
  /usr/bin/chown root:root "${tmp}" 2>/dev/null || true
  /usr/bin/chmod 0600 "${tmp}"
  /usr/bin/mv -fT -- "${tmp}" "${LINEAGE_STATE}"
}

redgres_tls_snapshot_credentials() {
  [[ "${CREDS}" == "/var/lib/redgres/secrets/certbot-dns.ini" ]] || return 0
  CERTBOT_CREDS_SNAPSHOT="$(/usr/bin/dirname -- "${RESULT}")/certbot-dns.ini"
  /usr/bin/python3 - "${CERTBOT_CREDS_SNAPSHOT}" <<'PY'
import os, re, secrets, stat, sys

out_path = sys.argv[1]
nofollow = getattr(os, "O_NOFOLLOW", 0)
directory = getattr(os, "O_DIRECTORY", 0)
root_fd = os.open("/var/lib/redgres", os.O_RDONLY | directory | nofollow)
try:
    secrets_fd = os.open("secrets", os.O_RDONLY | directory | nofollow, dir_fd=root_fd)
    try:
        cred_fd = os.open("certbot-dns.ini", os.O_RDONLY | nofollow, dir_fd=secrets_fd)
        try:
            info = os.fstat(cred_fd)
            if not stat.S_ISREG(info.st_mode) or info.st_size > 4096:
                raise OSError("untrusted credentials file")
            data = os.read(cred_fd, 4097)
        finally:
            os.close(cred_fd)
    finally:
        os.close(secrets_fd)
finally:
    os.close(root_fd)

text = data.decode("utf-8")
if not re.fullmatch(r"dns_cloudflare_api_token = [^\s=]+\n", text):
    raise OSError("invalid credentials format")
out_dir, out_name = os.path.split(out_path)
dir_fd = os.open(out_dir, os.O_RDONLY | directory | nofollow)
tmp_name = f".certbot-dns.{secrets.token_hex(12)}"
try:
    fd = os.open(tmp_name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | nofollow, 0o600, dir_fd=dir_fd)
    try:
        os.write(fd, data)
        os.fsync(fd)
    finally:
        os.close(fd)
    os.replace(tmp_name, out_name, src_dir_fd=dir_fd, dst_dir_fd=dir_fd)
finally:
    try:
        os.unlink(tmp_name, dir_fd=dir_fd)
    except FileNotFoundError:
        pass
    os.close(dir_fd)
PY
  CERTBOT_CREDS="${CERTBOT_CREDS_SNAPSHOT}"
}

redgres_tls_cleanup_domain() {
  local state_dir
  state_dir="$(/usr/bin/dirname -- "${RESULT}")"
  # Disconnect removes the copied DNS authorization only. The complete Certbot
  # lineage and renewal configuration remain intact for exact-set reuse after a
  # reconnect; without the credential snapshot, unattended renewal cannot place
  # a DNS challenge. Full uninstall owns destructive lineage deletion.
  rm -f -- "${state_dir}/certbot-dns.ini" "${ACTIVE_REQUEST}"
}

redgres_tls_active_trusted() {
  local metadata
  [[ -f "${ACTIVE_REQUEST}" && ! -L "${ACTIVE_REQUEST}" ]] || return 1
  if [[ "${ACTIVE_REQUEST}" == "/var/lib/redgres-tls/active.request" ]]; then
    metadata="$(/usr/bin/stat -c '%U:%G:%a' "${ACTIVE_REQUEST}")" || return 1
    [[ "${metadata}" == "root:root:600" ]] || return 1
  fi
}

redgres_tls_claim_request() {
  if [[ "${REQUEST}" != "/var/lib/redgres/tls-issue.request" || "${ACTIVE_REQUEST}" != "/var/lib/redgres-tls/active.request" ]]; then
    /usr/bin/mv -fT -- "${REQUEST}" "${ACTIVE_REQUEST}" || return 1
    return 0
  fi
  /usr/bin/python3 <<'PY'
import os, secrets, stat

nofollow = getattr(os, "O_NOFOLLOW", 0)
directory = getattr(os, "O_DIRECTORY", 0)
source_dir = os.open("/var/lib/redgres", os.O_RDONLY | directory | nofollow)
state_dir = os.open("/var/lib/redgres-tls", os.O_RDONLY | directory | nofollow)
tmp_name = f".active-request.{secrets.token_hex(12)}"
before = None
try:
    try:
        source = os.open("tls-issue.request", os.O_RDONLY | nofollow, dir_fd=source_dir)
        try:
            before = os.fstat(source)
            if not stat.S_ISREG(before.st_mode) or before.st_size > 1024:
                raise OSError("untrusted request")
            data = os.read(source, 1025)
            if len(data) > 1024:
                raise OSError("request too large")
        finally:
            os.close(source)
        target = os.open(tmp_name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | nofollow, 0o600, dir_fd=state_dir)
        try:
            os.write(target, data)
            os.fsync(target)
        finally:
            os.close(target)
        os.replace(tmp_name, "active.request", src_dir_fd=state_dir, dst_dir_fd=state_dir)
    finally:
        if before is not None:
            try:
                current = os.stat("tls-issue.request", dir_fd=source_dir, follow_symlinks=False)
                if (current.st_dev, current.st_ino) == (before.st_dev, before.st_ino):
                    os.unlink("tls-issue.request", dir_fd=source_dir)
            except FileNotFoundError:
                pass
finally:
    try:
        os.unlink(tmp_name, dir_fd=state_dir)
    except FileNotFoundError:
        pass
    os.close(state_dir)
    os.close(source_dir)
PY
}

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
TLS_TXN_DESTS=()
TLS_TXN_BACKUPS=()

redgres_tls_txn_begin() {
  TLS_TXN_DIR="$(/usr/bin/mktemp -d "$(/usr/bin/dirname -- "${RESULT}")/.tls-copy.XXXXXX")" || return 1
}

redgres_tls_txn_backup() {
  local dest="$1" index backup fail_after=0
  [[ ! -L "${dest}" ]] || return 1
  index="${#TLS_TXN_DESTS[@]}"
  backup="${TLS_TXN_DIR}/${index}"
  if [[ "${RESULT}" != "/var/lib/redgres-tls/issue.result" ]]; then
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

redgres_tls_txn_finish() {
  [[ -z "${TLS_TXN_DIR}" ]] || rm -rf -- "${TLS_TXN_DIR}"
  TLS_TXN_DIR=""
  TLS_TXN_DESTS=()
  TLS_TXN_BACKUPS=()
}

redgres_tls_pair_matches() {
  local chain="$1" key="$2" cert_hash key_hash
  cert_hash="$("${OPENSSL}" x509 -in "${chain}" -pubkey -noout 2>/dev/null | "${OPENSSL}" pkey -pubin -outform DER 2>/dev/null | /usr/bin/sha256sum)" || return 1
  key_hash="$("${OPENSSL}" pkey -in "${key}" -pubout -outform DER 2>/dev/null | /usr/bin/sha256sum)" || return 1
  [[ -n "${cert_hash}" && "${cert_hash}" == "${key_hash}" ]]
}

redgres_tls_verify_served_pair() {
  local port="$1" expected_chain="$2" served expected_fp served_fp
  # Test-only path overrides do not touch live services. Production must prove
  # that the reloaded endpoint actually presents the selected leaf certificate.
  [[ "${RESULT}" == "/var/lib/redgres-tls/issue.result" ]] || return 0
  served="$(/usr/bin/mktemp "$(/usr/bin/dirname -- "${RESULT}")/.served-cert.XXXXXX")" || return 1
  if ! /usr/bin/timeout 15 "${OPENSSL}" s_client -starttls postgres -connect "127.0.0.1:${port}" -showcerts </dev/null 2>/dev/null |
      /usr/bin/awk '/-----BEGIN CERTIFICATE-----/ {capture=1} capture {print} /-----END CERTIFICATE-----/ {exit}' >"${served}"; then
    rm -f -- "${served}"
    return 1
  fi
  [[ -s "${served}" ]] || { rm -f -- "${served}"; return 1; }
  expected_fp="$("${OPENSSL}" x509 -in "${expected_chain}" -noout -sha256 -fingerprint 2>/dev/null)" || { rm -f -- "${served}"; return 1; }
  served_fp="$("${OPENSSL}" x509 -in "${served}" -noout -sha256 -fingerprint 2>/dev/null)" || { rm -f -- "${served}"; return 1; }
  rm -f -- "${served}"
  [[ -n "${expected_fp}" && "${expected_fp}" == "${served_fp}" ]]
}

redgres_tls_install_pair() {
  local src_chain="$1" src_key="$2" dest_chain="$3" dest_key="$4" chain_mode="$5" key_mode="$6" owner="${7:-}"
  redgres_tls_txn_backup "${dest_chain}" || return 1
  redgres_tls_txn_backup "${dest_key}" || return 1
  redgres_tls_atomic_install "${src_chain}" "${dest_chain}" "${chain_mode}" "${owner}" || return 1
  redgres_tls_atomic_install "${src_key}" "${dest_key}" "${key_mode}" "${owner}" || return 1
}

redgres_tls_install_owned() {
  local user="$1" dest_dir="$2" src_chain="$3" src_key="$4"
  redgres_tls_install_pair "${src_chain}" "${src_key}" "${dest_dir}/redgres-fullchain.pem" "${dest_dir}/redgres-privkey.pem" 0644 0600 "${user}" || return 1
  redgres_tls_readable_as "${user}" "${dest_dir}/redgres-privkey.pem"
}

redgres_tls_apply_pgbouncer() {
  local dest_chain="$1" dest_key="$2" tmp
  [[ "${TLS_PGBOUNCER}" == "1" ]] || return 0
  [[ -f "${PGBOUNCER_INI}" && ! -L "${PGBOUNCER_INI}" ]] || return 1
  grep -q '^client_tls_cert_file[[:space:]]*=' "${PGBOUNCER_INI}" || return 1
  grep -q '^client_tls_key_file[[:space:]]*=' "${PGBOUNCER_INI}" || return 1
  tmp="$(/usr/bin/mktemp "${PGBOUNCER_INI}.XXXXXX")"
  redgres_tls_txn_backup "${PGBOUNCER_INI}" || return 1
  /usr/bin/awk -v c="${dest_chain}" -v k="${dest_key}" '
    /^client_tls_cert_file[[:space:]]*=/ { print "client_tls_cert_file = " c; next }
    /^client_tls_key_file[[:space:]]*=/ { print "client_tls_key_file = " k; next }
    { print }
  ' "${PGBOUNCER_INI}" >"${tmp}"
  /usr/bin/chmod --reference="${PGBOUNCER_INI}" "${tmp}" 2>/dev/null || /usr/bin/chmod 640 "${tmp}"
  if ! /usr/bin/chown --reference="${PGBOUNCER_INI}" "${tmp}" 2>/dev/null; then
    rm -f -- "${tmp}"
    return 1
  fi
  /usr/bin/mv -fT "${tmp}" "${PGBOUNCER_INI}"
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet pgbouncer; then
    if ! systemctl reload pgbouncer >/dev/null 2>&1; then
      return 1
    fi
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

redgres_tls_exact_sans() {
  local chain="$1" requested host san_text entry
  shift
  local -a cert_hosts=() request_hosts=() san_entries=()
  san_text="$("${OPENSSL}" x509 -in "${chain}" -noout -ext subjectAltName 2>/dev/null)" || return 1
  [[ "${san_text}" == *'Subject Alternative Name:'* ]] || return 1
  san_text="${san_text#*Subject Alternative Name:}"
  san_text="$(printf '%s' "${san_text}" | /usr/bin/tr '\r\n' ' ' | /usr/bin/sed -E 's/^[[:space:]]*critical[[:space:]]*//')"
  mapfile -t san_entries < <(printf '%s' "${san_text}" | /usr/bin/tr ',' '\n' | /usr/bin/sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//; /^$/d')
  [[ "${#san_entries[@]}" -gt 0 ]] || return 1
  for entry in "${san_entries[@]}"; do
    [[ "${entry}" == DNS:* ]] || return 1
    cert_hosts+=("$(printf '%s' "${entry#DNS:}" | /usr/bin/tr '[:upper:]' '[:lower:]')")
  done
  mapfile -t cert_hosts < <(printf '%s\n' "${cert_hosts[@]}" | /usr/bin/sort -u)
  for requested in "$@"; do
    request_hosts+=("$(printf '%s' "${requested}" | /usr/bin/tr '[:upper:]' '[:lower:]')")
  done
  mapfile -t request_hosts < <(printf '%s\n' "${request_hosts[@]}" | /usr/bin/sort -u)
  [[ "${#cert_hosts[@]}" -eq "${#request_hosts[@]}" ]] || return 1
  for host in "${!request_hosts[@]}"; do
    [[ "${cert_hosts[$host]}" == "${request_hosts[$host]}" ]] || return 1
  done
}

redgres_tls_copy_certs_inner() {
  local live="$1" dest_chain dest_key
  [[ -f "${live}/fullchain.pem" && -f "${live}/privkey.pem" ]] || return 1
  redgres_tls_pair_matches "${live}/fullchain.pem" "${live}/privkey.pem" || return 1
  redgres_tls_prepare_dest "${CERT_DIR}"
  dest_chain="${CERT_DIR}/fullchain.pem"
  dest_key="${CERT_DIR}/privkey.pem"
  redgres_tls_install_pair "${live}/fullchain.pem" "${live}/privkey.pem" "${dest_chain}" "${dest_key}" 0644 0640 || return 1
  local pgb_chain="${dest_chain}" pgb_key="${dest_key}"
  if [[ "${TLS_PGBOUNCER}" == "1" ]]; then
    if [[ -d /etc/pgbouncer ]]; then
      local pgb_user="${TLS_PGBOUNCER_USER}"
      if /usr/bin/getent passwd "${pgb_user}" >/dev/null 2>&1; then
        redgres_tls_install_owned "${pgb_user}" /etc/pgbouncer "${dest_chain}" "${dest_key}" || return 1
        pgb_chain=/etc/pgbouncer/redgres-fullchain.pem
        pgb_key=/etc/pgbouncer/redgres-privkey.pem
      fi
    fi
    redgres_tls_apply_pgbouncer "${pgb_chain}" "${pgb_key}" || return 1
    redgres_tls_verify_served_pair 6432 "${dest_chain}" || return 1
  fi
  if [[ -z "${TLS_POSTGRES_CLUSTER}" ]]; then
    return 0
  fi
  local version cluster maindir snippet next pg_chain pg_key
  version="${TLS_POSTGRES_CLUSTER%%/*}"
  cluster="${TLS_POSTGRES_CLUSTER#*/}"
  maindir="${POSTGRES_ROOT}/${version}/${cluster}"
  snippet="${maindir}/conf.d/redgres-ssl.conf"
  [[ -f "${snippet}" && ! -L "${snippet}" ]] || return 1
  pg_chain="${maindir}/redgres-fullchain.pem"
  pg_key="${maindir}/redgres-privkey.pem"
  redgres_tls_install_owned postgres "${maindir}" "${dest_chain}" "${dest_key}" || return 1
  redgres_tls_txn_backup "${snippet}" || return 1
  next="$(/usr/bin/mktemp "${snippet}.next.XXXXXX")"
  printf '%s\n' "ssl = on" "ssl_cert_file = '${pg_chain}'" "ssl_key_file = '${pg_key}'" >"${next}"
  /usr/bin/chmod --reference="${snippet}" "${next}" 2>/dev/null || /usr/bin/chmod 644 "${next}"
  if ! /usr/bin/chown --reference="${snippet}" "${next}" 2>/dev/null; then
    rm -f -- "${next}"
    return 1
  fi
  /usr/bin/mv -fT "${next}" "${snippet}"
  if command -v pg_ctlcluster >/dev/null 2>&1 && ! pg_ctlcluster "${version}" "${cluster}" reload >/dev/null 2>&1; then
    return 1
  fi
  redgres_tls_verify_served_pair 5432 "${dest_chain}" || return 1
}

redgres_tls_copy_certs() {
  local live="$1" rc=0 version cluster
  redgres_tls_txn_begin || return 1
  redgres_tls_copy_certs_inner "${live}" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then
    redgres_tls_txn_restore
    if [[ "${TLS_PGBOUNCER}" == "1" ]] && command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet pgbouncer; then
      systemctl reload pgbouncer >/dev/null 2>&1 || true
    fi
    if [[ -n "${TLS_POSTGRES_CLUSTER}" ]] && command -v pg_ctlcluster >/dev/null 2>&1; then
      version="${TLS_POSTGRES_CLUSTER%%/*}"
      cluster="${TLS_POSTGRES_CLUSTER#*/}"
      pg_ctlcluster "${version}" "${cluster}" reload >/dev/null 2>&1 || true
    fi
    redgres_tls_txn_finish
    return "${rc}"
  fi
  redgres_tls_txn_finish
}

MATCHED_LIVE=""
MATCHED_RENEWAL_NAME=""
redgres_tls_find_existing() {
  local primary="$1" candidate host chain key covers
  shift
  MATCHED_LIVE=""
  MATCHED_RENEWAL_NAME=""
  shopt -s nullglob
  for candidate in "${LIVE_ROOT}/${primary}" "${LIVE_ROOT}/${primary}-"*; do
    [[ -d "${candidate}" && ! -L "${candidate}" ]] || continue
    chain="${candidate}/fullchain.pem"
    key="${candidate}/privkey.pem"
    [[ -f "${chain}" && -f "${key}" ]] || continue
    redgres_tls_pair_matches "${chain}" "${key}" || continue
    redgres_tls_exact_sans "${chain}" "$@" || continue
    covers=1
    for host in "$@"; do
      if ! "${OPENSSL}" x509 -in "${chain}" -checkhost "${host}" -noout >/dev/null 2>&1; then
        covers=0
        break
      fi
    done
    if [[ "${covers}" -eq 1 ]]; then
      if "${OPENSSL}" x509 -in "${chain}" -checkend "${MIN_VALID_SECONDS}" -noout >/dev/null 2>&1; then
        MATCHED_LIVE="${candidate}"
        shopt -u nullglob
        return 0
      fi
      [[ -n "${MATCHED_RENEWAL_NAME}" ]] || MATCHED_RENEWAL_NAME="${candidate##*/}"
    fi
  done
  shopt -u nullglob
  return 1
}

FAIL_REASON=dependency
RETRY_AFTER=""
redgres_tls_classify_certbot_failure() {
  local output="$1" retry_raw=""
  FAIL_REASON=dependency
  RETRY_AFTER=""
  if grep -Eqi 'too many certificates|rate limit' "${output}"; then
    FAIL_REASON=rate_limited
    retry_raw="$(sed -nE 's/.*retry after ([0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2} UTC).*/\1/p' "${output}" | head -n1)"
    if [[ -n "${retry_raw}" ]]; then
      RETRY_AFTER="$(date -u -d "${retry_raw}" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || true)"
    fi
  elif grep -Eqi 'another instance of certbot|lock file.*certbot' "${output}"; then
    FAIL_REASON=busy
  elif grep -Eqi 'unauthorized|invalid credentials|api token|zone_id' "${output}"; then
    FAIL_REASON=credentials
  elif grep -Eqi 'dns problem|nxdomain|txt record|dns challenge|propagation' "${output}"; then
    FAIL_REASON=dns
  fi
}

redgres_tls_rate_limit_active() {
  local first reason retry retry_epoch now host matched cooldown_host
  local cooldown_hosts=()
  [[ -f "${RESULT}" && ! -L "${RESULT}" ]] || return 1
  first="$(/usr/bin/head -n1 "${RESULT}")"
  [[ "${first}" == "failed" ]] || return 1
  reason="$(/usr/bin/sed -n 's/^reason=//p' "${RESULT}" | /usr/bin/head -n1)"
  [[ "${reason}" == "rate_limited" ]] || return 1
  retry="$(/usr/bin/sed -n 's/^retry_after=//p' "${RESULT}" | /usr/bin/head -n1)"
  [[ "${retry}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] || return 1
  mapfile -t cooldown_hosts < <(/usr/bin/sed -n 's/^host=//p' "${RESULT}")
  [[ "${#cooldown_hosts[@]}" -eq 2 ]] || return 1
  for host in "${hosts[@]}"; do
    matched=0
    for cooldown_host in "${cooldown_hosts[@]}"; do
      [[ "${host}" == "${cooldown_host}" ]] && matched=1
    done
    [[ "${matched}" -eq 1 ]] || return 1
  done
  retry_epoch="$(/usr/bin/date -u -d "${retry}" '+%s' 2>/dev/null || true)"
  now="$(/usr/bin/date -u '+%s')"
  [[ -n "${retry_epoch}" && "${retry_epoch}" -gt "${now}" ]] || return 1
  RETRY_AFTER="${retry}"
  return 0
}

redgres_tls_publish_result() {
  local tmp result_dir
  result_dir="$(/usr/bin/dirname -- "${RESULT}")"
  [[ -d "${result_dir}" ]] || return 1
  tmp="$(/usr/bin/mktemp "${result_dir}/.tls-issue.result.XXXXXX")" || return 1
  if ! printf '%s\n' "$@" >"${tmp}" || ! /usr/bin/chmod 0644 "${tmp}" || ! /usr/bin/mv -fT -- "${tmp}" "${RESULT}"; then
    rm -f -- "${tmp}"
    return 1
  fi
}

redgres_tls_publish_cleanup_result() {
  local value="$1" tmp result_dir
  [[ "${value}" == "cleaned" || "${value}" == "failed" ]] || return 1
  result_dir="$(/usr/bin/dirname -- "${CLEANUP_RESULT}")"
  tmp="$(/usr/bin/mktemp "${result_dir}/.tls-cleanup.result.XXXXXX")" || return 1
  if ! printf '%s\n' "${value}" >"${tmp}" || ! /usr/bin/chmod 0644 "${tmp}" || ! /usr/bin/mv -fT -- "${tmp}" "${CLEANUP_RESULT}"; then
    rm -f -- "${tmp}"
    return 1
  fi
}

redgres_tls_publish_issued() {
  local db_status=certificate_prepared
  [[ -z "${TLS_POSTGRES_CLUSTER}" ]] || db_status=issued
  redgres_tls_publish_result 'issued' "${hosts[@]}" "db_status=${db_status}" 'rs_status=certificate_prepared'
}

fail() {
  local reason="${1:-dependency}" retry_after="${2:-}" failed_host
  local lines=('failed' "reason=${reason}")
  for failed_host in "${hosts[@]}"; do
    lines+=("host=${failed_host}")
  done
  if [[ -n "${retry_after}" ]]; then
    lines+=("retry_after=${retry_after}")
  fi
  redgres_tls_publish_result "${lines[@]}" || true
  rm -f "${ACTIVE_REQUEST}"
  exit 1
}

# Missing request is a PathChanged-on-delete race; do not write failed.
if redgres_tls_active_trusted; then
  : # Resume a root-owned claim left by a killed/interrupted previous run.
elif [[ -f "${REQUEST}" && ! -L "${REQUEST}" ]]; then
  if ! redgres_tls_claim_request; then
    redgres_tls_publish_result 'failed' 'reason=dependency' || true
    exit 0
  fi
else
  exit 0
fi
redgres_tls_active_trusted || fail dependency
[[ "$(/usr/bin/stat -c '%s' "${ACTIVE_REQUEST}")" -le 1024 ]] || fail dependency
if [[ "$(/usr/bin/cat "${ACTIVE_REQUEST}")" == "cleanup_domain_tls" ]]; then
  if redgres_tls_cleanup_domain && redgres_tls_publish_cleanup_result cleaned; then
    exit 0
  fi
  redgres_tls_publish_cleanup_result failed || true
  rm -f -- "${ACTIVE_REQUEST}"
  exit 1
fi
redgres_tls_load_targets || fail dependency

mapfile -t hosts < <(/usr/bin/tr -d '\r' <"${ACTIVE_REQUEST}" | /usr/bin/awk 'NF && $0 !~ /^#/')
[[ "${#hosts[@]}" -eq 2 ]] || fail dependency
primary=""
args=()
for i in "${!hosts[@]}"; do
  host="${hosts[$i]}"
  host="$(printf '%s' "${host}" | /usr/bin/tr '[:upper:]' '[:lower:]')"
  redgres_tls_valid_hostname "${host}" || fail
  hosts[$i]="${host}"
  [[ -n "${primary}" ]] || primary="${host}"
  args+=(-d "${host}")
done
[[ "${hosts[0]}" != "${hosts[1]}" ]] || fail dependency

[[ -f "${CREDS}" && ! -L "${CREDS}" ]] || fail credentials
redgres_tls_snapshot_credentials || fail credentials

if redgres_tls_find_existing "${primary}" "${hosts[@]}"; then
  redgres_tls_copy_certs "${MATCHED_LIVE}" || fail dependency
  redgres_tls_record_lineage "${MATCHED_LIVE}" || fail dependency
  printf 'TLS certificate already valid for %s hostname(s); copied without a new ACME order.\n' "${#hosts[@]}"
  redgres_tls_publish_issued || fail dependency
  rm -f "${ACTIVE_REQUEST}"
  exit 0
fi

if redgres_tls_rate_limit_active; then
  fail rate_limited "${RETRY_AFTER}"
fi

certbot_output="$(/usr/bin/mktemp)"
cert_name="${MATCHED_RENEWAL_NAME:-${primary}}"
if ! "${CERTBOT}" certonly \
  --non-interactive \
  --agree-tos \
  --register-unsafely-without-email \
  --dns-cloudflare \
  --dns-cloudflare-credentials "${CERTBOT_CREDS}" \
  --dns-cloudflare-propagation-seconds 60 \
  --cert-name "${cert_name}" \
  --keep-until-expiring \
  "${args[@]}" >"${certbot_output}" 2>&1; then
  redgres_tls_classify_certbot_failure "${certbot_output}"
  printf 'TLS issuance failed (%s).\n' "${FAIL_REASON}" >&2
  if [[ -n "${RETRY_AFTER}" ]]; then
    printf 'Retry after %s.\n' "${RETRY_AFTER}" >&2
  fi
  printf '%s\n' 'See /var/log/letsencrypt/letsencrypt.log for Certbot diagnostics.' >&2
  fail "${FAIL_REASON}" "${RETRY_AFTER}"
fi
printf 'TLS certificate issued for %s hostname(s).\n' "${#hosts[@]}"

redgres_tls_find_existing "${primary}" "${hosts[@]}" || fail dependency
redgres_tls_copy_certs "${MATCHED_LIVE}" || fail dependency
redgres_tls_record_lineage "${MATCHED_LIVE}" || fail dependency
redgres_tls_publish_issued || fail dependency
rm -f "${ACTIVE_REQUEST}"
exit 0
