#!/usr/bin/env bash
# POSIX installer dispatcher tests (OPS-001 / OPS-002 / OPS-006 Partial).
# Prepends failing mutation stubs so a real host call fails the test.
# Detection stubs print fixture --version stdout and must not append to stub_log.
set -euo pipefail

tests_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
deploy_dir="$(cd "${tests_dir}/.." && pwd)"
install_sh="${deploy_dir}/install.sh"
fixtures_dir="${tests_dir}/fixtures"

failures=0
passes=0

pass() {
  printf 'ok - %s\n' "$1"
  passes=$((passes + 1))
}

fail() {
  printf 'not ok - %s\n' "$1"
  failures=$((failures + 1))
}

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/redgres-install-test.XXXXXX")"
stub_dir="${tmpdir}/stubs"
detect_dir="${tmpdir}/detect"
stub_log="${tmpdir}/stub.log"
config_file="${tmpdir}/install.env"
plan_file="${tmpdir}/postgres-extensions.json"
sourced_marker="${tmpdir}/config-sourced"
mkdir -p "${stub_dir}" "${detect_dir}"
: >"${stub_log}"
original_path="${PATH}"

cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

# Canary must never appear in installer output (env is not a log dump).
export REDGRES_INSTALLER_CANARY='rg-canary-do-not-print-7f2c'

cat >"${config_file}" <<EOF
echo CONFIG_SOURCED >"${sourced_marker}"
echo "${REDGRES_INSTALLER_CANARY}"
exit 99
EOF

printf '%s\n' '{"not":"parsed"}' >"${plan_file}"

STUB_NAMES='apt-get apt dnf yum docker dockerd systemctl ufw cloudflared certbot curl wget initdb pg_dropcluster pg_createcluster'

write_stub() {
  local name="$1"
  cat >"${stub_dir}/${name}" <<STUB
#!/usr/bin/env bash
printf '%s\n' "${name}" >>"${stub_log}"
printf 'FORBIDDEN stub invoked: %s\n' "${name}" >&2
exit 1
STUB
  chmod +x "${stub_dir}/${name}"
}

for _stub in ${STUB_NAMES}; do
  write_stub "${_stub}"
done

# Installer PATH: detection stubs, mutation stubs, then original PATH minus
# directories that contain host postgres/redis-server/pgbouncer (so missing-binary
# tests cannot leak a real binary, without hiding cat/dirname).
installer_path="${detect_dir}:${stub_dir}"
_old_ifs="${IFS}"
IFS=':'
for _dir in ${original_path}; do
  [[ -n "${_dir}" ]] || continue
  if [[ -e "${_dir}/postgres" || -e "${_dir}/postgres.exe" \
     || -e "${_dir}/redis-server" || -e "${_dir}/redis-server.exe" \
     || -e "${_dir}/pgbouncer" || -e "${_dir}/pgbouncer.exe" ]]; then
    continue
  fi
  installer_path="${installer_path}:${_dir}"
done
IFS="${_old_ifs}"
unset _dir _old_ifs

clear_detect_stubs() {
  rm -f "${detect_dir}/postgres" "${detect_dir}/redis-server" "${detect_dir}/pgbouncer"
}

write_detect_stub() {
  local name="$1"
  local fixture="$2"
  cat >"${detect_dir}/${name}" <<STUB
#!/usr/bin/env bash
# Detection stub: fixture stdout only. Do not append to mutation stub_log.
cat '${fixture}'
exit 0
STUB
  chmod +x "${detect_dir}/${name}"
}

DETECT_POSTGRES=''
DETECT_REDIS=''
DETECT_PGBOUNCER=''

export PATH="${stub_dir}:${PATH}"

output=''
status=0

run_install() {
  : >"${stub_log}"
  rm -f "${sourced_marker}"
  clear_detect_stubs
  if [[ -n "${DETECT_POSTGRES}" ]]; then
    write_detect_stub postgres "${DETECT_POSTGRES}"
  fi
  if [[ -n "${DETECT_REDIS}" ]]; then
    write_detect_stub redis-server "${DETECT_REDIS}"
  fi
  if [[ -n "${DETECT_PGBOUNCER}" ]]; then
    write_detect_stub pgbouncer "${DETECT_PGBOUNCER}"
  fi
  DETECT_POSTGRES=''
  DETECT_REDIS=''
  DETECT_PGBOUNCER=''
  set +e
  output="$(PATH="${installer_path}" "${BASH}" "${install_sh}" "$@" 2>&1)"
  status=$?
  set -e
}

assert_no_mutation() {
  local name="$1"
  if [[ -s "${stub_log}" ]]; then
    fail "${name}: mutation stub invoked: $(tr '\n' ' ' <"${stub_log}")"
    return 1
  fi
  if [[ -e "${sourced_marker}" ]]; then
    fail "${name}: --config was sourced"
    return 1
  fi
  return 0
}

assert_no_canary() {
  local name="$1"
  case "${output}" in
    *"${REDGRES_INSTALLER_CANARY}"*)
      fail "${name}: canary env token printed"
      return 1
      ;;
  esac
  return 0
}

expect_status() {
  local name="$1"
  local expected="$2"
  if ! assert_no_mutation "${name}"; then
    return
  fi
  if ! assert_no_canary "${name}"; then
    return
  fi
  if [[ "${status}" -eq "${expected}" ]]; then
    pass "${name}"
  else
    fail "${name}: expected exit ${expected}, got ${status}: ${output}"
  fi
}

expect_status_keyword() {
  local name="$1"
  local expected="$2"
  local keyword="$3"
  if ! assert_no_mutation "${name}"; then
    return
  fi
  if ! assert_no_canary "${name}"; then
    return
  fi
  if [[ "${status}" -ne "${expected}" ]]; then
    fail "${name}: expected exit ${expected}, got ${status}: ${output}"
    return
  fi
  case "${output}" in
    *"${keyword}"*) pass "${name}" ;;
    *) fail "${name}: missing '${keyword}': ${output}" ;;
  esac
}

expect_status_and_stages() {
  local name="$1"
  shift
  if ! assert_no_mutation "${name}"; then
    return
  fi
  if ! assert_no_canary "${name}"; then
    return
  fi
  if [[ "${status}" -ne 0 ]]; then
    fail "${name}: expected exit 0, got ${status}: ${output}"
    return
  fi
  local missing=''
  local stage
  for stage in \
    'Preflight' \
    'Inventory' \
    'Safety gate' \
    'Packages' \
    'Identity/filesystem' \
    'Redis' \
    'PostgreSQL/PgBouncer' \
    'TLS/DNS' \
    'Application release' \
    'Cloudflare' \
    'Firewall' \
    'End-to-end verify' \
    'Report'; do
    case "${output}" in
      *"${stage}"*) ;;
      *) missing="${missing} ${stage}" ;;
    esac
  done
  local extra
  for extra in "$@"; do
    case "${output}" in
      *"${extra}"*) ;;
      *) missing="${missing} |${extra}|" ;;
    esac
  done
  if [[ -n "${missing}" ]]; then
    fail "${name}: missing:${missing}"
    return
  fi
  pass "${name}"
}

# --- --help ---
run_install --help
expect_status 'help exits 0' 0

# --- happy dry-run (existing services) ---
DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --expect-postgres-major 17 \
  --redis-mode existing \
  --expect-redis-series 8.2 \
  --pgbouncer-mode existing \
  --config "${config_file}"
expect_status_and_stages 'dry-run existing-postgres combo prints 13 stages' \
  'Inventory (read-only, host --version; not SQL SHOW/INFO):' \
  'postgres: detected=postgres (PostgreSQL) 17.11 major=17 expect=17 result=ok' \
  'redis: detected=Redis server v=8.2.0 sha=00000000:0 malloc=libc bits=64 build=0 series=8.2 expect=8.2 result=ok' \
  'pgbouncer: detected=PgBouncer 1.24.1 result=recorded'

# --- happy dry-run (fresh services) ---
run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --extension-plan "${plan_file}" \
  --approve-postgres-restart
expect_status_and_stages 'dry-run fresh-postgres combo prints 13 stages' \
  'Inventory (read-only, host --version; not SQL SHOW/INFO):' \
  'postgres: skipped (fresh-postgres)' \
  'redis: skipped (fresh)' \
  'pgbouncer: skipped (disabled)'

# --- fail-closed: unknown flag ---
run_install --non-interactive --dry-run --mode existing-postgres --redis-mode existing --pgbouncer-mode existing --unknown-flag
expect_status 'unknown flag exits 1' 1

# --- fail-closed: latest / latest-tested / tags / other majors / prerelease ---
run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version latest --redis-mode fresh --redis-version 8.2 --pgbouncer-mode fresh
expect_status 'postgres-version latest exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version latest-tested --redis-mode fresh --redis-version 8.2 --pgbouncer-mode fresh
expect_status 'postgres-version latest-tested exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 18.6 --redis-mode fresh --redis-version 8.2 --pgbouncer-mode fresh
expect_status 'postgres-version 18.6 tag form exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 16 --redis-mode fresh --redis-version 8.2 --pgbouncer-mode fresh
expect_status 'postgres-version 16 exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 18rc1 --redis-mode fresh --redis-version 8.2 --pgbouncer-mode fresh
expect_status 'postgres-version prerelease exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 18 --redis-mode fresh --redis-version latest --pgbouncer-mode fresh
expect_status 'redis-version latest exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 18 --redis-mode fresh --redis-version latest-tested --pgbouncer-mode fresh
expect_status 'redis-version latest-tested exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 18 --redis-mode fresh --redis-version 8.8.2 --pgbouncer-mode fresh
expect_status 'redis-version 8.8.2 tag form exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 18 --redis-mode fresh --redis-version 8.10 --pgbouncer-mode fresh
expect_status 'redis-version 8.10 exits 1' 1

run_install --non-interactive --dry-run --mode existing-postgres --expect-postgres-major 7 --redis-mode existing --pgbouncer-mode existing
expect_status 'expect-postgres-major 7 exits 1' 1

# --- fail-closed: missing required --non-interactive flags ---
run_install --dry-run --mode existing-postgres --redis-mode existing --pgbouncer-mode existing
expect_status 'missing --non-interactive exits 1' 1

run_install --non-interactive --dry-run --redis-mode existing --pgbouncer-mode existing
expect_status 'missing --mode exits 1' 1

run_install --non-interactive --dry-run --mode existing-postgres --pgbouncer-mode existing
expect_status 'missing --redis-mode exits 1' 1

run_install --non-interactive --dry-run --mode existing-postgres --redis-mode existing
expect_status 'missing --pgbouncer-mode exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --redis-mode fresh --redis-version 8.2 --pgbouncer-mode fresh
expect_status 'fresh-postgres missing --postgres-version exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 18 --redis-mode fresh --pgbouncer-mode fresh
expect_status 'redis-mode fresh missing --redis-version exits 1' 1

# --- fail-closed: flag/mode mismatches ---
run_install --non-interactive --dry-run --mode existing-postgres --postgres-version 18 --redis-mode existing --pgbouncer-mode existing
expect_status '--postgres-version with existing-postgres exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 18 --expect-postgres-major 18 --redis-mode fresh --redis-version 8.8 --pgbouncer-mode disabled
expect_status '--expect-postgres-major with fresh-postgres exits 1' 1

run_install --non-interactive --dry-run --mode existing-postgres --redis-mode existing --redis-version 8.2 --pgbouncer-mode existing
expect_status '--redis-version with redis-mode existing exits 1' 1

run_install --non-interactive --dry-run --mode fresh-postgres --postgres-version 17 --redis-mode fresh --redis-version 8.2 --expect-redis-series 8.2 --pgbouncer-mode existing
expect_status '--expect-redis-series with redis-mode fresh exits 1' 1

# --- subcommands: not implemented, exit 2, no mutation ---
run_install verify
expect_status 'verify subcommand exits 2' 2

run_install backup
expect_status 'backup subcommand exits 2' 2

run_install update
expect_status 'update subcommand exits 2' 2

run_install rollback
expect_status 'rollback subcommand exits 2' 2

run_install postgres-plan
expect_status 'postgres-plan subcommand exits 2' 2

run_install postgres-extensions apply
expect_status 'postgres-extensions subcommand exits 2' 2

# --- valid flags without --dry-run: mutation not implemented (before inventory) ---
DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
run_install \
  --non-interactive \
  --mode existing-postgres \
  --expect-postgres-major 17 \
  --redis-mode existing \
  --expect-redis-series 8.2 \
  --pgbouncer-mode existing
expect_status 'valid flags without --dry-run exit 2' 2
case "${output}" in
  *'Inventory (read-only'*)
    fail 'valid flags without --dry-run must not inventory'
    ;;
  *)
    pass 'valid flags without --dry-run skip inventory'
    ;;
esac

# --- OPS-002 inventory fail-closed ---
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --expect-postgres-major 17 \
  --redis-mode existing \
  --expect-redis-series 8.2 \
  --pgbouncer-mode existing
expect_status_keyword 'existing without detection stubs exits 1' 1 'postgres not found'

DETECT_POSTGRES="${fixtures_dir}/postgres-18.6.version"
DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --expect-postgres-major 17 \
  --redis-mode existing \
  --expect-redis-series 8.2 \
  --pgbouncer-mode existing
expect_status_keyword 'expect-postgres-major mismatch exits 1' 1 'mismatch'
case "${output}" in
  *[Uu]pgrade*)
    fail 'expect-postgres-major mismatch must not say upgrade'
    ;;
  *)
    pass 'expect-postgres-major mismatch has no upgrade wording'
    ;;
esac

DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
DETECT_REDIS="${fixtures_dir}/redis-8.8.2.version"
DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --expect-postgres-major 17 \
  --redis-mode existing \
  --expect-redis-series 8.2 \
  --pgbouncer-mode existing
expect_status_keyword 'expect-redis-series mismatch exits 1' 1 'mismatch'

DETECT_POSTGRES="${fixtures_dir}/postgres-16.15.version"
DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --redis-mode existing \
  --pgbouncer-mode existing
expect_status_keyword 'unsupported postgres 16 exits 1' 1 'unsupported'

DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
DETECT_REDIS="${fixtures_dir}/redis-8.10.1.version"
DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --redis-mode existing \
  --pgbouncer-mode existing
expect_status_keyword 'unsupported redis 8.10 exits 1' 1 'unsupported'

DETECT_POSTGRES="${fixtures_dir}/postgres-unparseable.version"
DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --redis-mode existing \
  --pgbouncer-mode existing
expect_status_keyword 'unparseable postgres --version exits 1' 1 'unparseable'

DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --redis-mode existing \
  --pgbouncer-mode disabled
expect_status_and_stages 'pgbouncer disabled skips pgbouncer binary' \
  'pgbouncer: skipped (disabled)' \
  'postgres: detected=postgres (PostgreSQL) 17.11 major=17 expect=unset result=ok' \
  'redis: detected=Redis server v=8.2.0 sha=00000000:0 malloc=libc bits=64 build=0 series=8.2 expect=unset result=ok'

DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --redis-mode existing \
  --pgbouncer-mode existing
expect_status_keyword 'pgbouncer existing missing binary exits 1' 1 'pgbouncer not found'

printf '\n%s passed, %s failed\n' "${passes}" "${failures}"
if [[ "${failures}" -ne 0 ]]; then
  exit 1
fi
exit 0
