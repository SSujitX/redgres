#!/usr/bin/env bash
# POSIX installer dispatcher tests (OPS-001 / OPS-002 / OPS-003 / OPS-004 / OPS-005 / OPS-006 / OPS-007 Partial).
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

redgres_test_mode_is_group_or_world_writable() {
  local mode="$1"
  (( (8#${mode} & 8#022) != 0 ))
}

tmp_base="${TMPDIR:-/tmp}"
if [[ "${EUID}" -eq 0 ]]; then
  tmp_base='/root'
fi
tmpdir="$(mktemp -d "${tmp_base}/redgres-install-test.XXXXXX")"
stub_dir="${tmpdir}/stubs"
detect_dir="${tmpdir}/detect"
unsafe_dir="${tmpdir}/unsafe"
stub_log="${tmpdir}/stub.log"
process_canary="${tmpdir}/process-canary"
config_file="${tmpdir}/install.env"
plan_file="${tmpdir}/postgres-extensions.json"
sourced_marker="${tmpdir}/config-sourced"
mkdir -p "${stub_dir}" "${detect_dir}" "${unsafe_dir}"
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

printf '%s\n' '{"policy":"preserve","selections":[]}' >"${plan_file}"

STUB_NAMES='apt-get apt dnf yum docker dockerd systemctl ufw cloudflared certbot curl wget tar initdb pg_dropcluster pg_createcluster'

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
EXTRA_PATH_PREFIX=''

export PATH="${stub_dir}:${PATH}"

output=''
status=0

run_install() {
  : >"${stub_log}"
  rm -f "${sourced_marker}"
  rm -f "${process_canary}"
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
  output="$(PATH="${EXTRA_PATH_PREFIX:+${EXTRA_PATH_PREFIX}:}${installer_path}" "${BASH}" "${install_sh}" "$@" 2>&1)"
  status=$?
  set -e
  EXTRA_PATH_PREFIX=''
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

expect_verify_partial() {
  local name="$1"
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
  case "${output}" in
    *'Inventory (read-only'*)
      fail "${name}: must not call inventory"
      return
      ;;
    *'result=ok'*)
      fail "${name}: skips must not be result=ok"
      return
      ;;
  esac
  local missing=''
  local keyword
  for keyword in \
    'Verify (read-only --dry-run; not DNS/Cloudflare/public TLS; not Complete):' \
    'config: path-ok (unread, not sourced)' \
    'dns: skipped (this Partial cannot check DNS)' \
    'cloudflare: skipped (this Partial cannot check Tunnel/Access/routes)' \
    'tls_public: skipped (this Partial cannot check public certificates/TLS)' \
    'http_healthz: skipped (GET /api/v1/healthz not probed; curl not invoked)' \
    'auth_boundaries: skipped (GET /api/v1/status not probed)' \
    'bindings: skipped (live sockets deferred; intended redgres 127.0.0.1:8790)' \
    'services: skipped (cluster SHOW/INFO deferred; PATH --version is OPS-002 install)' \
    'backup_prerequisites: skipped (no named backup keys; OPS-004 owns backup)' \
    'result=partial'; do
    case "${output}" in
      *"${keyword}"*) ;;
      *) missing="${missing} |${keyword}|" ;;
    esac
  done
  if [[ -n "${missing}" ]]; then
    fail "${name}: missing:${missing}: ${output}"
    return
  fi
  case "$(tr '\n' ' ' <"${stub_log}")" in
    *curl*)
      fail "${name}: curl in stub_log"
      return
      ;;
  esac
  pass "${name}"
}

expect_update_partial() {
  local name="$1"
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
  case "${output}" in
    *'Inventory (read-only'*)
      fail "${name}: must not call inventory"
      return
      ;;
    *'result=ok'*)
      fail "${name}: skips must not be result=ok"
      return
      ;;
    *'data_reversal:'*)
      fail "${name}: update must not print data_reversal"
      return
      ;;
  esac
  local missing=''
  local keyword
  for keyword in \
    'Update (read-only --dry-run; not Complete):' \
    'release: path-ok (unread, not extracted)' \
    'checksum: skipped (no expected digest key; CONFIGURATION.md has none)' \
    'extract: skipped (/opt/redgres/releases not written)' \
    'symlink: skipped (current not switched)' \
    'sqlite_migrate: skipped' \
    'systemd: skipped (unit/credentials not written)' \
    'health_gate: skipped (GET /api/v1/healthz not probed; curl not invoked)' \
    'postgres_packages: skipped (not part of application update)' \
    'result=partial'; do
    case "${output}" in
      *"${keyword}"*) ;;
      *) missing="${missing} |${keyword}|" ;;
    esac
  done
  if [[ -n "${missing}" ]]; then
    fail "${name}: missing:${missing}: ${output}"
    return
  fi
  case "$(tr '\n' ' ' <"${stub_log}")" in
    *curl*|*tar*)
      fail "${name}: curl or tar in stub_log"
      return
      ;;
  esac
  pass "${name}"
}

expect_rollback_partial() {
  local name="$1"
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
  case "${output}" in
    *'Inventory (read-only'*)
      fail "${name}: must not call inventory"
      return
      ;;
    *'result=ok'*)
      fail "${name}: skips must not be result=ok"
      return
      ;;
    *'rel-1'*)
      fail "${name}: must not print VERSION"
      return
      ;;
  esac
  local missing=''
  local keyword
  for keyword in \
    'Rollback (read-only --dry-run; not Complete):' \
    'target: accepted (unread; symlink not switched)' \
    'schema_compat: skipped (SQLite schema compatibility not checked)' \
    'symlink: skipped (current not switched)' \
    'config_restore: skipped' \
    'systemd: skipped (unit not restarted)' \
    'health_gate: skipped (GET /api/v1/healthz not probed; curl not invoked)' \
    'data_reversal: skipped (rollback never reverses PostgreSQL/Redis/vault/credentials/DNS/schema automatically)' \
    'result=partial'; do
    case "${output}" in
      *"${keyword}"*) ;;
      *) missing="${missing} |${keyword}|" ;;
    esac
  done
  if [[ -n "${missing}" ]]; then
    fail "${name}: missing:${missing}: ${output}"
    return
  fi
  case "$(tr '\n' ' ' <"${stub_log}")" in
    *curl*|*tar*)
      fail "${name}: curl or tar in stub_log"
      return
      ;;
  esac
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

# --- OPS-007 main dry-run validates an optional extension plan ---
plan_valid_apply="${tmpdir}/plan-valid-apply.json"
cat >"${plan_valid_apply}" <<'PLAN'
{
  "policy": "apply-selected",
  "selections": [
    { "capability": "pg_stat_statements", "databases": ["app_production"] },
    { "capability": "vector", "databases": ["search_production"] }
  ]
}
PLAN

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --extension-plan "${plan_valid_apply}"
expect_status_and_stages 'main dry-run valid extension plan exits 0' \
  'postgres: skipped (fresh-postgres)' \
  'redis: skipped (fresh)' \
  'pgbouncer: skipped (disabled)'

plan_invalid_cap="${tmpdir}/plan-invalid-cap.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"nope","databases":["x"]}]}' >"${plan_invalid_cap}"
run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --extension-plan "${plan_invalid_cap}"
expect_status 'main dry-run invalid extension plan exits 1' 1

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --extension-plan "${tmpdir}/missing-plan.json"
expect_status 'main dry-run missing extension plan path exits 1' 1

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --extension-plan "${tmpdir}"
expect_status 'main dry-run extension plan directory exits 1' 1

# --- OPS-001 main dry-run validates optional --config as a regular file ---
run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --config "${config_file}"
expect_status_and_stages 'main dry-run valid --config exits 0' \
  'postgres: skipped (fresh-postgres)' \
  'redis: skipped (fresh)' \
  'pgbouncer: skipped (disabled)'

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --config "${tmpdir}"
expect_status 'main dry-run --config directory exits 1' 1

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --config "${tmpdir}/missing.env"
expect_status 'main dry-run --config missing path exits 1' 1

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

# --- OPS-003 verify --dry-run skip matrix ---
run_install verify --help
expect_status 'verify --help exits 0' 0

run_install verify --non-interactive --dry-run --config "${config_file}"
expect_verify_partial 'verify dry-run skip matrix exits 0'

run_install verify
expect_status 'verify without flags exits 1' 1

run_install verify --non-interactive --config "${config_file}"
expect_status 'verify without --dry-run exits 2' 2
case "${output}" in
  *'Inventory (read-only'*)
    fail 'verify without --dry-run must not inventory'
    ;;
  *)
    pass 'verify without --dry-run skips inventory'
    ;;
esac

run_install verify --non-interactive --dry-run
expect_status 'verify missing --config exits 1' 1

run_install verify --non-interactive --dry-run --config "${tmpdir}/missing.env"
expect_status 'verify --config missing path exits 1' 1

run_install verify --non-interactive --dry-run --config "${tmpdir}"
expect_status 'verify --config directory exits 1' 1

run_install verify --non-interactive --dry-run --config "${config_file}" --mode existing-postgres
expect_status 'verify unknown --mode flag exits 1' 1

# --- subcommands: not implemented, exit 2, no mutation ---
run_install postgres-extensions
expect_status 'postgres-extensions without apply subcommand exits 1' 1

run_install postgres-extensions apply
expect_status 'postgres-extensions apply without flags exits 1' 1

# --- OPS-007 postgres-extensions apply --dry-run skip matrix ---
expect_extensions_partial() {
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
  case "${output}" in
    *'Inventory (read-only'*)
      fail "${name}: must not call inventory"
      return
      ;;
    *'result=ok'*)
      fail "${name}: skips must not be result=ok"
      return
      ;;
  esac
  local missing=''
  local keyword
  for keyword in \
    'postgres-extensions apply (read-only --dry-run; not Complete):' \
    'config: path-ok (unread, not sourced)' \
    'plan: path-ok (validated, not applied)' \
    'package_resolution: skipped (no release manifest in this Partial)' \
    'inventory: skipped (live cluster state not probed)' \
    'backup_verification: skipped (backup evidence not checked)' \
    'preload_merge: skipped (shared_preload_libraries not read)' \
    'restart_approval: skipped (--approve-postgres-restart is apply-time)' \
    'extension_ddl: skipped (CREATE EXTENSION not executed)' \
    'verification: skipped (capability smoke checks deferred)' \
    'result=partial' "$@"; do
    case "${output}" in
      *"${keyword}"*) ;;
      *) missing="${missing} |${keyword}|" ;;
    esac
  done
  if [[ -n "${missing}" ]]; then
    fail "${name}: missing:${missing}: ${output}"
    return
  fi
  pass "${name}"
}

# Fixtures for the OPS-007 postgres-extensions apply dry-run tests.
plan_config="${tmpdir}/plan-config.env"
printf 'unused-plan-config\n' >"${plan_config}"

plan_apply_file="${tmpdir}/plan-apply.json"
cat >"${plan_apply_file}" <<'PLAN'
{
  "policy": "apply-selected",
  "selections": [
    { "capability": "pg_stat_statements", "databases": ["app_production"] },
    { "capability": "vector", "databases": ["search_production"] },
    { "capability": "pg_partman", "databases": ["events_production"], "scheduler": "pg_cron" }
  ]
}
PLAN

plan_bad_cap="${tmpdir}/plan-bad-cap.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"nope","databases":["x"]}]}' >"${plan_bad_cap}"

run_install postgres-extensions apply --non-interactive --dry-run --config "${plan_config}" --extension-plan "${plan_apply_file}"
expect_extensions_partial 'postgres-extensions apply dry-run exits 0' 'policy: apply-selected'

run_install postgres-extensions apply --non-interactive --dry-run --config "${plan_config}" --extension-plan "${plan_apply_file}" --approve-postgres-restart
expect_extensions_partial 'postgres-extensions apply dry-run with restart approval exits 0'

run_install postgres-extensions apply --non-interactive --dry-run --config "${plan_config}" --extension-plan "${plan_bad_cap}"
expect_status 'postgres-extensions apply invalid plan exits 1' 1

run_install postgres-extensions apply --non-interactive --dry-run --config "${plan_config}"
expect_status 'postgres-extensions apply missing --extension-plan exits 1' 1

run_install postgres-extensions apply --non-interactive --config "${plan_config}" --extension-plan "${plan_apply_file}"
expect_status 'postgres-extensions apply without --dry-run exits 2' 2

run_install postgres-extensions apply --non-interactive --dry-run --config "${tmpdir}/missing.env" --extension-plan "${plan_apply_file}"
expect_status 'postgres-extensions apply --config missing path exits 1' 1

run_install postgres-extensions apply --non-interactive --dry-run --config "${plan_config}" --extension-plan "${tmpdir}"
expect_status 'postgres-extensions apply --extension-plan directory exits 1' 1

run_install postgres-extensions apply --non-interactive --dry-run --config "${plan_config}" --extension-plan "${plan_apply_file}" --mode existing-postgres
expect_status 'postgres-extensions apply unknown --mode flag exits 1' 1

run_install postgres-extensions apply --help
expect_status 'postgres-extensions apply --help exits 0' 0

run_install postgres-extensions --help
expect_status 'postgres-extensions --help exits 0' 0

run_install postgres-extensions apply --non-interactive --dry-run --config
expect_status 'postgres-extensions apply missing --config value exits 1' 1

run_install postgres-extensions apply --non-interactive --dry-run --config "${plan_config}" --extension-plan
expect_status 'postgres-extensions apply missing --extension-plan value exits 1' 1

run_install postgres-extensions apply --non-interactive --dry-run --config "${plan_config}" --extension-plan "${plan_apply_file}" extra
expect_status 'postgres-extensions apply bare positional exits 1' 1

run_install postgres-extensions apply --non-interactive --dry-run --config "${tmpdir}" --extension-plan "${plan_apply_file}"
expect_status 'postgres-extensions apply --config directory exits 1' 1

# --- OPS-004 backup --dry-run skip matrix ---
expect_backup_partial() {
  local name="$1"
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
  case "${output}" in
    *'Inventory (read-only'*)
      fail "${name}: must not call inventory"
      return
      ;;
    *'result=ok'*)
      fail "${name}: skips must not be result=ok"
      return
      ;;
  esac
  local missing=''
  local keyword
  for keyword in \
    'Backup (read-only --dry-run; not Complete):' \
    'config: path-ok (unread, not sourced)' \
    'postgres_dump: skipped (pg_dump/pg_dumpall not invoked; installer-recovery)' \
    'redis_snapshot: skipped (BGSAVE/LASTSAVE not invoked; installer-recovery)' \
    'sqlite_backup: skipped (SQLite online backup not invoked; installer-recovery)' \
    'manifest: skipped (no backup manifest written)' \
    'checksums: skipped (no artifact SHA-256 computed)' \
    'retention: skipped (no pruning of manifest-owned paths)' \
    'off_host: skipped (no encrypted off-host copy)' \
    'restore_evidence: skipped (no isolated restore test)' \
    'result=partial'; do
    case "${output}" in
      *"${keyword}"*) ;;
      *) missing="${missing} |${keyword}|" ;;
    esac
  done
  if [[ -n "${missing}" ]]; then
    fail "${name}: missing:${missing}: ${output}"
    return
  fi
  pass "${name}"
}

run_install backup --non-interactive --dry-run --config "${config_file}"
expect_backup_partial 'backup dry-run skip matrix exits 0'

run_install backup
expect_status 'backup without flags exits 1' 1

run_install backup --dry-run --config "${config_file}"
expect_status 'backup missing --non-interactive exits 1' 1

run_install backup --non-interactive --config "${config_file}"
expect_status 'backup without --dry-run exits 2' 2
case "${output}" in
  *'Inventory (read-only'*)
    fail 'backup without --dry-run must not inventory'
    ;;
  *)
    pass 'backup without --dry-run skips inventory'
    ;;
esac

run_install backup --non-interactive --dry-run
expect_status 'backup missing --config exits 1' 1

run_install backup --non-interactive --dry-run --config "${tmpdir}/missing.env"
expect_status 'backup --config missing path exits 1' 1

run_install backup --non-interactive --dry-run --config "${tmpdir}"
expect_status 'backup --config directory exits 1' 1

run_install backup --non-interactive --dry-run --config "${config_file}" --mode existing-postgres
expect_status 'backup unknown --mode flag exits 1' 1

# --- OPS-007 postgres-plan read-only plan validation ---
expect_plan_partial() {
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
  case "${output}" in
    *'Inventory (read-only'*)
      fail "${name}: must not call inventory"
      return
      ;;
    *'result=ok'*)
      fail "${name}: plan skips must not be result=ok"
      return
      ;;
  esac
  local missing=''
  local keyword
  for keyword in \
    'postgres-plan (read-only; not Complete):' \
    'config: path-ok (unread, not sourced)' \
    'plan: path-ok (validated, not applied)' \
    'package_resolution: skipped (no release manifest in this Partial)' \
    'inventory: skipped (live cluster state not probed)' \
    'backup_verification: skipped (backup evidence not checked)' \
    'preload_merge: skipped (shared_preload_libraries not read)' \
    'restart_approval: skipped (--approve-postgres-restart is apply-time)' \
    'extension_ddl: skipped (CREATE EXTENSION not executed)' \
    'verification: skipped (capability smoke checks deferred)' \
    'result=partial' "$@"; do
    case "${output}" in
      *"${keyword}"*) ;;
      *) missing="${missing} |${keyword}|" ;;
    esac
  done
  if [[ -n "${missing}" ]]; then
    fail "${name}: missing:${missing}: ${output}"
    return
  fi
  pass "${name}"
}

plan_config="${tmpdir}/plan-config.env"
printf 'unused-plan-config\n' >"${plan_config}"

plan_apply_file="${tmpdir}/plan-apply.json"
cat >"${plan_apply_file}" <<'PLAN'
{
  "policy": "apply-selected",
  "selections": [
    { "capability": "pg_stat_statements", "databases": ["app_production"] },
    { "capability": "vector", "databases": ["search_production"] },
    { "capability": "pg_partman", "databases": ["events_production"], "scheduler": "pg_cron" }
  ]
}
PLAN

plan_preserve_file="${tmpdir}/plan-preserve.json"
cat >"${plan_preserve_file}" <<'PLAN'
{
  "policy": "preserve",
  "selections": []
}
PLAN

run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_apply_file}"
expect_plan_partial 'postgres-plan apply-selected plan exits 0' \
  'policy: apply-selected' \
  'selection: pg_stat_statements databases=[app_production]' \
  'selection: vector databases=[search_production]' \
  'selection: pg_partman databases=[events_production] scheduler=pg_cron'

run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_preserve_file}"
expect_plan_partial 'postgres-plan preserve plan exits 0' \
  'policy: preserve' \
  'selection: (none)'

# --- postgres-plan fail-closed: missing/invalid flags ---
run_install postgres-plan
expect_status 'postgres-plan missing flags exits 1' 1

run_install postgres-plan --config "${plan_config}"
expect_status 'postgres-plan missing --extension-plan exits 1' 1

run_install postgres-plan --extension-plan "${plan_apply_file}"
expect_status 'postgres-plan missing --config exits 1' 1

run_install postgres-plan --config "${tmpdir}/missing.env" --extension-plan "${plan_apply_file}"
expect_status 'postgres-plan --config missing path exits 1' 1

run_install postgres-plan --config "${tmpdir}" --extension-plan "${plan_apply_file}"
expect_status 'postgres-plan --config directory exits 1' 1

run_install postgres-plan --config "${plan_config}" --extension-plan "${tmpdir}/missing.json"
expect_status 'postgres-plan --extension-plan missing path exits 1' 1

run_install postgres-plan --config "${plan_config}" --extension-plan "${tmpdir}"
expect_status 'postgres-plan --extension-plan directory exits 1' 1

run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_apply_file}" --mode existing-postgres
expect_status 'postgres-plan unknown --mode flag exits 1' 1

# --- postgres-plan fail-closed: invalid plans ---
plan_bad_cap="${tmpdir}/plan-bad-cap.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"nope","databases":["x"]}]}' >"${plan_bad_cap}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_bad_cap}"
expect_status 'postgres-plan unknown capability exits 1' 1

plan_bad_policy="${tmpdir}/plan-bad-policy.json"
printf '%s' '{"policy":"everything","selections":[]}' >"${plan_bad_policy}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_bad_policy}"
expect_status 'postgres-plan invalid policy exits 1' 1

plan_bad_db="${tmpdir}/plan-bad-db.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"vector","databases":["template1"]}]}' >"${plan_bad_db}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_bad_db}"
expect_status 'postgres-plan protected database exits 1' 1

plan_bad_name="${tmpdir}/plan-bad-name.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"vector","databases":["bad name"]}]}' >"${plan_bad_name}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_bad_name}"
expect_status 'postgres-plan invalid database name exits 1' 1

plan_empty_str_name="${tmpdir}/plan-empty-str-name.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"vector","databases":[""]}]}' >"${plan_empty_str_name}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_empty_str_name}"
expect_status 'postgres-plan empty-string database name exits 1' 1

plan_trailing_empty="${tmpdir}/plan-trailing-empty.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"vector","databases":["a",""]}]}' >"${plan_trailing_empty}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_trailing_empty}"
expect_status 'postgres-plan trailing empty database name exits 1' 1

plan_trailing_empty2="${tmpdir}/plan-trailing-empty2.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"vector","databases":["a","b",""]}]}' >"${plan_trailing_empty2}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_trailing_empty2}"
expect_status 'postgres-plan trailing empty database name (multi) exits 1' 1

plan_leading_empty="${tmpdir}/plan-leading-empty.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"vector","databases":["","b"]}]}' >"${plan_leading_empty}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_leading_empty}"
expect_status 'postgres-plan leading empty database name exits 1' 1

plan_pg_cron_two_db="${tmpdir}/plan-pg-cron-two-db.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"pg_cron","databases":["a","b"]}]}' >"${plan_pg_cron_two_db}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_pg_cron_two_db}"
expect_status 'postgres-plan pg_cron two databases exits 1' 1

plan_pg_cron_bgw="${tmpdir}/plan-pg-cron-bgw.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"pg_cron","databases":["a"]},{"capability":"pg_partman","databases":["b"],"scheduler":"pg_partman_bgw"}]}' >"${plan_pg_cron_bgw}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_pg_cron_bgw}"
expect_status 'postgres-plan pg_cron plus pg_partman_bgw exits 1' 1

plan_pg_cron_ok="${tmpdir}/plan-pg-cron-ok.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"pg_cron","databases":["a"]},{"capability":"pg_partman","databases":["b"],"scheduler":"pg_cron"}]}' >"${plan_pg_cron_ok}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_pg_cron_ok}"
expect_plan_partial 'postgres-plan pg_cron plus pg_cron scheduler exits 0' \
  'policy: apply-selected' \
  'selection: pg_cron databases=[a]' \
  'selection: pg_partman databases=[b] scheduler=pg_cron'

run_install postgres-plan --help
expect_status 'postgres-plan --help exits 0' 0

run_install backup --help
expect_status 'backup --help exits 0' 0

plan_empty_db="${tmpdir}/plan-empty-db.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"vector","databases":[]}]}' >"${plan_empty_db}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_empty_db}"
expect_status 'postgres-plan empty databases exits 1' 1

plan_sched_wrong="${tmpdir}/plan-sched-wrong.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"vector","databases":["a"],"scheduler":"pg_cron"}]}' >"${plan_sched_wrong}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_sched_wrong}"
expect_status 'postgres-plan scheduler on non-pg_partman exits 1' 1

plan_two_sched="${tmpdir}/plan-two-sched.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"pg_partman","databases":["a"],"scheduler":"pg_cron"},{"capability":"pg_partman","databases":["b"],"scheduler":"external"}]}' >"${plan_two_sched}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_two_sched}"
expect_status 'postgres-plan two schedulers exits 1' 1

plan_bad_json="${tmpdir}/plan-bad-json.json"
printf '%s' '{"policy":"preserve","selections":[]' >"${plan_bad_json}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_bad_json}"
expect_status 'postgres-plan malformed JSON exits 1' 1

plan_unknown_key="${tmpdir}/plan-unknown-key.json"
printf '%s' '{"policy":"apply-selected","selections":[{"capability":"vector","databases":["a"],"extra":"x"}]}' >"${plan_unknown_key}"
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_unknown_key}"
expect_status 'postgres-plan unknown selection key exits 1' 1

# postgres-plan never sources --config: config marker must not be created
rm -f "${sourced_marker}"
cat >"${plan_config}" <<EOF
echo CONFIG_SOURCED >"${sourced_marker}"
echo "${REDGRES_INSTALLER_CANARY}"
exit 99
EOF
run_install postgres-plan --config "${plan_config}" --extension-plan "${plan_apply_file}"
expect_plan_partial 'postgres-plan never sources --config' 'policy: apply-selected'
if [[ -e "${sourced_marker}" ]]; then
  fail 'postgres-plan --config was sourced'
else
  pass 'postgres-plan --config was not sourced'
fi
printf 'unused-plan-config\n' >"${plan_config}"


# --- OPS-005 update --dry-run skip matrix ---
run_install update --help
expect_status 'update --help exits 0' 0

run_install update --non-interactive --dry-run --release "${config_file}"
expect_update_partial 'update dry-run skip matrix exits 0'

run_install update
expect_status 'update without flags exits 1' 1

run_install update --non-interactive --release "${config_file}"
expect_status 'update without --dry-run exits 2' 2
case "${output}" in
  *'Inventory (read-only'*)
    fail 'update without --dry-run must not inventory'
    ;;
  *)
    pass 'update without --dry-run skips inventory'
    ;;
esac

run_install update --non-interactive --dry-run
expect_status 'update missing --release exits 1' 1

run_install update --non-interactive --dry-run --release "${tmpdir}/missing.tar.gz"
expect_status 'update --release missing path exits 1' 1

run_install update --non-interactive --dry-run --release "${tmpdir}"
expect_status 'update --release directory exits 1' 1

run_install update --non-interactive --dry-run --release "${config_file}" --config "${config_file}"
expect_status 'update unknown --config flag exits 1' 1

run_install update --non-interactive --dry-run --release "${config_file}" --mode existing-postgres
expect_status 'update unknown --mode flag exits 1' 1

# --- OPS-005 rollback --dry-run skip matrix ---
run_install rollback --help
expect_status 'rollback --help exits 0' 0

run_install rollback --non-interactive --dry-run --to rel-1
expect_rollback_partial 'rollback dry-run skip matrix exits 0'

run_install rollback
expect_status 'rollback without flags exits 1' 1

run_install rollback --non-interactive --to rel-1
expect_status 'rollback without --dry-run exits 2' 2
case "${output}" in
  *'Inventory (read-only'*)
    fail 'rollback without --dry-run must not inventory'
    ;;
  *)
    pass 'rollback without --dry-run skips inventory'
    ;;
esac

run_install rollback --non-interactive --dry-run
expect_status 'rollback missing --to exits 1' 1

run_install rollback --non-interactive --dry-run --to /abs
expect_status 'rollback --to absolute path exits 1' 1

run_install rollback --non-interactive --dry-run --to ..
expect_status 'rollback --to .. exits 1' 1

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
cat >"${unsafe_dir}/postgres-target" <<EOF
#!/usr/bin/env bash
: >"${process_canary}"
cat '${fixtures_dir}/postgres-17.11.version'
EOF
chmod 700 "${unsafe_dir}/postgres-target"
ln -s "${unsafe_dir}/postgres-target" "${unsafe_dir}/postgres"
if [[ -L "${unsafe_dir}/postgres" ]]; then
  DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
  DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
  DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
  EXTRA_PATH_PREFIX="${unsafe_dir}"
  run_install \
    --non-interactive \
    --dry-run \
    --mode existing-postgres \
    --redis-mode existing \
    --pgbouncer-mode existing
  expect_status_keyword 'symlinked PATH postgres exits 1' 1 'not trusted'
  if [[ -e "${process_canary}" ]]; then
    fail 'symlinked PATH postgres was executed'
  else
    pass 'symlinked PATH postgres was not executed'
  fi
else
  pass 'symlinked PATH postgres test skipped (filesystem has no symlink semantics)'
fi
rm -f "${unsafe_dir}/postgres" "${unsafe_dir}/postgres-target"

cat >"${unsafe_dir}/redis-server" <<EOF
#!/usr/bin/env bash
: >"${process_canary}"
cat '${fixtures_dir}/redis-8.2.0.version'
EOF
chmod 777 "${unsafe_dir}/redis-server"
unsafe_mode="$(/usr/bin/stat -Lc '%a' -- "${unsafe_dir}/redis-server")"
if redgres_test_mode_is_group_or_world_writable "${unsafe_mode}"; then
  DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
  DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
  DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
  EXTRA_PATH_PREFIX="${unsafe_dir}"
  run_install \
    --non-interactive \
    --dry-run \
    --mode existing-postgres \
    --redis-mode existing \
    --pgbouncer-mode existing
  expect_status_keyword 'writable PATH redis-server exits 1' 1 'not trusted'
  if [[ -e "${process_canary}" ]]; then
    fail 'writable PATH redis-server was executed'
  else
    pass 'writable PATH redis-server was not executed'
  fi
else
  pass 'writable PATH redis-server test skipped (filesystem has no Unix mode semantics)'
fi
rm -f "${unsafe_dir}/redis-server"

writable_parent="${unsafe_dir}/writable-parent"
mkdir -p "${writable_parent}"
cat >"${writable_parent}/postgres" <<EOF
#!/usr/bin/env bash
: >"${process_canary}"
cat '${fixtures_dir}/postgres-17.11.version'
EOF
chmod 700 "${writable_parent}/postgres"
chmod 777 "${writable_parent}"
writable_parent_mode="$(/usr/bin/stat -Lc '%a' -- "${writable_parent}")"
if redgres_test_mode_is_group_or_world_writable "${writable_parent_mode}"; then
  DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
  DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
  DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
  EXTRA_PATH_PREFIX="${writable_parent}"
  run_install \
    --non-interactive \
    --dry-run \
    --mode existing-postgres \
    --redis-mode existing \
    --pgbouncer-mode existing
  expect_status_keyword 'writable-parent PATH postgres exits 1' 1 'not trusted'
  if [[ -e "${process_canary}" ]]; then
    fail 'writable-parent PATH postgres was executed'
  else
    pass 'writable-parent PATH postgres was not executed'
  fi
else
  pass 'writable-parent PATH postgres test skipped (filesystem has no Unix mode semantics)'
fi
rm -rf "${writable_parent}"

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
DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-unparseable.version"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --redis-mode existing \
  --pgbouncer-mode existing
expect_status_keyword 'unparseable pgbouncer --version exits 1' 1 'unparseable'

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
