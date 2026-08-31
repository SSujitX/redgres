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

# Trusted-path tests cannot live below the world-writable /tmp ancestry on
# Linux. Keep disposable fixtures below the current user's trusted directory;
# fall back to the already-trusted checkout only when that is unavailable.
test_tmp_base="${HOME:-}"
if [[ -z "${test_tmp_base}" || ! -d "${test_tmp_base}" ]]; then
  test_tmp_base="${deploy_dir%/*}"
fi
tmpdir="$(mktemp -d "${test_tmp_base}/redgres-install-test.XXXXXX")"
stub_dir="${tmpdir}/stubs"
detect_dir="${tmpdir}/detect"
unsafe_dir="${tmpdir}/unsafe"
stub_log="${tmpdir}/stub.log"
process_canary="${tmpdir}/process-canary"
bootstrap_helper_dir="${tmpdir}/bootstrap-helpers"
bootstrap_helper_log="${tmpdir}/bootstrap-helper.log"
source_tree_marker="${tmpdir}/source-tree-sourced"
config_file="${tmpdir}/install.env"
startup_env_file="${tmpdir}/startup.env"
unknown_config_file="${tmpdir}/unknown-install.env"
duplicate_config_file="${tmpdir}/duplicate-install.env"
malformed_config_file="${tmpdir}/malformed-install.env"
literal_config_file="${tmpdir}/literal-install.env"
oversized_config_file="${tmpdir}/oversized-install.env"
nul_config_file="${tmpdir}/nul-install.env"
release_file="${tmpdir}/redgres_1.0.0_linux_amd64.tar.gz"
checksum_file="${tmpdir}/SHA256SUMS"
plan_file="${tmpdir}/postgres-extensions.json"
sourced_marker="${tmpdir}/config-sourced"
mkdir -p "${stub_dir}" "${detect_dir}" "${unsafe_dir}" "${bootstrap_helper_dir}"
: >"${stub_log}"
: >"${bootstrap_helper_log}"
original_path="${PATH}"

cleanup() {
  case "${tmpdir}" in
    "${test_tmp_base}"/redgres-install-test.*) rm -rf -- "${tmpdir}" ;;
    *) printf 'refusing unsafe installer-test cleanup target\n' >&2 ;;
  esac
}
trap cleanup EXIT

# Canary must never appear in installer output (env is not a log dump).
export REDGRES_INSTALLER_CANARY='rg-canary-do-not-print-7f2c'

cat >"${startup_env_file}" <<EOF
echo CONFIG_SOURCED >"${sourced_marker}"
echo "${REDGRES_INSTALLER_CANARY}"
exit 99
EOF

cat >"${config_file}" <<'EOF'
# Installer lifecycle values are data, not shell.
POSTGRES_MODE=fresh
POSTGRES_MAJOR=18
PGBOUNCER_MODE=disabled
POSTGRES_EXTENSION_POLICY=preserve
POSTGRES_EXTENSION_PLAN_FILE=
EOF

printf '%s\n' "UNSUPPORTED_KEY=${REDGRES_INSTALLER_CANARY}" >"${unknown_config_file}"
printf '%s\n' 'POSTGRES_MODE=fresh' 'POSTGRES_MODE=existing' >"${duplicate_config_file}"
printf '%s\n' 'export POSTGRES_MODE=fresh' >"${malformed_config_file}"
printf '%s\n' "POSTGRES_EXTENSION_PLAN_FILE=\$(printf owned >'${sourced_marker}')" >"${literal_config_file}"
/usr/bin/head -c 65537 /dev/zero | /usr/bin/tr '\000' 'A' >"${oversized_config_file}"
printf 'POSTGRES_MODE=fresh\000' >"${nul_config_file}"
printf '%s' 'redgres-release-fixture-v1' >"${release_file}"
release_digest="$(/usr/bin/sha256sum "${release_file}")"
release_digest="${release_digest%% *}"
printf '%s  %s\n' "${release_digest}" "${release_file##*/}" >"${checksum_file}"

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

write_bootstrap_helper_stub() {
  local name="$1"
  cat >"${bootstrap_helper_dir}/${name}" <<STUB
#!/usr/bin/env bash
printf '%s\n' "${name}" >>"${bootstrap_helper_log}"
printf 'FORBIDDEN ambient bootstrap helper invoked: %s\n' "${name}" >&2
exit 97
STUB
  chmod +x "${bootstrap_helper_dir}/${name}"
}

for _helper in dirname cat stat; do
  write_bootstrap_helper_stub "${_helper}"
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
if [[ -n "\${REDGRES_INSTALLER_CANARY-}" || -n "\${BASH_ENV-}" || -n "\${ENV-}" || -n "\${LD_PRELOAD-}" || -n "\${LD_LIBRARY_PATH-}" || -n "\${LD_AUDIT-}" ]]; then
  printf 'inventory child inherited ambient environment\n' >'${process_canary}'
  exit 96
fi
cat '${fixture}'
exit 0
STUB
  chmod +x "${detect_dir}/${name}"
}

DETECT_POSTGRES=''
DETECT_REDIS=''
DETECT_PGBOUNCER=''
EXTRA_PATH_PREFIX=''
INSTALL_PATH_OVERRIDE=''

export PATH="${stub_dir}:${PATH}"

output=''
status=0

run_install() {
  : >"${stub_log}"
  : >"${bootstrap_helper_log}"
  rm -f "${sourced_marker}"
  rm -f "${process_canary}"
  rm -f "${source_tree_marker}"
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
  local installer_under_test="${INSTALL_PATH_OVERRIDE:-${install_sh}}"
  set +e
  output="$(PATH="${EXTRA_PATH_PREFIX:+${EXTRA_PATH_PREFIX}:}${installer_path}" "${BASH}" -p "${installer_under_test}" "$@" 2>&1)"
  status=$?
  set -e
  EXTRA_PATH_PREFIX=''
  INSTALL_PATH_OVERRIDE=''
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
  if [[ -s "${bootstrap_helper_log}" ]]; then
    fail "${name}: ambient bootstrap helper invoked: $(tr '\n' ' ' <"${bootstrap_helper_log}")"
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
    'release: checksum-verified (not extracted)' \
    'checksum: verified (adjacent SHA256SUMS; signature/provenance not verified)' \
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

set +e
output="$(
  builtin cd -- "${deploy_dir}/.." &&
    PATH="${bootstrap_helper_dir}:${installer_path}" \
    BASH_ENV="${startup_env_file}" \
    ./deploy/install.sh --help
  2>&1
)"
status=$?
set -e
if [[ "${status}" -eq 0 && ! -e "${sourced_marker}" && ! -s "${bootstrap_helper_log}" ]]; then
  pass 'direct invocation ignores BASH_ENV and ambient PATH helpers'
else
  fail "direct invocation did not establish privileged bootstrap: exit=${status}: ${output}"
fi

# --- trusted bootstrap: ambient PATH is data, not executable authority ---
EXTRA_PATH_PREFIX="${bootstrap_helper_dir}"
run_install --help
expect_status 'malicious ambient PATH cannot hijack bootstrap helpers' 0

DETECT_POSTGRES="${fixtures_dir}/postgres-17.11.version"
DETECT_REDIS="${fixtures_dir}/redis-8.2.0.version"
DETECT_PGBOUNCER="${fixtures_dir}/pgbouncer-1.24.1.version"
EXTRA_PATH_PREFIX="${bootstrap_helper_dir}"
run_install \
  --non-interactive \
  --dry-run \
  --mode existing-postgres \
  --expect-postgres-major 17 \
  --redis-mode existing \
  --expect-redis-series 8.2 \
  --pgbouncer-mode existing
expect_status_and_stages 'sanitized runtime PATH inventories captured host-search fixtures' \
  'postgres: detected=postgres (PostgreSQL) 17.11 major=17 expect=17 result=ok' \
  'redis: detected=Redis server v=8.2.0 sha=00000000:0 malloc=libc bits=64 build=0 series=8.2 expect=8.2 result=ok' \
  'pgbouncer: detected=PgBouncer 1.24.1 result=recorded'
assert_no_mutation 'sanitized inventory child receives no ambient environment'

# Validate copied trees only; never weaken or rewrite the repository checkout.
unsafe_source_root="${tmpdir}/unsafe-source"
unsafe_source_deploy="${unsafe_source_root}/deploy"
mkdir -p "${unsafe_source_deploy}"
cp -R "${deploy_dir}/." "${unsafe_source_deploy}/"
printf "\nprintf '' >'%s'\n" "${source_tree_marker}" >>"${unsafe_source_deploy}/lib/common.sh"
chmod 777 "${unsafe_source_deploy}/lib"
unsafe_source_mode="$(/usr/bin/stat -Lc '%a' -- "${unsafe_source_deploy}/lib")"
if redgres_test_mode_is_group_or_world_writable "${unsafe_source_mode}"; then
  INSTALL_PATH_OVERRIDE="${unsafe_source_deploy}/install.sh"
  run_install --help
  if [[ "${status}" -eq 1 && ! -e "${source_tree_marker}" ]]; then
    pass 'writable installer source tree is rejected before sourcing'
  else
    fail "writable installer source tree was not rejected before sourcing: exit=${status}: ${output}"
  fi
else
  pass 'writable installer source tree test skipped (filesystem has no Unix mode semantics)'
fi

symlink_source_root="${tmpdir}/symlink-source"
symlink_source_deploy="${symlink_source_root}/real-deploy"
symlink_source_entry="${symlink_source_root}/linked-deploy"
mkdir -p "${symlink_source_deploy}"
cp -R "${deploy_dir}/." "${symlink_source_deploy}/"
printf "\nprintf '' >'%s'\n" "${source_tree_marker}" >>"${symlink_source_deploy}/lib/common.sh"
if ln -s "${symlink_source_deploy}" "${symlink_source_entry}" 2>/dev/null && [[ -L "${symlink_source_entry}" ]]; then
  INSTALL_PATH_OVERRIDE="${symlink_source_entry}/install.sh"
  run_install --help
  if [[ "${status}" -eq 1 && ! -e "${source_tree_marker}" ]]; then
    pass 'symlinked installer source tree is rejected before sourcing'
  else
    fail "symlinked installer source tree was not rejected before sourcing: exit=${status}: ${output}"
  fi
else
  pass 'symlinked installer source tree test skipped (filesystem has no symlink semantics)'
fi

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
  'pgbouncer: detected=PgBouncer 1.24.1 result=recorded' \
  'Cloudflare inventory (read-only PATH scan; not live API):' \
  'TLS inventory (read-only PATH scan; no secret read):'

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
  --config "${unknown_config_file}"
expect_status 'main dry-run rejects unknown config key' 1

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --config "${duplicate_config_file}"
expect_status 'main dry-run rejects duplicate config key' 1

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --config "${malformed_config_file}"
expect_status 'main dry-run rejects shell-style config syntax' 1

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --config "${literal_config_file}"
expect_status 'main dry-run treats config command substitution as literal data' 0

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --config "${oversized_config_file}"
expect_status 'main dry-run rejects oversized config' 1

run_install \
  --non-interactive \
  --dry-run \
  --mode fresh-postgres \
  --postgres-version 18 \
  --redis-mode fresh \
  --redis-version 8.8 \
  --pgbouncer-mode disabled \
  --config "${nul_config_file}"
expect_status 'main dry-run rejects config containing NUL' 1

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

run_install update --non-interactive --dry-run --release "${release_file}"
expect_update_partial 'update dry-run skip matrix exits 0'

printf '%064d  %s\n' 0 "${release_file##*/}" >"${checksum_file}"
run_install update --non-interactive --dry-run --release "${release_file}"
expect_status 'update dry-run rejects checksum mismatch' 1

rm -f "${checksum_file}"
run_install update --non-interactive --dry-run --release "${release_file}"
expect_status 'update dry-run requires adjacent SHA256SUMS' 1

printf '%s  %s\n%s  %s\n' "${release_digest}" "${release_file##*/}" "${release_digest}" "${release_file##*/}" >"${checksum_file}"
run_install update --non-interactive --dry-run --release "${release_file}"
expect_status 'update dry-run rejects duplicate checksum entry' 1

printf '%s  %s\n' "${release_digest}" "${release_file##*/}" >"${checksum_file}"

run_install update
expect_status 'update without flags exits 1' 1

# Live update under REDGRES_OPT_ROOT (fixture tarball with stub binary + VERSION).
live_stage="${tmpdir}/live-release-stage"
live_opt="${tmpdir}/opt-redgres"
live_unit="${tmpdir}/redgres.service"
mkdir -p "${live_stage}"
printf '#!/bin/sh\necho stub\n' >"${live_stage}/redgres"
chmod 0755 "${live_stage}/redgres"
printf '1.0.0\n' >"${live_stage}/VERSION"
live_release="${tmpdir}/redgres_live_1.0.0_linux_amd64.tar.gz"
/usr/bin/tar -C "${live_stage}" -czf "${live_release}" redgres VERSION
live_digest="$(/usr/bin/sha256sum "${live_release}")"
live_digest="${live_digest%% *}"
printf '%s  %s\n' "${live_digest}" "${live_release##*/}" >"${checksum_file}"

REDGRES_OPT_ROOT="${live_opt}" \
REDGRES_UNIT_PATH="${live_unit}" \
REDGRES_SKIP_HEALTHZ=1 \
  run_install update --non-interactive --release "${live_release}"
expect_status 'live update under REDGRES_OPT_ROOT exits 0' 0
case "${output}" in
  *'result=applied'*) pass 'live update prints result=applied' ;;
  *) fail 'live update must print result=applied' ;;
esac
case "${output}" in
  *'Inventory (read-only'*)
    fail 'live update must not inventory'
    ;;
  *)
    pass 'live update skips inventory'
    ;;
esac
if [[ -x "${live_opt}/releases/1.0.0/redgres" && -f "${live_opt}/releases/1.0.0/VERSION" ]]; then
  pass 'live update writes releases/1.0.0'
else
  fail 'live update missing releases/1.0.0'
fi
if [[ -L "${live_opt}/current" ]]; then
  cur="$(readlink -f "${live_opt}/current" || true)"
  case "${cur}" in
    */releases/1.0.0) pass 'live update current symlink points at 1.0.0' ;;
    *) fail "live update current is '${cur}', want .../releases/1.0.0" ;;
  esac
elif [[ -x "${live_opt}/current/redgres" ]]; then
  pass 'live update current present (filesystem has no symlink semantics)'
else
  fail 'live update missing current'
fi

vault_retry_rc=0
(
  retry_stage="${tmpdir}/vault-retry-stage"
  retry_opt="${tmpdir}/vault-retry-opt"
  retry_env="${tmpdir}/vault-retry.env"
  retry_manifest="${tmpdir}/vault-retry.adopt"
  retry_canonical="${tmpdir}/vault-retry-canonical"
  mkdir -p "${retry_stage}"
  printf '#!/bin/sh\necho retry-stub\n' >"${retry_stage}/redgres"
  chmod 0755 "${retry_stage}/redgres"
  printf '1.0.3\n' >"${retry_stage}/VERSION"
  retry_release="${tmpdir}/redgres_retry_1.0.3_linux_amd64.tar.gz"
  /usr/bin/tar -C "${retry_stage}" -czf "${retry_release}" redgres VERSION
  retry_digest="$(/usr/bin/sha256sum "${retry_release}")"
  retry_digest="${retry_digest%% *}"
  printf '%s  %s\n' "${retry_digest}" "${retry_release##*/}" >"${checksum_file}"
  : >"${retry_env}"
  if (
    # shellcheck source=../lib/common.sh
    source "${deploy_dir}/lib/common.sh"
    # shellcheck source=../lib/mutate.sh
    source "${deploy_dir}/lib/mutate.sh"
    # shellcheck source=../lib/app_install.sh
    source "${deploy_dir}/lib/app_install.sh"
    # shellcheck source=../lib/release.sh
    source "${deploy_dir}/lib/release.sh"
    redgres_adopt_legacy_vault_secret() { return 1; }
    REDGRES_OPT_ROOT="${retry_opt}" REDGRES_UNIT_PATH="${tmpdir}/vault-retry.service" REDGRES_SKIP_HEALTHZ=1 \
      REDGRES_VAULT_ADOPTION_ENV_FILE="${retry_env}" REDGRES_VAULT_ADOPTION_CANONICAL_DIR="${retry_canonical}" \
      REDGRES_VAULT_ADOPTION_MANIFEST="${retry_manifest}" redgres_update_apply "${retry_release}"
  ); then
    exit 1
  fi
  [[ ! -e "${retry_opt}/releases/1.0.3" ]]
  (
    # shellcheck source=../lib/common.sh
    source "${deploy_dir}/lib/common.sh"
    # shellcheck source=../lib/mutate.sh
    source "${deploy_dir}/lib/mutate.sh"
    # shellcheck source=../lib/app_install.sh
    source "${deploy_dir}/lib/app_install.sh"
    # shellcheck source=../lib/release.sh
    source "${deploy_dir}/lib/release.sh"
    redgres_adopt_legacy_vault_secret() { return 0; }
    REDGRES_OPT_ROOT="${retry_opt}" REDGRES_UNIT_PATH="${tmpdir}/vault-retry.service" REDGRES_SKIP_HEALTHZ=1 \
      REDGRES_VAULT_ADOPTION_ENV_FILE="${retry_env}" REDGRES_VAULT_ADOPTION_CANONICAL_DIR="${retry_canonical}" \
      REDGRES_VAULT_ADOPTION_MANIFEST="${retry_manifest}" redgres_update_apply "${retry_release}" >/dev/null
  )
  grep -qx '1.0.3' "${retry_opt}/current/VERSION"
) || vault_retry_rc=$?
if [[ "${vault_retry_rc}" -eq 0 ]]; then
  pass 'vault adoption failure leaves no release directory and same-version retry succeeds'
else
  fail "vault adoption transactional retry (rc=${vault_retry_rc})"
fi

packaged_upgrade_rc=0
(
  packaged_root="${tmpdir}/packaged-upgrade"
  packaged_stage="${packaged_root}/stage"
  packaged_opt="${tmpdir}/packaged-opt"
  mkdir -p "${packaged_stage}/installer"
  cp -a "${deploy_dir}/install.sh" "${deploy_dir}/lib" "${deploy_dir}/systemd" "${packaged_stage}/installer/"
  printf '#!/bin/sh\necho packaged-stub\n' >"${packaged_stage}/redgres"
  chmod 0755 "${packaged_stage}/redgres" "${packaged_stage}/installer/install.sh"
  printf '1.0.2\n' >"${packaged_stage}/VERSION"
  packaged_release="${packaged_root}/redgres_1.0.2_linux_amd64.tar.gz"
  /usr/bin/tar -C "${packaged_stage}" -czf "${packaged_release}" redgres VERSION installer
  packaged_digest="$(/usr/bin/sha256sum "${packaged_release}")"
  packaged_digest="${packaged_digest%% *}"
  printf '%s  %s\n' "${packaged_digest}" "${packaged_release##*/}" >"${packaged_root}/SHA256SUMS"
  REDGRES_OPT_ROOT="${packaged_opt}" REDGRES_UNIT_PATH="${tmpdir}/packaged-redgres.service" REDGRES_SKIP_HEALTHZ=1 \
    /bin/bash -p "${packaged_stage}/installer/install.sh" update --non-interactive --release "${packaged_release}" >/dev/null
  grep -qx '1.0.2' "${packaged_opt}/current/VERSION"
) || packaged_upgrade_rc=$?
if [[ "${packaged_upgrade_rc}" -eq 0 ]]; then
  pass 'checksummed release-packaged dispatcher executes the canonical update transaction'
else
  fail "packaged dispatcher transaction (rc=${packaged_upgrade_rc})"
fi

# Second version for rollback target
live_stage2="${tmpdir}/live-release-stage-2"
mkdir -p "${live_stage2}"
printf '#!/bin/sh\necho stub2\n' >"${live_stage2}/redgres"
chmod 0755 "${live_stage2}/redgres"
printf '1.0.1\n' >"${live_stage2}/VERSION"
live_release2="${tmpdir}/redgres_live_1.0.1_linux_amd64.tar.gz"
/usr/bin/tar -C "${live_stage2}" -czf "${live_release2}" redgres VERSION
live_digest2="$(/usr/bin/sha256sum "${live_release2}")"
live_digest2="${live_digest2%% *}"
printf '%s  %s\n' "${live_digest2}" "${live_release2##*/}" >"${checksum_file}"
REDGRES_OPT_ROOT="${live_opt}" \
REDGRES_UNIT_PATH="${live_unit}" \
REDGRES_SKIP_HEALTHZ=1 \
  run_install update --non-interactive --release "${live_release2}"
expect_status 'live update 1.0.1 exits 0' 0

REDGRES_OPT_ROOT="${live_opt}" \
REDGRES_UNIT_PATH="${live_unit}" \
REDGRES_SKIP_HEALTHZ=1 \
  run_install rollback --non-interactive --to 1.0.0
expect_status 'live rollback to 1.0.0 exits 0' 0
case "${output}" in
  *'result=applied'*) pass 'live rollback prints result=applied' ;;
  *) fail 'live rollback must print result=applied' ;;
esac
cur="$(readlink -f "${live_opt}/current" || true)"
if [[ -L "${live_opt}/current" ]]; then
  case "${cur}" in
    */releases/1.0.0) pass 'live rollback points current at 1.0.0' ;;
    *) fail "live rollback current is '${cur}', want .../releases/1.0.0" ;;
  esac
elif [[ -f "${live_opt}/current/VERSION" ]]; then
  rolled="$(tr -d '\r\n' <"${live_opt}/current/VERSION")"
  if [[ "${rolled}" == "1.0.0" ]]; then
    pass 'live rollback current VERSION is 1.0.0 (no symlink semantics)'
  else
    fail "live rollback current VERSION is '${rolled}', want 1.0.0"
  fi
else
  fail "live rollback current missing ('${cur}')"
fi

rollback_runtime_rc=0
(
  # shellcheck source=../lib/release.sh
  source "${deploy_dir}/lib/release.sh"
  runtime_root="${tmpdir}/rollback-runtime-root"
  release_dir="${tmpdir}/rollback-runtime-release"
  export REDGRES_RUNTIME_ROOT_PREFIX="${runtime_root}"
  mkdir -p \
    "${release_dir}" \
    "${runtime_root}/etc/redgres" \
    "${runtime_root}/usr/libexec/redgres" \
    "${runtime_root}/etc/letsencrypt/renewal-hooks/deploy" \
    "${runtime_root}/etc/systemd/system"
  printf '%s\n' 'REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres/tls-issue.result' 'KEEP_ROTATED=old' >"${runtime_root}/etc/redgres/redgres.env"
  printf 'v1-helper\n' >"${runtime_root}/usr/libexec/redgres/issue-tls.sh"
  printf 'v1-hook\n' >"${runtime_root}/etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh"
  printf 'v1-service\n' >"${runtime_root}/etc/systemd/system/redgres-tls-issue.service"
  redgres_snapshot_rollback_runtime "${release_dir}"
  printf '%s\n' 'REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres-tls/issue.result' 'KEEP_ROTATED=new' >"${runtime_root}/etc/redgres/redgres.env"
  printf 'v2-helper\n' >"${runtime_root}/usr/libexec/redgres/issue-tls.sh"
  printf 'v2-hook\n' >"${runtime_root}/etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh"
  printf 'v2-service\n' >"${runtime_root}/etc/systemd/system/redgres-tls-issue.service"
  printf 'v2-path\n' >"${runtime_root}/etc/systemd/system/redgres-tls-issue.path"
  redgres_restore_rollback_runtime "${release_dir}"
  grep -qx 'REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres/tls-issue.result' "${runtime_root}/etc/redgres/redgres.env"
  grep -qx 'KEEP_ROTATED=new' "${runtime_root}/etc/redgres/redgres.env"
  grep -qx 'v1-helper' "${runtime_root}/usr/libexec/redgres/issue-tls.sh"
  grep -qx 'v1-hook' "${runtime_root}/etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh"
  grep -qx 'v1-service' "${runtime_root}/etc/systemd/system/redgres-tls-issue.service"
  [[ ! -e "${runtime_root}/etc/systemd/system/redgres-tls-issue.path" ]]
  rm -f "${release_dir}/.rollback-runtime/issue-service"
  printf '%s\n' 'REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres-tls/issue.result' 'KEEP_ROTATED=three' >"${runtime_root}/etc/redgres/redgres.env"
  if redgres_restore_rollback_runtime "${release_dir}"; then
    exit 1
  fi
  grep -qx 'KEEP_ROTATED=three' "${runtime_root}/etc/redgres/redgres.env"

  printf 'v1-service\n' >"${release_dir}/.rollback-runtime/issue-service"
  printf '%s\n' 'REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres-tls/issue.result' 'KEEP_ROTATED=four' >"${runtime_root}/etc/redgres/redgres.env"
  printf 'v4-helper\n' >"${runtime_root}/usr/libexec/redgres/issue-tls.sh"
  printf 'v4-hook\n' >"${runtime_root}/etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh"
  printf 'v4-service\n' >"${runtime_root}/etc/systemd/system/redgres-tls-issue.service"
  printf 'v4-path\n' >"${runtime_root}/etc/systemd/system/redgres-tls-issue.path"
  export REDGRES_RUNTIME_RESTORE_FAIL_AFTER=2
  if redgres_restore_rollback_runtime "${release_dir}"; then
    exit 1
  fi
  grep -qx 'REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres-tls/issue.result' "${runtime_root}/etc/redgres/redgres.env"
  grep -qx 'KEEP_ROTATED=four' "${runtime_root}/etc/redgres/redgres.env"
  grep -qx 'v4-helper' "${runtime_root}/usr/libexec/redgres/issue-tls.sh"
  grep -qx 'v4-hook' "${runtime_root}/etc/letsencrypt/renewal-hooks/deploy/redgres-copy-certs.sh"
  grep -qx 'v4-service' "${runtime_root}/etc/systemd/system/redgres-tls-issue.service"
  grep -qx 'v4-path' "${runtime_root}/etc/systemd/system/redgres-tls-issue.path"
) || rollback_runtime_rc=$?
if [[ "${rollback_runtime_rc}" -eq 0 ]]; then
  pass 'rollback runtime snapshot restores atomically and rejects incomplete snapshots before mutation'
else
  fail "rollback runtime snapshot transaction (rc=${rollback_runtime_rc})"
fi

update_order_rc=0
(
  release_lib="${deploy_dir}/lib/release.sh"
  quiesce_line="$(grep -n 'systemctl stop redgres-tls-issue.path' "${release_lib}" | head -n1 | cut -d: -f1)"
  runtime_line="$(grep -n 'redgres_install_domain_runtime || redgres_die' "${release_lib}" | head -n1 | cut -d: -f1)"
  restart_line="$(grep -n 'systemctl restart redgres.service || {' "${release_lib}" | head -n1 | cut -d: -f1)"
  adoption_line="$(grep -n 'redgres_adopt_legacy_vault_secret || redgres_die' "${release_lib}" | head -n1 | cut -d: -f1)"
  dest_create_line="$(grep -n 'redgres_chmod_opt_layout "${dest}"' "${release_lib}" | head -n1 | cut -d: -f1)"
  [[ -n "${quiesce_line}" && -n "${runtime_line}" && -n "${restart_line}" && -n "${adoption_line}" && -n "${dest_create_line}" ]]
  [[ "${quiesce_line}" -lt "${runtime_line}" && "${runtime_line}" -lt "${restart_line}" ]]
  [[ "${adoption_line}" -lt "${dest_create_line}" && "${dest_create_line}" -lt "${quiesce_line}" ]]
) || update_order_rc=$?
if [[ "${update_order_rc}" -eq 0 ]]; then
  pass 'update quiesces old app/helper before installing matched runtime and restarting'
else
  fail "update runtime ordering (rc=${update_order_rc})"
fi

# Restore checksum fixture for remaining dry-run negative tests that use release_file.
printf '%s  %s\n' "${release_digest}" "${release_file##*/}" >"${checksum_file}"

run_install update --non-interactive --dry-run
expect_status 'update missing --release exits 1' 1

run_install update --non-interactive --dry-run --release "${tmpdir}/missing.tar.gz"
expect_status 'update --release missing path exits 1' 1

run_install update --non-interactive --dry-run --release "${tmpdir}"
expect_status 'update --release directory exits 1' 1

run_install update --non-interactive --dry-run --release "${release_file}" --config "${config_file}"
expect_status 'update unknown --config flag exits 1' 1

run_install update --non-interactive --dry-run --release "${release_file}" --mode existing-postgres
expect_status 'update unknown --mode flag exits 1' 1

# --- OPS-005 rollback --dry-run skip matrix ---
run_install rollback --help
expect_status 'rollback --help exits 0' 0

run_install rollback --non-interactive --dry-run --to rel-1
expect_rollback_partial 'rollback dry-run skip matrix exits 0'

run_install rollback
expect_status 'rollback without flags exits 1' 1

run_install rollback --non-interactive --to missing-rel
expect_status 'live rollback missing target exits 1' 1
case "${output}" in
  *'Inventory (read-only'*)
    fail 'live rollback must not inventory'
    ;;
  *)
    pass 'live rollback skips inventory'
    ;;
esac

run_install rollback --non-interactive --dry-run
expect_status 'rollback missing --to exits 1' 1

run_install rollback --non-interactive --dry-run --to /abs
expect_status 'rollback --to absolute path exits 1' 1

run_install rollback --non-interactive --dry-run --to ..
expect_status 'rollback --to .. exits 1' 1

# --- S2 OS gate + Redis digest pins (no host mutation) ---
s2_lib_src() {
  # shellcheck disable=SC1091
  source "${deploy_dir}/lib/common.sh"
  # shellcheck disable=SC1091
  source "${deploy_dir}/lib/pins.sh"
  # shellcheck disable=SC1091
  source "${deploy_dir}/lib/mutate.sh"
  source "${deploy_dir}/lib/app_install.sh"
}

postgres_control_rc=0
postgres_control_err="$(
  s2_lib_src
  test_pass='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef'
  redgres_assert_postgres_admin_password "${test_pass}" || exit 1
  if redgres_assert_postgres_admin_password 'bad$redgres$; SELECT 1; --'; then exit 1; fi
  if redgres_assert_postgres_admin_password $'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\nB'; then exit 1; fi
  sql="$(redgres_postgres_control_sql "${test_pass}")" || exit 1
  printf '%s\n' "${sql}" | grep -q 'ALTER ROLE redgres_admin WITH LOGIN CREATEDB CREATEROLE NOSUPERUSER NOREPLICATION PASSWORD' || exit 1
  printf '%s\n' "${sql}" | grep -q 'CREATE ROLE redgres_admin LOGIN CREATEDB CREATEROLE NOSUPERUSER NOREPLICATION PASSWORD' || exit 1
  printf '%s\n' "${sql}" | grep -q 'CREATE DATABASE database_console_vault OWNER redgres_admin' || exit 1
  printf '%s\n' "${sql}" | grep -q 'REVOKE CONNECT ON DATABASE database_console_vault FROM PUBLIC' || exit 1
  printf '%s\n' "${sql}" | grep -q 'CREATE TABLE IF NOT EXISTS public.project_credentials' || exit 1
  printf '%s\n' "${sql}" | grep -q 'ALTER TABLE public.project_credentials OWNER TO redgres_admin' || exit 1
  printf '%s\n' "${sql}" | grep -q "${test_pass}" || exit 1
  sql_capture="${tmpdir}/postgres-control.sql"
  redgres_postgres_control_exec() { /usr/bin/cat >"${sql_capture}"; }
  redgres_run_postgres_control_sql "${test_pass}" || exit 1
  grep -q 'CREATE DATABASE database_console_vault OWNER redgres_admin' "${sql_capture}" || exit 1
  redgres_postgres_control_exec() { /usr/bin/cat >/dev/null; return 23; }
  if redgres_run_postgres_control_sql "${test_pass}"; then exit 1; fi
  env_file="${tmpdir}/postgres-vault.env"
  printf '%s\n' 'REDGRES_ENVIRONMENT=production' >"${env_file}"
  redgres_assert_fresh_vault_env_path "${env_file}" || exit 1
  printf '%s\n' 'REDGRES_LEGACY_VAULT_SECRET_FILE=/etc/redgres/secrets/legacy-vault-secret' >"${env_file}.canonical"
  if redgres_assert_fresh_vault_env_path "${env_file}.canonical"; then exit 1; fi
  printf '%s\n' 'REDGRES_LEGACY_VAULT_SECRET_FILE=/custom/legacy-secret' >"${env_file}.keep"
  if redgres_assert_fresh_vault_env_path "${env_file}.keep"; then exit 1; fi
  vault_dir="${tmpdir}/vault-secrets"
  chown_log="${tmpdir}/vault-chown.log"
  redgres_chown_legacy_vault_secret_dir() { printf 'dir:%s\n' "$1" >>"${chown_log}"; }
  redgres_chown_legacy_vault_secret() { printf '%s\n' "$1" >>"${chown_log}"; }
  redgres_ensure_legacy_vault_secret "${vault_dir}" || exit 1
  vault_secret="${vault_dir}/legacy-vault-secret"
  vault_marker="${vault_dir}/legacy-vault-secret.managed"
  [[ -s "${vault_secret}" && ! -L "${vault_secret}" ]] || exit 1
  [[ "$(cat "${vault_marker}")" == 'managed-by-redgres-installer-v1' ]] || exit 1
  original_hash="$(sha256sum "${vault_secret}" | cut -d' ' -f1)" || exit 1
  redgres_ensure_legacy_vault_secret "${vault_dir}" || exit 1
  [[ "$(sha256sum "${vault_secret}" | cut -d' ' -f1)" == "${original_hash}" ]] || exit 1
  [[ "$(wc -l <"${chown_log}")" -ge 8 ]] || exit 1
  grep -q "^dir:${vault_dir}$" "${chown_log}" || exit 1
  redgres_vault_source_metadata() { printf '%s\n' '0:0:600'; }
  modern_release="${tmpdir}/vault-modern-release"
  legacy_release="${tmpdir}/vault-legacy-release"
  mkdir -p "${modern_release}" "${legacy_release}"
  printf '%s\n' '1.1.9' >"${modern_release}/VERSION"
  printf '%s\n' '1.1.8' >"${legacy_release}/VERSION"
  unit_lines="$(redgres_app_unit_runtime_lines "${modern_release}/redgres" "${vault_secret}" "${vault_marker}")" || exit 1
  [[ "${unit_lines}" == *"LoadCredential=legacy-vault-secret:${vault_secret}"* ]] || exit 1
  [[ "${unit_lines}" == *"ExecStart=/usr/bin/env REDGRES_ENVIRONMENT=production REDGRES_LEGACY_VAULT_SECRET_FILE=%d/legacy-vault-secret ${modern_release}/redgres serve"* ]] || exit 1
  inherited='source-path'
  effective="$(env REDGRES_ENVIRONMENT=development REDGRES_LEGACY_VAULT_SECRET_FILE="${inherited}" /usr/bin/env REDGRES_ENVIRONMENT=production REDGRES_LEGACY_VAULT_SECRET_FILE='%d/legacy-vault-secret' /bin/sh -c 'printf "%s:%s" "$REDGRES_ENVIRONMENT" "$REDGRES_LEGACY_VAULT_SECRET_FILE"')" || exit 1
  [[ "${effective}" == 'production:%d/legacy-vault-secret' ]] || exit 1
  custom_lines="$(redgres_app_unit_runtime_lines "${modern_release}/redgres" "${vault_secret}" "${vault_marker}")" || exit 1
  [[ "${custom_lines}" == *'REDGRES_LEGACY_VAULT_SECRET_FILE=%d/legacy-vault-secret'* ]] || exit 1
  legacy_lines="$(redgres_app_unit_runtime_lines "${legacy_release}/redgres" "${vault_secret}" "${vault_marker}")" || exit 1
  [[ "${legacy_lines}" == *'RuntimeDirectory=redgres-vault'* ]] || exit 1
  [[ "${legacy_lines}" == *'ExecStartPre=/usr/bin/install -m 0600 %d/legacy-vault-secret /run/redgres-vault/legacy-vault-secret'* ]] || exit 1
  [[ "${legacy_lines}" == *"REDGRES_LEGACY_VAULT_SECRET_FILE=/run/redgres-vault/legacy-vault-secret ${legacy_release}/redgres serve"* ]] || exit 1
  adoption_lines="$(redgres_app_unit_runtime_lines "${modern_release}/redgres" "${vault_secret}" "${vault_marker}.absent")" || exit 1
  [[ "${adoption_lines}" == "ExecStart=/usr/bin/env REDGRES_ENVIRONMENT=production REDGRES_LEGACY_VAULT_SECRET_FILE= ${modern_release}/redgres serve" ]] || exit 1
  redgres_vault_source_metadata() { printf '%s\n' '1000:1000:600'; }
  untrusted_lines="$(redgres_app_unit_runtime_lines "${modern_release}/redgres" "${vault_secret}" "${vault_marker}")" || exit 1
  [[ "${untrusted_lines}" == "ExecStart=/usr/bin/env REDGRES_ENVIRONMENT=production REDGRES_LEGACY_VAULT_SECRET_FILE= ${modern_release}/redgres serve" ]] || exit 1
)" 2>&1 || postgres_control_rc=$?
if [[ "${postgres_control_rc}" -eq 0 ]]; then
  pass 'fresh PostgreSQL control role and encrypted vault satisfy create-database prerequisites'
else
  fail "fresh PostgreSQL create-database prerequisites (rc=${postgres_control_rc})"
  printf '%s\n' "${postgres_control_err}" >&2
fi

vault_adoption_rc=0
vault_adoption_err="$(
  s2_lib_src
  candidate="${tmpdir}/legacy-adoption-source"
  staged="${tmpdir}/legacy-adoption-staged"
  env_file="${tmpdir}/legacy-adoption.env"
  canonical_dir="${tmpdir}/legacy-adoption-canonical"
  adoption_manifest="${tmpdir}/legacy-vault-secret.adopt"
  printf '%s\n' 'stable-existing-vault-source' >"${candidate}"
  cp "${candidate}" "${staged}"
  printf 'REDGRES_LEGACY_VAULT_SECRET_FILE=%s\n' "${candidate}" >"${env_file}"
  if redgres_adopt_legacy_vault_secret "${env_file}" "${canonical_dir}" "${adoption_manifest}"; then exit 1; fi
  printf '%s\n' "${staged}" >"${adoption_manifest}"
  redgres_trusted_legacy_vault_source() {
    exec {adopt_fd}<"$1" || return 1
    REDGRES_TRUSTED_VAULT_FD="${adopt_fd}"
    REDGRES_TRUSTED_VAULT_PID="${BASHPID}"
  }
  redgres_open_legacy_vault_source_for_comparison() {
    exec {compare_fd}<"$1" || return 1
    REDGRES_LEGACY_COMPARE_FD="${compare_fd}"
    REDGRES_LEGACY_COMPARE_PID="${BASHPID}"
  }
  redgres_chown_legacy_vault_secret_dir() { :; }
  redgres_chown_legacy_vault_secret() { :; }
  redgres_vault_source_metadata() { printf '%s\n' '0:0:600'; }
  printf 'REDGRES_LEGACY_VAULT_SECRET_FILE=%s\nREDGRES_LEGACY_VAULT_SECRET_FILE=%s\n' "${candidate}" "${candidate}" >"${env_file}"
  if redgres_adopt_legacy_vault_secret "${env_file}" "${canonical_dir}" "${adoption_manifest}"; then exit 1; fi
  printf 'REDGRES_LEGACY_VAULT_SECRET_FILE=%s\n' "${candidate}" >"${env_file}"
  printf '%s\n' 'different-staged-source' >"${staged}"
  if redgres_adopt_legacy_vault_secret "${env_file}" "${canonical_dir}" "${adoption_manifest}"; then exit 1; fi
  cp "${candidate}" "${staged}"
  redgres_adopt_legacy_vault_secret "${env_file}" "${canonical_dir}" "${adoption_manifest}" || exit 1
  cmp -s "${staged}" "${canonical_dir}/legacy-vault-secret" || exit 1
  [[ "$(cat "${canonical_dir}/legacy-vault-secret.managed")" == 'managed-by-redgres-installer-v1' ]] || exit 1
  [[ ! -e "${adoption_manifest}" ]] || exit 1
)" 2>&1 || vault_adoption_rc=$?
if [[ "${vault_adoption_rc}" -eq 0 ]]; then
  pass 'upgrade requires and consumes one root-authorized legacy vault adoption manifest'
else
  fail "legacy vault adoption migration (rc=${vault_adoption_rc})"
  printf '%s\n' "${vault_adoption_err}" >&2
fi

vault_preflight_rc=0
vault_preflight_err="$(
  s2_lib_src
  mode='fresh-postgres'
  redis_mode='fresh'
  pgbouncer_mode='disabled'
  marker="${tmpdir}/vault-preflight-order"
  redgres_require_root() { printf '%s\n' root >>"${marker}"; }
  redgres_assert_fresh_vault_env_path() { printf '%s\n' gate >>"${marker}"; return 1; }
  redgres_read_os() { printf '%s\n' mutation >>"${marker}"; }
  if redgres_live_install; then exit 1; fi
)" 2>&1 || vault_preflight_rc=$?
if [[ "${vault_preflight_rc}" -ne 0 ]] && grep -qx root "${tmpdir}/vault-preflight-order" && grep -qx gate "${tmpdir}/vault-preflight-order" && ! grep -q mutation "${tmpdir}/vault-preflight-order"; then
  pass 'fresh vault env compatibility gate runs before live-install mutation'
else
  fail "fresh vault env preflight ordering (rc=${vault_preflight_rc})"
  printf '%s\n' "${vault_preflight_err}" >&2
fi

coreutils_pick_rc=0
coreutils_pick_err="$(
  s2_lib_src
  pick_root="${tmpdir}/coreutils-pick"
  mkdir -p "${pick_root}"
  ln -s "${pick_root}/missing" "${pick_root}/stat"
  printf '%s\n' '#!/bin/sh' 'exit 0' >"${pick_root}/gnustat"
  chmod +x "${pick_root}/gnustat"
  redgres_pick_coreutils_applet stat "${pick_root}/stat" "${pick_root}/gnustat"
  [[ "${REDGRES_PICK_BIN}" == "${pick_root}/gnustat" ]]
  [[ "${#REDGRES_PICK_PREFIX[@]}" -eq 0 ]]
  printf '%s\n' '#!/bin/sh' 'exit 0' >"${pick_root}/coreutils"
  chmod +x "${pick_root}/coreutils"
  redgres_pick_coreutils_applet stat "${pick_root}/stat" "${pick_root}/coreutils"
  [[ "${REDGRES_PICK_BIN}" == "${pick_root}/coreutils" ]]
  [[ "${REDGRES_PICK_PREFIX[0]}" == 'stat' ]]
  ! redgres_pick_coreutils_applet stat "${pick_root}/stat"
) 2>&1" || coreutils_pick_rc=$?
if [[ "${coreutils_pick_rc}" -eq 0 ]]; then
  pass 'coreutils picker skips rust-coreutils symlinks'
else
  fail "coreutils picker (rc=${coreutils_pick_rc})"
  printf '%s\n' "${coreutils_pick_err}" >&2
fi

os_release_rc=0
os_release_err="$(
  s2_lib_src
  osr="${tmpdir}/os-release-layout"
  mkdir -p "${osr}/usr" "${osr}/etc"
  printf '%s\n' 'ID=ubuntu' >"${osr}/usr/os-release"
  printf '%s\n' 'ID=ubuntu' >"${osr}/etc/plain"
  got="$(redgres_resolve_os_release "${osr}/etc/plain" "${osr}/usr/os-release")"
  [[ "${got}" == "${osr}/etc/plain" ]]
  if ln -s "${osr}/usr/os-release" "${osr}/etc/os-release" 2>/dev/null && [[ -L "${osr}/etc/os-release" ]]; then
    got="$(redgres_resolve_os_release "${osr}/etc/os-release" "${osr}/usr/os-release")"
    [[ "${got}" == "${osr}/usr/os-release" ]]
    ! redgres_resolve_os_release "${osr}/etc/os-release" "${osr}/usr/missing"
  fi
) 2>&1" || os_release_rc=$?
if [[ "${os_release_rc}" -eq 0 ]]; then
  pass 'os-release resolver accepts Debian usr/lib file via /etc symlink'
else
  fail "os-release resolver (rc=${os_release_rc})"
  printf '%s\n' "${os_release_err}" >&2
fi

uninstall_checkout_rc=0
uninstall_checkout_err="$(
  # shellcheck disable=SC1091
  REDGRES_UNINSTALL_FUNCTIONS_ONLY=1 source "${deploy_dir%/*}/uninstall.sh"
  fake="${tmpdir}/not-a-checkout"
  mkdir -p "${fake}"
  ! redgres_uninstall_is_git_checkout "${fake}"
  ! redgres_uninstall_is_git_checkout /
  ! redgres_uninstall_is_git_checkout /root
  clone="${tmpdir}/redgres-src"
  mkdir -p "${clone}/deploy" "${clone}/.git"
  printf '%s\n' '#!/bin/sh' >"${clone}/deploy/install.sh"
  printf '%s\n' '#!/bin/sh' >"${clone}/uninstall.sh"
  redgres_uninstall_is_git_checkout "${clone}"
  redgres_uninstall_cloudflare_status_confirmed api_ok
  redgres_uninstall_cloudflare_status_confirmed no_domain
  redgres_uninstall_cloudflare_status_confirmed manual_dns
  ! redgres_uninstall_cloudflare_status_confirmed no_state
  ! redgres_uninstall_cloudflare_status_confirmed no_token
  ! redgres_uninstall_cloudflare_status_confirmed api_partial
  restore_log="${tmpdir}/uninstall-restore.log"
  systemctl() { printf '%s\n' "$*" >>"${restore_log}"; }
  REDGRES_UNINSTALL_QUIESCE_GUARD=1
  REDGRES_UNINSTALL_APP_WAS_ACTIVE=1
  REDGRES_UNINSTALL_TLS_PATH_WAS_ACTIVE=1
  redgres_uninstall_restore_quiesced
  grep -Fqx -- '--no-block start redgres.service' "${restore_log}" || exit 1
  grep -Fqx -- '--no-block start redgres-tls-issue.path' "${restore_log}" || exit 1
  [[ "${REDGRES_UNINSTALL_QUIESCE_GUARD}" == "0" ]] || exit 1
  lineage_fixture="${tmpdir}/tls-lineage-fixture"
  printf '%s\n' '/etc/letsencrypt/live/db.example.com' >"${lineage_fixture}"
  chmod 0600 "${lineage_fixture}"
  export REDGRES_UNINSTALL_LINEAGE_EXPECTED_METADATA="$(/usr/bin/stat -c '%U:%G:%a' "${lineage_fixture}")"
  ! redgres_uninstall_delete_trusted_lineage "${lineage_fixture}" "${tmpdir}/missing-certbot"
  [[ -f "${lineage_fixture}" ]]
  fake_certbot="${tmpdir}/fake-uninstall-certbot"
  printf '%s\n' '#!/bin/sh' 'exit 0' >"${fake_certbot}"
  chmod 0700 "${fake_certbot}"
  redgres_uninstall_delete_trusted_lineage "${lineage_fixture}" "${fake_certbot}"
  unset REDGRES_UNINSTALL_LINEAGE_EXPECTED_METADATA
) 2>&1" || uninstall_checkout_rc=$?
if [[ "${uninstall_checkout_rc}" -eq 0 ]]; then
  pass 'uninstall git-checkout detector matches clone layout only'
else
  fail "uninstall git-checkout detector (rc=${uninstall_checkout_rc})"
  printf '%s\n' "${uninstall_checkout_err}" >&2
fi

uninstall_cloudflare_output=""
if uninstall_cloudflare_output="$(bash "${tests_dir}/uninstall_cloudflare_test.sh" 2>&1)" && \
  [[ "${uninstall_cloudflare_output}" == *'uninstall_cloudflare_cleanup=pass'* ]]; then
  pass 'uninstall quiesces and restores cloudflared around connector cleanup'
else
  fail 'uninstall cloudflared quiesce and connector cleanup ordering'
  printf '%s\n' "${uninstall_cloudflare_output}" >&2
fi

uninstall_purge_rc=0
uninstall_purge_err="$(
  # shellcheck disable=SC1091
  REDGRES_UNINSTALL_FUNCTIONS_ONLY=1 source "${deploy_dir%/*}/uninstall.sh"
  leftover_root="${tmpdir}/pg-leftovers"
  mkdir -p "${leftover_root}/etc/postgresql/18/main" "${leftover_root}/var/log/postgresql"
  printf 'log\n' >"${leftover_root}/var/log/postgresql/postgresql-18-main.log"
  mapfile -t leftover_dirs < <(redgres_uninstall_postgres_leftover_dirs "${leftover_root}")
  [[ "${leftover_dirs[0]}" == "${leftover_root}/etc/postgresql" ]]
  [[ "${leftover_dirs[1]}" == "${leftover_root}/etc/postgresql-common" ]]
  [[ "${leftover_dirs[2]}" == "${leftover_root}/var/log/postgresql" ]]
  [[ "${leftover_dirs[3]}" == "${leftover_root}/var/lib/postgresql" ]]
  redgres_uninstall_remove_postgres_leftovers "${leftover_root}"
  [[ ! -e "${leftover_root}/etc/postgresql" ]]
  [[ ! -e "${leftover_root}/var/log/postgresql" ]]
  redgres_uninstall_export_apt_env
  [[ "${DEBIAN_FRONTEND}" == noninteractive ]]
  [[ "${NEEDRESTART_MODE}" == l ]]
  [[ "${NEEDRESTART_SUSPEND}" == 1 ]]
  [[ "${APT_LISTCHANGES_FRONTEND}" == none ]]
  cwd_jail="${tmpdir}/deleted-cwd"
  mkdir -p "${cwd_jail}"
  (
    cd "${cwd_jail}"
    redgres_uninstall_enter_safe_cwd
    [[ "$(pwd)" != "${cwd_jail}" ]]
  )
  grep -q '\[8/8\]' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'redgres_uninstall_enter_safe_cwd' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'redgres_uninstall_remove_postgres_leftovers' "${deploy_dir%/*}/uninstall.sh"
  grep -q '</dev/null' "${deploy_dir%/*}/uninstall.sh"
  grep -q '/etc/ssl/redgres' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'redgres-copy-certs.sh' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'redgres-tls-issue.service' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'redgres_uninstall_purge_postgresql_packages' "${deploy_dir%/*}/uninstall.sh"
  ! grep -F "purge -y postgresql" "${deploy_dir%/*}/uninstall.sh"
  grep -q 'APT::Get::Assume-Yes=true' "${deploy_dir%/*}/uninstall.sh"
  grep -F -q 'exec bash "${_uninstall_tmp}" "$@" </dev/null' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'redgres_uninstall_apt_handle_log' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'redgres_uninstall_purge_installed' "${deploy_dir%/*}/uninstall.sh"
  grep -A1 'Removing PostgreSQL clusters' "${deploy_dir%/*}/uninstall.sh" | grep -q purge_postgresql
  grep -A1 'Removing leftover packages' "${deploy_dir%/*}/uninstall.sh" | grep -q redgres_uninstall_enter_safe_cwd
  apt_ok="$(mktemp)"
  printf '%s\n' 'Reading package lists... Done' 'Scanning processes...' >"${apt_ok}"
  [[ -z "$(redgres_uninstall_apt_handle_log "${apt_ok}" 0)" ]]
  apt_bad="$(mktemp)"
  printf '%s\n' 'E: Unable to fetch' 'requirepass secret' 'password leaked' >"${apt_bad}"
  dump="$(redgres_uninstall_apt_handle_log "${apt_bad}" 1 2>&1)" || true
  [[ "${dump}" == *'apt-get failed'* ]]
  [[ "${dump}" == *'E: Unable to fetch'* ]]
  [[ "${dump}" != *'requirepass'* ]]
  [[ "${dump}" != *'password leaked'* ]]
  stub_bin="${tmpdir}/uninstall-stub-bin"
  mkdir -p "${stub_bin}"
  printf '%s\n' '#!/bin/sh' 'exit 1' >"${stub_bin}/dpkg-query"
  printf '%s\n' '#!/bin/sh' 'echo RAN_APT; exit 1' >"${stub_bin}/apt-get"
  chmod +x "${stub_bin}/dpkg-query" "${stub_bin}/apt-get"
  miss="$(PATH="${stub_bin}:${PATH}" REDGRES_UNINSTALL_APT_GET="${stub_bin}/apt-get" redgres_uninstall_purge_installed redis-server pgbouncer cloudflared)"
  [[ -z "${miss}" ]]
  [[ "${miss}" != *RAN_APT* ]]
  redis_purge_log="${tmpdir}/uninstall-redis-purge.log"
  redgres_uninstall_purge_installed() { printf '%s\n' "$*" >"${redis_purge_log}"; }
  redgres_uninstall_purge_native_redis_packages
  grep -Fqx 'redis-server redis redis-tools' "${redis_purge_log}" || exit 1
) 2>&1" || uninstall_purge_rc=$?
if [[ "${uninstall_purge_rc}" -eq 0 ]]; then
  pass 'uninstall purge leaves leftover Postgres dirs, noninteractive apt, and a safe cwd'
else
  fail "uninstall leftover-dir / apt / cwd helpers (rc=${uninstall_purge_rc})"
  printf '%s\n' "${uninstall_purge_err}" >&2
fi

os_gate_rc=0
os_gate_err="$(
  s2_lib_src
  redgres_assert_pgdg_ubuntu ubuntu 24.04 noble
  redgres_assert_pgdg_ubuntu ubuntu 26.04 resolute
  pin82="$(redgres_redis_image_pin 8.2)"
  pin88="$(redgres_redis_image_pin 8.8)"
  [[ "${pin82}" == 'redis:8.2.9@sha256:7d1e4ce8b9395088377ab382d1f6cfdbd13b3690795198a0399ab8d683064d6d' ]]
  [[ "${pin88}" == 'redis:8.8.2@sha256:c514823c0ec1a40764df434efc2dc4ab5ec669c71c1cb00e4f7b1a694cee9fc3' ]]
) 2>&1" || os_gate_rc=$?
if [[ "${os_gate_rc}" -eq 0 ]]; then
  pass 'PGDG OS gate accepts noble/resolute and Redis digest pins'
else
  fail "PGDG OS gate / Redis pins (rc=${os_gate_rc})"
  printf '%s\n' "${os_gate_err}" >&2
fi

expect_os_rejected() {
  local desc="$1" os_id="$2" os_version_id="$3" os_codename="$4"
  local rc=0
  ( s2_lib_src; redgres_assert_pgdg_ubuntu "${os_id}" "${os_version_id}" "${os_codename}" ) >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 1 ]]; then
    pass "${desc}"
  else
    fail "${desc} (rc=${rc})"
  fi
}
expect_os_rejected 'PGDG OS gate rejects oracular (24.10)' ubuntu 24.10 oracular
expect_os_rejected 'PGDG OS gate rejects jammy (22.04)' ubuntu 22.04 jammy
expect_os_rejected 'PGDG OS gate rejects debian' debian 13 trixie

apt_parse_rc=0
apt_parse_err="$(
  s2_lib_src
  got="$(redgres_apt_candidate_from_policy $'postgresql-18:\n  Installed: (none)\n  Candidate: 18.6-1.pgdg24.04+1\n')"
  [[ "${got}" == '18.6-1.pgdg24.04+1' ]]
  got="$(redgres_apt_candidate_from_policy $'docker.io:\n  Installed: (none)\n  Candidate: 1:29.1.3-0ubuntu4.1\n')"
  [[ "${got}" == '1:29.1.3-0ubuntu4.1' ]]
  redgres_assert_redis_pong $'OK\nPONG\n'
)" 2>&1 || apt_parse_rc=$?
if [[ "${apt_parse_rc}" -eq 0 ]]; then
  pass 'apt Candidate pin parser and redis PONG assertion'
else
  fail "apt Candidate pin parser (rc=${apt_parse_rc})"
  printf '%s\n' "${apt_parse_err}" >&2
fi

expect_apt_candidate_rejected() {
  local desc="$1" policy="$2"
  local rc=0
  ( s2_lib_src; redgres_apt_candidate_from_policy "${policy}" ) >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 1 ]]; then
    pass "${desc}"
  else
    fail "${desc} (rc=${rc})"
  fi
}
expect_apt_candidate_rejected 'apt Candidate (none) is rejected' $'pkg:\n  Candidate: (none)\n'
expect_apt_candidate_rejected 'apt Candidate injection is rejected' $'pkg:\n  Candidate: 1.0;id\n'
expect_apt_candidate_rejected 'apt Candidate missing is rejected' $'pkg:\n  Installed: (none)\n'

progress_rc=0
progress_err="$(
  s2_lib_src
  sec="$(redgres_section 2 8 Packages)"
  [[ "${sec}" == *'[2/8] Packages'* ]]
  [[ "${sec}" != *$'\033['* ]]
  quiet="$(redgres_run_quiet demo true)"
  [[ -z "${quiet}" ]]
  fail_out="$(redgres_run_quiet demo bash -c 'printf "%s\n" "requirepass secret"; printf "%s\n" "boom"; exit 1' 2>&1)" || true
  [[ "${fail_out}" == *'demo failed'* ]]
  [[ "${fail_out}" == *'boom'* ]]
  [[ "${fail_out}" != *'requirepass'* ]]
  apt_ok="$(/usr/bin/mktemp)"
  printf '%s\n' 'Hit:1 http://example' 'Reading package lists... Done' 'Scanning processes...' 'NO_PUBKEY 7FCC7D46ACCC4CF8' >"${apt_ok}"
  note="$(redgres_apt_handle_log "${apt_ok}" 0)"
  [[ "${note}" == *'using cached PGDG index'* ]]
  [[ "${note}" != *'Hit:1'* ]]
  [[ "${note}" != *'Reading package lists'* ]]
  [[ "${note}" != *'Scanning processes'* ]]
  apt_bad="$(/usr/bin/mktemp)"
  printf '%s\n' 'E: Unable to fetch' 'Failed to fetch https://apt.example/dists' 'requirepass secret' 'password leaked' 'postgresql://u:p@h/db' >"${apt_bad}"
  dump="$(redgres_apt_handle_log "${apt_bad}" 1 2>&1)" || true
  [[ "${dump}" == *'apt-get failed'* ]]
  [[ "${dump}" == *'E: Unable to fetch'* ]]
  [[ "${dump}" == *'Failed to fetch https://apt.example/dists'* ]]
  [[ "${dump}" != *'requirepass'* ]]
  [[ "${dump}" != *'password leaked'* ]]
  [[ "${dump}" != *':p@h/db'* ]]
  [[ "${dump}" == *'[redacted]@'* ]]
  fake_ok="${tmpdir}/fake-apt-ok"
  printf '%s\n' '#!/bin/sh' 'printf "%s\n" "Reading package lists... Done"' 'exit 0' >"${fake_ok}"
  chmod +x "${fake_ok}"
  wrap_ok="$(REDGRES_APT_GET="${fake_ok}" redgres_apt_get update)"
  [[ -z "${wrap_ok}" ]]
  ! ls /tmp/redgres-cmd.* >/dev/null 2>&1
  fake_bad="${tmpdir}/fake-apt-bad"
  printf '%s\n' '#!/bin/sh' 'printf "%s\n" "E: Unable" "Failed to fetch https://apt.example/dists" "requirepass secret" "postgresql://u:p@h/db"' 'exit 1' >"${fake_bad}"
  chmod +x "${fake_bad}"
  wrap_rc=0
  wrap_fail="$(REDGRES_APT_GET="${fake_bad}" redgres_apt_get update 2>&1)" || wrap_rc=$?
  [[ "${wrap_rc}" -ne 0 ]]
  [[ "${wrap_fail}" == *'apt-get failed'* ]]
  [[ "${wrap_fail}" == *'Failed to fetch https://apt.example/dists'* ]]
  [[ "${wrap_fail}" != *'requirepass'* ]]
  [[ "${wrap_fail}" != *':p@h/db'* ]]
  ! ls /tmp/redgres-cmd.* >/dev/null 2>&1
  grep -q "redgres_section 1 8 'Preflight'" "${deploy_dir}/lib/mutate.sh"
  grep -q "redgres_section 8 8 'Application'" "${deploy_dir}/lib/mutate.sh"
  package_sec="$(/usr/bin/awk '/redgres_section 2 8 .Packages./,/redgres_section 3 8 .Redis./' "${deploy_dir}/lib/mutate.sh")"
  redis_sec="$(/usr/bin/awk '/redgres_section 3 8 .Redis./,/redgres_section 4 8 .PostgreSQL./' "${deploy_dir}/lib/mutate.sh")"
  pg_sec="$(/usr/bin/awk '/redgres_section 4 8 .PostgreSQL./,/redgres_section 5 8 .PgBouncer./' "${deploy_dir}/lib/mutate.sh")"
  printf '%s\n' "${package_sec}" | /usr/bin/grep -Fq 'redgres_apt_candidate python3' || exit 1
  printf '%s\n' "${package_sec}" | /usr/bin/grep -Fq 'redgres_apt_install python3' || exit 1
  printf '%s\n' "${package_sec}" | /usr/bin/grep -Fq '[[ -x /usr/bin/python3 ]]' || exit 1
  python_candidate_line="$(printf '%s\n' "${package_sec}" | /usr/bin/awk '/redgres_apt_candidate python3/ { print NR; exit }')" || exit 1
  python_install_line="$(printf '%s\n' "${package_sec}" | /usr/bin/awk '/redgres_apt_install python3/ { print NR; exit }')" || exit 1
  ca_install_line="$(printf '%s\n' "${package_sec}" | /usr/bin/awk '/redgres_apt_install ca-certificates/ { print NR; exit }')" || exit 1
  [[ -n "${python_candidate_line}" && -n "${python_install_line}" && -n "${ca_install_line}" ]] || exit 1
  (( python_candidate_line < python_install_line && python_install_line < ca_install_line )) || exit 1
  printf '%s\n' "${redis_sec}" | /usr/bin/grep -q redgres_ensure_redisinsight
  ! printf '%s\n' "${redis_sec}" | /usr/bin/grep -q redgres_ensure_pgadmin
  ! printf '%s\n' "${redis_sec}" | /usr/bin/grep -q redgres_ensure_expert_tools
  printf '%s\n' "${pg_sec}" | /usr/bin/grep -q redgres_ensure_pgadmin
  ! printf '%s\n' "${pg_sec}" | /usr/bin/grep -q redgres_ensure_redisinsight
  grep -q 'REDGRES_DOMAIN_PACKAGES_OPTIONAL=1' "${deploy_dir}/lib/mutate.sh"
  grep -q "redgres_install_domain_runtime || redgres_die 'domain runtime failed" "${deploy_dir}/lib/release.sh"
  grep -q 'NEEDRESTART_SUSPEND=1' "${deploy_dir}/lib/mutate.sh"
  ! grep -E 'DEBIAN_FRONTEND=noninteractive /usr/bin/apt-get update' "${deploy_dir}/lib/mutate.sh"
  grep -q 'psql -q -d postgres' "${deploy_dir}/lib/mutate.sh"
)" 2>&1 || progress_rc=$?
if [[ "${progress_rc}" -eq 0 ]]; then
  pass 'live install progress is compact and secret-safe'
else
  fail "live install progress helpers (rc=${progress_rc})"
  printf '%s\n' "${progress_err}" >&2
fi

pong_rc=0
( s2_lib_src; redgres_assert_redis_pong $'NOAUTH Authentication required.\n' ) >/dev/null 2>&1 || pong_rc=$?
if [[ "${pong_rc}" -eq 1 ]]; then
  pass 'redis PONG assertion rejects NOAUTH'
else
  fail "redis PONG assertion should reject NOAUTH (rc=${pong_rc})"
fi

compose_yaml_rc=0
compose_yaml_err="$(
  s2_lib_src
  yaml="$(redgres_redis_compose_yaml "$(redgres_redis_image_pin 8.8)")"
  printf '%s\n' "${yaml}" | /usr/bin/grep -q 'SKIP_FIX_PERMS: "1"'
  printf '%s\n' "${yaml}" | /usr/bin/grep -q 'redis.conf:/usr/local/etc/redis/redis.conf:ro'
  printf '%s\n' "${yaml}" | /usr/bin/grep -q 'redis:8.8.2@sha256:c514823c0ec1a40764df434efc2dc4ab5ec669c71c1cb00e4f7b1a694cee9fc3'
)" 2>&1 || compose_yaml_rc=$?

expert_pin_rc=0
expert_pin_err="$(
  s2_lib_src || exit 1
  printf '%s\n' "$(redgres_pgadmin_image_pin)" | /usr/bin/grep -q 'dpage/pgadmin4:9.17@sha256:2f4ce946ddf8360680d7eff4eaba1d91859eb6b4003e6623bad5c63a322c2f4d' || exit 1
  printf '%s\n' "$(redgres_redisinsight_image_pin)" | /usr/bin/grep -q 'redis/redisinsight:3.8.0@sha256:b5e19ee240abef6edb435871b90ff8a210995422e8e018ab61c0339d318a1f84' || exit 1
  yaml="$(redgres_expert_tools_compose_yaml)" || exit 1
  printf '%s\n' "${yaml}" | /usr/bin/grep -q '127.0.0.1:5052:80' || exit 1
  printf '%s\n' "${yaml}" | /usr/bin/grep -q '127.0.0.1:5542:5540' || exit 1
  printf '%s\n' "${yaml}" | /usr/bin/grep -q "PGADMIN_CONFIG_AUTHENTICATION_SOURCES" || exit 1
  printf '%s\n' "${yaml}" | /usr/bin/grep -q "PGADMIN_CONFIG_MASTER_PASSWORD_HOOK" || exit 1
  printf '%s\n' "${yaml}" | /usr/bin/grep -q 'pgadmin.master:/run/redgres/pgadmin.master:ro' || exit 1
  printf '%s\n' "${yaml}" | /usr/bin/grep -q 'pgadmin-master-hook:/pgadmin4/redgres-master-hook:ro' || exit 1
  ! printf '%s\n' "${yaml}" | /usr/bin/grep -q 'MASTER_PASSWORD_REQUIRED' || exit 1
  ! printf '%s\n' "${yaml}" | /usr/bin/grep -qi 'DEFAULT_PASSWORD' || exit 1
  expert_write="$(/usr/bin/awk '/^redgres_write_expert_tools_compose\(\)/,/^}/' "${deploy_dir}/lib/mutate.sh")" || exit 1
  printf '%s\n' "${expert_write}" | /usr/bin/grep -Fq 'redgres_prepare_owned_dir_no_follow /var/lib/redgres pgadmin 5050 5050 700' || exit 1
  printf '%s\n' "${expert_write}" | /usr/bin/grep -Fq 'redgres_prepare_owned_dir_no_follow /var/lib/redgres redisinsight 1000 1000 700' || exit 1
  ! printf '%s\n' "${expert_write}" | /usr/bin/grep -Eq '/usr/bin/(mkdir|chown|chmod) .*\b(pgadmin|redisinsight)\b' || exit 1
  owned_helper="$(/usr/bin/awk '/^redgres_prepare_owned_dir_no_follow\(\)/,/^}/' "${deploy_dir}/lib/mutate.sh")" || exit 1
  printf '%s\n' "${owned_helper}" | /usr/bin/grep -Fq 'os.O_NOFOLLOW' || exit 1
  printf '%s\n' "${owned_helper}" | /usr/bin/grep -Fq 'dir_fd=parent_fd' || exit 1
  printf '%s\n' "${owned_helper}" | /usr/bin/grep -Fq 'os.fchown(child_fd' || exit 1
  printf '%s\n' "${owned_helper}" | /usr/bin/grep -Fq 'os.fchmod(child_fd' || exit 1

  if [[ "$(/usr/bin/uname -s)" == Linux* ]]; then
    [[ -x /usr/bin/python3 ]] || exit 1
    owned_parent="${tmpdir}/redisinsight-owned-parent"
    outside="${tmpdir}/redisinsight-outside"
    /usr/bin/mkdir -p "${owned_parent}" "${outside}" || exit 1
    /usr/bin/chmod 755 "${outside}" || exit 1
    test_uid="$(/usr/bin/id -u)" || exit 1
    test_gid="$(/usr/bin/id -g)" || exit 1
    redgres_prepare_owned_dir_no_follow "${owned_parent}" redisinsight "${test_uid}" "${test_gid}" 700 || exit 1
    [[ "$(/usr/bin/stat -c '%u:%g %a' "${owned_parent}/redisinsight")" == "${test_uid}:${test_gid} 700" ]] || exit 1
    redgres_prepare_owned_dir_no_follow "${owned_parent}" redisinsight "${test_uid}" "${test_gid}" 700 || exit 1
    [[ "$(/usr/bin/stat -c '%u:%g %a' "${owned_parent}/redisinsight")" == "${test_uid}:${test_gid} 700" ]] || exit 1
    /usr/bin/rm -rf -- "${owned_parent}/redisinsight" || exit 1
    /usr/bin/ln -s -- "${outside}" "${owned_parent}/redisinsight" || exit 1
    if redgres_prepare_owned_dir_no_follow "${owned_parent}" redisinsight "${test_uid}" "${test_gid}" 700 >/dev/null 2>&1; then
      exit 1
    fi
    [[ "$(/usr/bin/stat -c '%a' "${outside}")" == 755 ]] || exit 1
    parent_link="${tmpdir}/redisinsight-parent-link"
    /usr/bin/ln -s -- "${owned_parent}" "${parent_link}" || exit 1
    if redgres_prepare_owned_dir_no_follow "${parent_link}" other "${test_uid}" "${test_gid}" 700 >/dev/null 2>&1; then
      exit 1
    fi
    [[ ! -e "${owned_parent}/other" ]] || exit 1
  fi
)" 2>&1 || expert_pin_rc=$?
if [[ "${expert_pin_rc}" -eq 0 ]]; then
  pass 'expert-tool pins, secret-free compose, and Redis Insight data ownership'
else
  fail "expert-tool pins/compose (rc=${expert_pin_rc})"
  printf '%s\n' "${expert_pin_err}" >&2
fi

upgrade_master_rc=0
upgrade_master_err="$(
  ! /usr/bin/grep -E 'sed[[:space:]]+-i.*MASTER_PASSWORD' "${deploy_dir%/*}/upgrade.sh"
  ! /usr/bin/grep -E "sed[[:space:]]+-i.*/var/lib/redgres/pgadmin" "${deploy_dir%/*}/upgrade.sh"
  /usr/bin/grep -q '/bin/bash -p "${INSTALLER}" update' "${deploy_dir%/*}/upgrade.sh"
  /usr/bin/grep -q 'release lacks its transactional installer' "${deploy_dir%/*}/upgrade.sh"
  /usr/bin/grep -q "RELEASE_JAIL='/var/lib/redgres-release'" "${deploy_dir%/*}/upgrade.sh"
  /usr/bin/grep -q 'REDGRES_DOMAIN_RUNTIME_IF_MANAGED=1 REDGRES_DOMAIN_PACKAGES_OPTIONAL=1' "${deploy_dir%/*}/upgrade.sh"
  /usr/bin/grep -q 'REDGRES_EXPERT_TOOLS_IF_MANAGED=1 REDGRES_EXPERT_TOOLS_OPTIONAL=1' "${deploy_dir%/*}/upgrade.sh"
  ! /usr/bin/grep -q 'REDGRES_DOMAIN_RUNTIME_OPTIONAL' "${deploy_dir%/*}/upgrade.sh"
  ! /usr/bin/grep -q '^ln -sfn "${DEST}" "${OPT_ROOT}/current"' "${deploy_dir%/*}/upgrade.sh"
  /usr/bin/grep -q 'redgres_restore_pgadmin_master_ownership' "${deploy_dir%/*}/install.sh"
  /usr/bin/grep -q 'redgres_restore_pgadmin_master_ownership' "${deploy_dir%/*}/install-dev.sh"
  /usr/bin/grep -q 'legacy-vault-secret.adopt' "${deploy_dir%/*}/upgrade.sh"
  /usr/bin/grep -q 'legacy vault migration requires root authorization' "${deploy_dir%/*}/upgrade.sh"
  stable_adopt_line="$(/usr/bin/grep -n 'legacy vault migration requires root authorization before changing the current release' "${deploy_dir%/*}/install.sh" | head -n1 | cut -d: -f1)"
  stable_switch_line="$(/usr/bin/grep -n 'ln -sfn "${DEST}" "${OPT_ROOT}/current"' "${deploy_dir%/*}/install.sh" | head -n1 | cut -d: -f1)"
  dev_adopt_line="$(/usr/bin/grep -n 'legacy vault migration requires root authorization before changing the current release' "${deploy_dir%/*}/install-dev.sh" | head -n1 | cut -d: -f1)"
  dev_switch_line="$(/usr/bin/grep -n 'ln -sfn "${DEST}" "${OPT_ROOT}/current"' "${deploy_dir%/*}/install-dev.sh" | head -n1 | cut -d: -f1)"
  [[ -n "${stable_adopt_line}" && -n "${stable_switch_line}" && "${stable_adopt_line}" -lt "${stable_switch_line}" ]]
  [[ -n "${dev_adopt_line}" && -n "${dev_switch_line}" && "${dev_adopt_line}" -lt "${dev_switch_line}" ]]
)" 2>&1 || upgrade_master_rc=$?
if [[ "${upgrade_master_rc}" -eq 0 ]]; then
  pass 'public upgrade delegates to checksummed transactional installer'
else
  fail "upgrade pgadmin master rewrite (rc=${upgrade_master_rc})"
  printf '%s\n' "${upgrade_master_err}" >&2
fi

release_clean_rc=0
release_clean_err="$(
  release_workflow="${deploy_dir%/*}/.github/workflows/release.yml"
  set_version="${deploy_dir%/*}/scripts/set-version.sh"
  sync_version="${deploy_dir%/*}/scripts/sync-version.sh"
  ! /usr/bin/grep -q 'chmod +x' "${release_workflow}" || exit 1
  /usr/bin/grep -Fq 'bash ./scripts/set-version.sh "${{ steps.ver.outputs.version }}"' "${release_workflow}" || exit 1
  /usr/bin/grep -Fq 'bash ./scripts/generate-release-notes.sh \' "${release_workflow}" || exit 1
  /usr/bin/grep -Fq 'bash ./deploy/build-release.sh "${{ steps.ver.outputs.version }}"' "${release_workflow}" || exit 1
  [[ "$(/usr/bin/grep -Fc 'git status --porcelain --untracked-files=no' "${release_workflow}")" -eq 2 ]] || exit 1
  /usr/bin/grep -Fq 'vcs.revision=${REVISION}' "${release_workflow}" || exit 1
  /usr/bin/grep -Fq 'vcs.modified=false' "${release_workflow}" || exit 1
  /usr/bin/grep -Fq 'bash ./scripts/sync-version.sh' "${set_version}" || exit 1
  /usr/bin/grep -Fq 'bash ./scripts/sync-version.sh check' "${sync_version}" || exit 1

  mode_fixture="${tmpdir}/release-script-modes"
  /usr/bin/mkdir -p "${mode_fixture}/scripts" "${mode_fixture}/web" || exit 1
  /usr/bin/cp "${set_version}" "${sync_version}" "${mode_fixture}/scripts/" || exit 1
  /usr/bin/cp "${deploy_dir%/*}/VERSION" "${deploy_dir%/*}/web/package.json" "${deploy_dir%/*}/web/package-lock.json" "${mode_fixture}/" || exit 1
  /usr/bin/mv "${mode_fixture}/package.json" "${mode_fixture}/web/package.json" || exit 1
  /usr/bin/mv "${mode_fixture}/package-lock.json" "${mode_fixture}/web/package-lock.json" || exit 1
  /usr/bin/chmod 0644 "${mode_fixture}/scripts/set-version.sh" "${mode_fixture}/scripts/sync-version.sh" || exit 1
  (
    cd "${mode_fixture}"
    bash ./scripts/set-version.sh 9.8.7 >/dev/null || exit 1
    /usr/bin/grep -qx '9.8.7' VERSION || exit 1
    [[ "$(node -p "require('./web/package.json').version")" == '9.8.7' ]] || exit 1
    [[ "$(node -p "require('./web/package-lock.json').version")" == '9.8.7' ]] || exit 1
  ) || exit 1
)" 2>&1 || release_clean_rc=$?
if [[ "${release_clean_rc}" -eq 0 ]]; then
  pass 'release workflow builds without dirtying script modes'
else
  fail "release workflow dirty checkout guard (rc=${release_clean_rc})"
  printf '%s\n' "${release_clean_err}" >&2
fi

encode_rc=0
encode_got="$(
  s2_lib_src
  redgres_redis_url_encode 'ab+c/d='
)" || encode_rc=$?
if [[ "${encode_rc}" -eq 0 && "${encode_got}" == 'ab%2Bc%2Fd%3D' ]]; then
  pass 'redis URL encoder percent-encodes base64 alphabet'
else
  fail "redis URL encoder (rc=${encode_rc} got=${encode_got})"
fi

logs_safe_rc=0
logs_safe_err="$(
  s2_lib_src
  got="$(redgres_redis_logs_safe $'Fatal error, cannot open config file\nrequirepass hunter2\nWarning: cannot change owner\nAUTH leaked\nPermission denied')"
  printf '%s\n' "${got}" | /usr/bin/grep -q 'Permission denied'
  printf '%s\n' "${got}" | /usr/bin/grep -q 'cannot change owner'
  ! printf '%s\n' "${got}" | /usr/bin/grep -qi 'requirepass'
  ! printf '%s\n' "${got}" | /usr/bin/grep -qi 'hunter2'
  ! printf '%s\n' "${got}" | /usr/bin/grep -q 'AUTH leaked'
)" 2>&1 || logs_safe_rc=$?
if [[ "${logs_safe_rc}" -eq 0 ]]; then
  pass 'redis log filter keeps permission errors and drops secrets'
else
  fail "redis log filter (rc=${logs_safe_rc})"
  printf '%s\n' "${logs_safe_err}" >&2
fi

download_root_rc=0
download_root="$(
  s2_lib_src
  redgres_release_download_root
)" || download_root_rc=$?
if [[ "${download_root_rc}" -eq 0 && "${download_root}" == '/var/lib/redgres-release' ]]; then
  pass 'release download jail is /var/lib/redgres-release'
else
  fail "release download jail (rc=${download_root_rc} got=${download_root})"
fi

urls_rc=0
urls_out="$(
  s2_lib_src
  json='{"tag_name":"v1.0.0","assets":[{"browser_download_url":"https://github.com/SSujitX/redgres/releases/download/v1.0.0/redgres_1.0.0_linux_amd64.tar.gz"},{"browser_download_url":"https://github.com/SSujitX/redgres/releases/download/v1.0.0/SHA256SUMS"}]}'
  redgres_release_urls_from_json "${json}"
) 2>&1" || urls_rc=$?
if [[ "${urls_rc}" -eq 0 ]] && [[ "${urls_out}" == *"redgres_1.0.0_linux_amd64.tar.gz"* ]] && [[ "${urls_out}" == *"SHA256SUMS"* ]]; then
  pass 'release URL parser extracts tarball and SHA256SUMS'
else
  fail "release URL parser (rc=${urls_rc})"
fi

missing_rc=0
( s2_lib_src; redgres_release_urls_from_json '{"tag_name":"v1.0.0","assets":[]}' ) >/dev/null 2>&1 || missing_rc=$?
if [[ "${missing_rc}" -eq 1 ]]; then
  pass 'release URL parser rejects missing assets'
else
  fail "release URL parser should reject missing assets (rc=${missing_rc})"
fi

evil_rc=0
( s2_lib_src; redgres_release_urls_from_json '{"tag_name":"v1.0.0","assets":[{"browser_download_url":"https://evil.example/redgres_1.0.0_linux_amd64.tar.gz"},{"browser_download_url":"https://github.com/SSujitX/redgres/releases/download/v1.0.0/SHA256SUMS"}]}' ) >/dev/null 2>&1 || evil_rc=$?
if [[ "${evil_rc}" -eq 1 ]]; then
  pass 'release URL parser rejects non-GitHub tarball URL'
else
  fail "release URL parser should reject non-GitHub tarball (rc=${evil_rc})"
fi

nested_rc=0
( s2_lib_src; redgres_release_urls_from_json '{"tag_name":"v1.0.0","assets":[{"browser_download_url":"https://github.com/SSujitX/redgres/releases/download/v1.0.0/nested/redgres_1.0.0_linux_amd64.tar.gz"},{"browser_download_url":"https://github.com/SSujitX/redgres/releases/download/v1.0.0/SHA256SUMS"}]}' ) >/dev/null 2>&1 || nested_rc=$?
if [[ "${nested_rc}" -eq 1 ]]; then
  pass 'release URL parser rejects extra path under download/vX.Y.Z/'
else
  fail "release URL parser should reject nested download path (rc=${nested_rc})"
fi

http_rc=0
( s2_lib_src; redgres_release_urls_from_json '{"tag_name":"v1.0.0","assets":[{"browser_download_url":"http://github.com/SSujitX/redgres/releases/download/v1.0.0/redgres_1.0.0_linux_amd64.tar.gz"},{"browser_download_url":"https://github.com/SSujitX/redgres/releases/download/v1.0.0/SHA256SUMS"}]}' ) >/dev/null 2>&1 || http_rc=$?
if [[ "${http_rc}" -eq 1 ]]; then
  pass 'release URL parser rejects http:// asset URL'
else
  fail "release URL parser should reject http:// (rc=${http_rc})"
fi

mock_bin="${tmpdir}/mock-redgres"
cat >"${mock_bin}" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod 700 "${mock_bin}"
owner_out="$(
  s2_lib_src
  redgres_have_owner_tty() { return 1; }
  redgres_owner_bootstrap "${mock_bin}"
) 2>&1" || true
if [[ "${owner_out}" == *"create-owner --username admin"* ]]; then
  pass 'owner bootstrap fallback prints create-owner command'
else
  fail "owner bootstrap fallback (out=${owner_out})"
fi

skip_marker="${tmpdir}/owner-generate-called"
skip_bin="${tmpdir}/mock-redgres-skip"
cat >"${skip_bin}" <<EOF
#!/usr/bin/env bash
: >"${skip_marker}"
exit 0
EOF
chmod 700 "${skip_bin}"
rm -f "${skip_marker}"
skip_out="$(
  s2_lib_src
  redgres_have_owner_tty() { return 1; }
  redgres_owner_bootstrap "${skip_bin}"
) 2>&1" || true
if [[ -e "${skip_marker}" ]]; then
  fail 'owner bootstrap invoked create-owner without a TTY'
elif [[ "${skip_out}" == *"no controlling terminal"* ]]; then
  pass 'owner bootstrap skips generate when /dev/tty cannot be opened'
else
  fail "owner bootstrap TTY skip (out=${skip_out})"
fi

genfail_rc=0
genfail_out="$(
  s2_lib_src
  REDGRES_OWNER_PASSWORD_FIFO="${tmpdir}/genfail.fifo"
  redgres_have_owner_tty() { return 0; }
  redgres_owner_bootstrap "${mock_bin}" 2>&1
)" || genfail_rc=$?
if [[ "${genfail_rc}" -eq 1 ]] && [[ "${genfail_out}" == *"create-owner --generate failed"* ]]; then
  pass 'owner bootstrap generate failure exits 1'
else
  fail "owner bootstrap generate failure (rc=${genfail_rc} out=${genfail_out})"
fi

fifo_bin="${tmpdir}/mock-redgres-fifo"
cat >"${fifo_bin}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
for a in "$@"; do
  if [[ "${a}" == "-h" || "${a}" == "--help" ]]; then
    printf '%s\n' '  -password-fifo string'
    exit 0
  fi
done
fifo=""
next=0
for a in "$@"; do
  if [[ "${next}" -eq 1 ]]; then
    fifo="${a}"
    next=0
    continue
  fi
  if [[ "${a}" == "--password-fifo" ]]; then
    next=1
  fi
done
[[ -n "${fifo}" ]] || exit 1
printf 'generated-from-mock\n' >"${fifo}"
exit 0
EOF
chmod 700 "${fifo_bin}"
fifo_rc=0
fifo_out="$(
  s2_lib_src
  REDGRES_OWNER_PASSWORD_FIFO="${tmpdir}/owner-pass.fifo"
  redgres_have_owner_tty() { return 0; }
  redgres_owner_bootstrap "${fifo_bin}"
  printf 'captured=%s\n' "${REDGRES_FINISH_OWNER_PASSWORD}"
) 2>&1" || fifo_rc=$?
if [[ "${fifo_rc}" -eq 0 ]] && [[ "${fifo_out}" == *'captured=generated-from-mock'* ]] && [[ "${fifo_out}" != *'Generated owner password:'* ]]; then
  pass 'owner bootstrap captures generated password from fifo'
else
  fail "owner bootstrap fifo capture (rc=${fifo_rc} out=${fifo_out})"
fi

old_bin="${tmpdir}/mock-redgres-old-release"
old_marker="${tmpdir}/old-generate-called"
cat >"${old_bin}" <<EOF
#!/usr/bin/env bash
for a in "\$@"; do
  if [[ "\${a}" == "-h" || "\${a}" == "--help" ]]; then
    printf '%s\n' 'Usage of create-owner:' '  -generate' '  -replace' '  -sqlite-path string' '  -username string'
    exit 0
  fi
  if [[ "\${a}" == "--password-fifo" || "\${a}" == "-password-fifo" ]]; then
    printf '%s\n' 'flag provided but not defined: -password-fifo' >&2
    exit 1
  fi
done
: >"${old_marker}"
exit 0
EOF
chmod 700 "${old_bin}"
rm -f "${old_marker}"
old_rc=0
old_out="$(
  s2_lib_src
  REDGRES_OWNER_PASSWORD_FIFO="${tmpdir}/old-owner-pass.fifo"
  redgres_have_owner_tty() { return 0; }
  redgres_owner_bootstrap "${old_bin}"
) 2>&1" || old_rc=$?
if [[ "${old_rc}" -eq 0 && -e "${old_marker}" && "${old_out}" == *'downloaded release older than installer'* && "${old_out}" != *'flag provided but not defined'* ]]; then
  pass 'owner bootstrap uses TTY generate when create-owner has no --password-fifo'
else
  fail "owner bootstrap old-release fallback (rc=${old_rc} out=${old_out})"
fi

finish_out="$(
  s2_lib_src
  mode=fresh-postgres
  postgres_version=18
  redis_version=8.2
  pgbouncer_mode=fresh
  REDGRES_OS_ID=ubuntu
  REDGRES_OS_VERSION_ID=26.04
  REDGRES_OS_CODENAME=resolute
  redgres_bootstrap_base_url() { printf 'http://203.0.113.10:8989\n'; }
  redgres_pkg_version() { printf 'stub-pkg'; }
  redgres_installed_app_version() { printf '1.0.5'; }
  redgres_ufw_on_off() { printf 'off'; }
  REDGRES_BOOTSTRAP_ALLOW_FROM='198.51.100.20'
  redgres_have_owner_tty() { return 1; }
  REDGRES_FINISH_OWNER_PASSWORD='once-owner-secret'
  REDGRES_FINISH_SHOW_PASSWORD=1
  REDGRES_PGADMIN_PASSWORD_FILE="${tmpdir}/pgadmin.pass"
  REDGRES_PGADMIN_MASTER_PASSWORD_FILE="${tmpdir}/pgadmin.master"
  printf '%s\n' 'pgadmin-login-canary-32chars!!!!' >"${REDGRES_PGADMIN_PASSWORD_FILE}"
  printf '%s\n' 'pgadmin-master-canary-32chars!!!!' >"${REDGRES_PGADMIN_MASTER_PASSWORD_FILE}"
  redgres_finish_report
) 2>&1" || true
if [[ "${finish_out}" == *'+-'* && "${finish_out}" == *'127.0.0.1:5432'* && "${finish_out}" == *'127.0.0.1:6380'* && "${finish_out}" == *'127.0.0.1:8790'* && "${finish_out}" == *'127.0.0.1:5052'* && "${finish_out}" == *'http://203.0.113.10:8989'* && "${finish_out}" == *'admin / once-owner-secret'* && "${finish_out}" == *'pgadmin-login-canary-32chars!!!!'* && "${finish_out}" == *'pgadmin-master-canary-32chars!!!!'* && "${finish_out}" == *'1.0.5'* && "${finish_out}" == *'UFW            off'* && "${finish_out}" == *'198.51.100.20'* && "${finish_out}" == *'fresh-postgres'* && "${finish_out}" == *'resolute'* ]]; then
  pass 'finish report box includes listeners, versions, UFW, and TTY login'
else
  fail "finish report box (out=${finish_out})"
fi
if [[ "${finish_out}" == *'super-redis-secret'* || "${finish_out}" == *'requirepass'* || "${finish_out}" == *'127.0.0.1:6432'* ]]; then
  fail "finish report leaked a Redis secret or claimed PgBouncer 6432 (out=${finish_out})"
else
  pass 'finish report does not emit Redis credentials or an unconfigured 6432 listener'
fi

hidden_tty="${tmpdir}/finish-tty.txt"
: >"${hidden_tty}"
hidden_out="$(
  s2_lib_src
  mode=fresh-postgres
  postgres_version=18
  redis_version=8.2
  pgbouncer_mode=fresh
  redgres_bootstrap_base_url() { printf 'http://203.0.113.10:8989\n'; }
  redgres_pkg_version() { printf 'stub-pkg'; }
  redgres_installed_app_version() { printf '1.0.5'; }
  redgres_ufw_on_off() { printf 'off'; }
  redgres_have_owner_tty() { return 1; }
  REDGRES_FINISH_OWNER_PASSWORD='once-owner-secret'
  REDGRES_FINISH_SHOW_PASSWORD=
  REDGRES_FINISH_TTY="${hidden_tty}"
  REDGRES_PGADMIN_PASSWORD_FILE="${tmpdir}/hidden-pgadmin.pass"
  REDGRES_PGADMIN_MASTER_PASSWORD_FILE="${tmpdir}/hidden-pgadmin.master"
  printf '%s\n' 'hidden-pgadmin-login-canary!!!!' >"${REDGRES_PGADMIN_PASSWORD_FILE}"
  printf '%s\n' 'hidden-pgadmin-master-canary!!!!' >"${REDGRES_PGADMIN_MASTER_PASSWORD_FILE}"
  redgres_finish_report
) 2>&1" || true
if [[ "${hidden_out}" == *'once-owner-secret'* || "${hidden_out}" == *'hidden-pgadmin-login-canary!!!!'* || "${hidden_out}" == *'hidden-pgadmin-master-canary!!!!'* ]]; then
  fail "finish report printed owner password without a TTY (out=${hidden_out})"
elif [[ "${hidden_out}" == *'shown on this terminal only'* ]] && grep -q 'admin / once-owner-secret' "${hidden_tty}" && grep -q 'hidden-pgadmin-login-canary!!!!' "${hidden_tty}" && grep -q 'hidden-pgadmin-master-canary!!!!' "${hidden_tty}"; then
  pass 'finish report omits owner password from stdout and writes it to the TTY sink'
else
  fail "finish report TTY omission (out=${hidden_out})"
fi

no_tty_out="$(
  s2_lib_src
  mode=fresh-postgres
  postgres_version=18
  redis_version=8.2
  pgbouncer_mode=fresh
  redgres_bootstrap_base_url() { printf 'http://203.0.113.10:8989\n'; }
  redgres_pkg_version() { printf 'stub-pkg'; }
  redgres_installed_app_version() { printf '1.0.5'; }
  redgres_ufw_on_off() { printf 'off'; }
  redgres_have_owner_tty() { return 1; }
  REDGRES_FINISH_OWNER_PASSWORD='once-owner-secret'
  REDGRES_FINISH_SHOW_PASSWORD=
  REDGRES_FINISH_TTY=
  REDGRES_PGADMIN_PASSWORD_FILE="${tmpdir}/notty-pgadmin.pass"
  REDGRES_PGADMIN_MASTER_PASSWORD_FILE="${tmpdir}/notty-pgadmin.master"
  printf '%s\n' 'notty-pgadmin-login-canary!!!!' >"${REDGRES_PGADMIN_PASSWORD_FILE}"
  printf '%s\n' 'notty-pgadmin-master-canary!!!!' >"${REDGRES_PGADMIN_MASTER_PASSWORD_FILE}"
  redgres_finish_report
) 2>&1" || true
if [[ "${no_tty_out}" == *'once-owner-secret'* || "${no_tty_out}" == *'notty-pgadmin-login-canary!!!!'* || "${no_tty_out}" == *'notty-pgadmin-master-canary!!!!'* ]]; then
  fail "finish report leaked secrets with no TTY (out=${no_tty_out})"
elif [[ "${no_tty_out}" == *'not shown (no TTY)'* && "${no_tty_out}" == *'Reveal in Expert tools'* && "${no_tty_out}" != *'shown on this terminal only'* ]]; then
  pass 'finish report uses honest no-TTY copy without leaking secrets'
else
  fail "finish report no-TTY honesty (out=${no_tty_out})"
fi

listen_out="$(
  s2_lib_src
  mode=fresh-postgres
  postgres_version=18
  redis_version=8.2
  pgbouncer_mode=fresh
  redgres_pgbouncer_listen=1
  redgres_bootstrap_base_url() { printf 'http://203.0.113.10:8989\n'; }
  redgres_pkg_version() { printf 'stub-pkg'; }
  redgres_installed_app_version() { printf '1.0.5'; }
  redgres_ufw_on_off() { printf 'off'; }
  redgres_have_owner_tty() { return 1; }
  REDGRES_FINISH_OWNER_PASSWORD='once-owner-secret'
  REDGRES_FINISH_SHOW_PASSWORD=1
  redgres_finish_report
) 2>&1" || true
if [[ "${listen_out}" == *'127.0.0.1:6432'* && "${listen_out}" == *'fresh  stub-pkg'* && "${listen_out}" != *'listen not configured'* ]]; then
  pass 'finish report claims loopback 6432 only after PgBouncer listen is configured'
else
  fail "finish report configured 6432 (out=${listen_out})"
fi

pgb_ini_rc=0
pgb_ini="$(
  s2_lib_src
  redgres_pgbouncer_ini
)" || pgb_ini_rc=$?
if [[ "${pgb_ini_rc}" -eq 0 && "${pgb_ini}" == *'listen_addr = 127.0.0.1'* && "${pgb_ini}" == *'listen_port = 6432'* && "${pgb_ini}" == *'admin_users = redgres_admin'* && "${pgb_ini}" == *'auth_user = redgres_admin'* && "${pgb_ini}" == *'auth_query ='* && "${pgb_ini}" == *'server_tls_sslmode = require'* && "${pgb_ini}" == *'client_tls_sslmode = require'* && "${pgb_ini}" != *'0.0.0.0'* && "${pgb_ini}" != *'PASSWORD'* && "${pgb_ini}" != *'postgres.pass'* ]]; then
  pass 'fresh PgBouncer ini listens on loopback 6432 with TLS and no secrets'
else
  fail "fresh PgBouncer ini (rc=${pgb_ini_rc} out=${pgb_ini})"
fi

userlist_line="$(
  s2_lib_src
  redgres_pgbouncer_userlist_line redgres_admin 'p@ss"word'
)"
if [[ "${userlist_line}" == '"redgres_admin" "p@ss\"word"' ]]; then
  pass 'PgBouncer userlist quotes the admin role and escapes password quotes'
else
  fail "PgBouncer userlist line (out=${userlist_line})"
fi

auth_sql="$(
  s2_lib_src
  redgres_pgbouncer_auth_sql
)"
if [[ "${auth_sql}" == *'pgbouncer.user_search'* && "${auth_sql}" == *'SECURITY DEFINER'* && "${auth_sql}" == *'redgres_admin'* && "${auth_sql}" != *'SUPERUSER'* ]]; then
  pass 'PgBouncer auth_query SQL is SECURITY DEFINER and not SUPERUSER'
else
  fail "PgBouncer auth SQL (out=${auth_sql})"
fi

pooled_line="$(
  s2_lib_src
  redgres_pgbouncer_listen=1
  redgres_pgbouncer_env_lines
)"
empty_pooled="$(
  s2_lib_src
  redgres_pgbouncer_listen=
  pgbouncer_mode=fresh
  redgres_pgbouncer_env_lines
)"
if [[ "${pooled_line}" == 'REDGRES_POSTGRES_POOLED_PORT=6432' && -z "${empty_pooled}" ]]; then
  pass 'env writes POOLED_PORT only after PgBouncer listen is configured'
else
  fail "PgBouncer env lines (set=${pooled_line} empty=${empty_pooled})"
fi

cfg_rc=0
cfg_err="$(
  s2_lib_src
  pgbouncer_mode=fresh
  pgb_root="${tmpdir}/pgbouncer-cfg"
  mkdir -p "${pgb_root}"
  printf '%s\n' 'canary-admin-pass' >"${pgb_root}/postgres.pass"
  export REDGRES_POSTGRES_PASSFILE="${pgb_root}/postgres.pass"
  export REDGRES_PGBOUNCER_INI="${pgb_root}/pgbouncer.ini"
  export REDGRES_PGBOUNCER_USERLIST="${pgb_root}/userlist.txt"
  export REDGRES_PGBOUNCER_SKIP_HOST=1
  redgres_configure_fresh_pgbouncer
  [[ "${redgres_pgbouncer_listen}" == "1" ]]
  grep -q 'listen_addr = 127.0.0.1' "${pgb_root}/pgbouncer.ini"
  grep -q '"redgres_admin" "canary-admin-pass"' "${pgb_root}/userlist.txt"
  ! grep -q 'canary-admin-pass' "${pgb_root}/pgbouncer.ini"
) 2>&1" || cfg_rc=$?
if [[ "${cfg_rc}" -eq 0 && "${cfg_err}" != *'canary-admin-pass'* ]]; then
  pass 'fresh PgBouncer configure writes listen files and never logs the password'
else
  fail "fresh PgBouncer configure (rc=${cfg_rc} err=${cfg_err})"
fi

skip_rc=0
skip_listen="$(
  s2_lib_src
  pgbouncer_mode=disabled
  redgres_configure_fresh_pgbouncer
  printf '%s' "${redgres_pgbouncer_listen:-}"
)" || skip_rc=$?
if [[ "${skip_rc}" -eq 0 && -z "${skip_listen}" ]]; then
  pass 'disabled PgBouncer skips listen configuration'
else
  fail "disabled PgBouncer configure (rc=${skip_rc} listen=${skip_listen})"
fi

preflight_ports="$(
  s2_lib_src
  pgbouncer_mode=fresh
  recorded=
  redgres_port_free() { recorded="${recorded} $1"; }
  redgres_live_preflight_ports
  printf '%s' "${recorded}"
)"
disabled_ports="$(
  s2_lib_src
  pgbouncer_mode=disabled
  recorded=
  redgres_port_free() { recorded="${recorded} $1"; }
  redgres_live_preflight_ports
  printf '%s' "${recorded}"
)"
if [[ "${preflight_ports}" == ' 5432 6380 6432' && "${disabled_ports}" == ' 5432 6380' ]]; then
  pass 'fresh preflight requires 6432 free; disabled does not'
else
  fail "preflight ports (fresh=${preflight_ports} disabled=${disabled_ports})"
fi

ip_rc=0
( s2_lib_src
  redgres_assert_bootstrap_allow_ip '203.0.113.10'
  ! redgres_assert_bootstrap_allow_ip '0.0.0.0'
  ! redgres_assert_bootstrap_allow_ip '127.0.0.1'
  ! redgres_assert_bootstrap_allow_ip '0.0.0.0/0'
  ! redgres_assert_bootstrap_allow_ip '203.0.113.10/32'
  REDGRES_BOOTSTRAP_ALLOW_FROM=
  SSH_CONNECTION='198.51.100.20 5555 203.0.113.10 22'
  got="$(redgres_bootstrap_allow_from)"
  [[ "${got}" == '198.51.100.20' ]]
  argv="$(redgres_ufw_bootstrap_allow_argv "${got}")"
  [[ "${argv}" == 'allow from 198.51.100.20 to any port 8989 proto tcp comment redgres-bootstrap' ]]
  [[ "${argv}" != *'allow 8989/tcp'* ]]
) >/dev/null 2>&1 || ip_rc=$?
if [[ "${ip_rc}" -eq 0 ]]; then
  pass 'bootstrap UFW allow-from is a single operator IP (never 8989/tcp world-open)'
else
  fail "bootstrap allow-from / UFW argv (rc=${ip_rc})"
fi

sudo_ip_rc=0
( s2_lib_src
  REDGRES_BOOTSTRAP_ALLOW_FROM=
  unset SSH_CONNECTION SSH_CLIENT
  REDGRES_BOOTSTRAP_WHO_LINE=
  REDGRES_BOOTSTRAP_PROC_ENVIRON="${tmpdir}/sudo-ssh.environ"
  printf 'HOME=/root\0SSH_CONNECTION=198.51.100.77 60022 203.0.113.10 22\0TERM=xterm\0' >"${REDGRES_BOOTSTRAP_PROC_ENVIRON}"
  got="$(redgres_bootstrap_allow_from)"
  [[ "${got}" == '198.51.100.77' ]]
) >/dev/null 2>&1 || sudo_ip_rc=$?
if [[ "${sudo_ip_rc}" -eq 0 ]]; then
  pass 'bootstrap allow-from recovers SSH client IP when sudo strips SSH_CONNECTION'
else
  fail "bootstrap allow-from sudo-stripped SSH (rc=${sudo_ip_rc})"
fi

who_ip_rc=0
( s2_lib_src
  REDGRES_BOOTSTRAP_ALLOW_FROM=
  unset SSH_CONNECTION SSH_CLIENT
  REDGRES_BOOTSTRAP_PROC_ENVIRON=
  REDGRES_BOOTSTRAP_WHO_LINE='ubuntu   pts/0        2026-08-30 04:49 (203.0.113.99)'
  got="$(redgres_bootstrap_allow_from)"
  [[ "${got}" == '203.0.113.99' ]]
  REDGRES_BOOTSTRAP_WHO_LINE='ubuntu   pts/0        2026-08-30 04:49 (localhost)'
  ! redgres_bootstrap_allow_from
  REDGRES_BOOTSTRAP_WHO_LINE='root     tty1         2026-08-30 04:49'
  ! redgres_bootstrap_allow_from
) >/dev/null 2>&1 || who_ip_rc=$?
if [[ "${who_ip_rc}" -eq 0 ]]; then
  pass 'bootstrap allow-from uses who remote IP and rejects localhost/console'
else
  fail "bootstrap allow-from who line (rc=${who_ip_rc})"
fi

prompt_ip_rc=0
( s2_lib_src
  REDGRES_BOOTSTRAP_ALLOW_FROM=
  unset SSH_CONNECTION SSH_CLIENT
  REDGRES_BOOTSTRAP_PROC_ENVIRON=
  REDGRES_BOOTSTRAP_WHO_LINE=
  REDGRES_BOOTSTRAP_ALLOW_TTY="${tmpdir}/bootstrap-allow.tty"
  printf '%s\n' '0.0.0.0' '203.0.113.50' >"${REDGRES_BOOTSTRAP_ALLOW_TTY}"
  got="$(redgres_resolve_bootstrap_allow_from)"
  [[ "${got}" == '203.0.113.50' ]]
  [[ "${REDGRES_BOOTSTRAP_ALLOW_FROM}" == '203.0.113.50' ]]
  argv="$(redgres_ufw_bootstrap_allow_argv "${got}")"
  [[ "${argv}" == 'allow from 203.0.113.50 to any port 8989 proto tcp comment redgres-bootstrap' ]]
  [[ "${argv}" != *'allow 8989/tcp'* ]]
) >/dev/null 2>&1 || prompt_ip_rc=$?
if [[ "${prompt_ip_rc}" -eq 0 ]]; then
  pass 'bootstrap resolve prompts once on TTY and rejects world-open 0.0.0.0'
else
  fail "bootstrap resolve TTY prompt (rc=${prompt_ip_rc})"
fi

closed_ip_rc=0
( s2_lib_src
  REDGRES_BOOTSTRAP_ALLOW_FROM=
  unset SSH_CONNECTION SSH_CLIENT
  REDGRES_BOOTSTRAP_PROC_ENVIRON=
  REDGRES_BOOTSTRAP_WHO_LINE=
  REDGRES_BOOTSTRAP_ALLOW_TTY="${tmpdir}/missing-bootstrap-tty"
  ! redgres_resolve_bootstrap_allow_from
  from="$(redgres_bootstrap_allow_from 2>/dev/null || true)"
  [[ -z "${from}" ]]
) >/dev/null 2>&1 || closed_ip_rc=$?
if [[ "${closed_ip_rc}" -eq 0 ]]; then
  pass 'bootstrap resolve stays closed when IP is unknown and no TTY'
else
  fail "bootstrap resolve fail-closed (rc=${closed_ip_rc})"
fi

unit_rc=0
unit_out="$(
  s2_lib_src
  redgres_app_unit_body '/opt/redgres/current/redgres'
) 2>&1" || unit_rc=$?
if [[ "${unit_rc}" -eq 0 ]] && [[ "${unit_out}" == *"User=redgres"* ]] && [[ "${unit_out}" == *"UMask=0077"* ]] && [[ "${unit_out}" == *"Group=redgres"* ]]; then
  pass 'systemd unit body uses User=redgres and UMask=0077'
else
  fail "systemd unit body hardening (rc=${unit_rc})"
fi
if grep -q 'User=redgres' "${deploy_dir}/systemd/redgres.service" && grep -q 'UMask=0077' "${deploy_dir}/systemd/redgres.service"; then
  pass 'deploy/systemd/redgres.service is User=redgres UMask=0077'
else
  fail 'deploy/systemd/redgres.service missing User=redgres or UMask=0077'
fi

domain_env_rc=0
domain_env_err="$(
  s2_lib_src
  env_file="${tmpdir}/redgres-domain.env"
  secrets_dir="${tmpdir}/redgres-secrets"
  printf '%s\n' 'REDGRES_ENVIRONMENT=production' 'REDGRES_ADDRESS=127.0.0.1:8790' >"${env_file}"
  REDGRES_SECRETS_DIR="${secrets_dir}"
  redgres_domain_secret_env_defaults | grep -q '^REDGRES_CLOUDFLARE_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-api-token$'
  redgres_domain_secret_env_defaults | grep -q '^REDGRES_TUNNEL_TOKEN_FILE=/var/lib/redgres/secrets/cloudflared-tunnel-token$'
  redgres_domain_secret_env_defaults | grep -q '^REDGRES_TLS_ISSUE_REQUEST_FILE=/var/lib/redgres/tls-issue.request$'
  redgres_domain_secret_env_defaults | grep -q '^REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres-tls/issue.result$'
  redgres_ensure_domain_secret_env "${env_file}"
  grep -q '^REDGRES_CLOUDFLARE_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-api-token$' "${env_file}"
  grep -q '^REDGRES_TUNNEL_TOKEN_FILE=/var/lib/redgres/secrets/cloudflared-tunnel-token$' "${env_file}"
  grep -q '^REDGRES_TLS_ISSUE_REQUEST_FILE=/var/lib/redgres/tls-issue.request$' "${env_file}"
  grep -q '^REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres-tls/issue.result$' "${env_file}"
  grep -q '^REDGRES_ENVIRONMENT=production$' "${env_file}"
  printf '%s\n' 'REDGRES_CLOUDFLARE_TOKEN_FILE=/custom/token' >"${env_file}.keep"
  printf '%s\n' 'REDGRES_ENVIRONMENT=production' >>"${env_file}.keep"
  REDGRES_SECRETS_DIR="${secrets_dir}"
  redgres_ensure_domain_secret_env "${env_file}.keep"
  grep -q '^REDGRES_CLOUDFLARE_TOKEN_FILE=/custom/token$' "${env_file}.keep"
  ! grep -q '^REDGRES_CLOUDFLARE_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-api-token$' "${env_file}.keep"
  grep -q '^REDGRES_TUNNEL_TOKEN_FILE=/var/lib/redgres/secrets/cloudflared-tunnel-token$' "${env_file}.keep"
  ! redgres_ensure_domain_secret_env "${env_file}.keep"
  printf '%s\n' 'REDGRES_ENVIRONMENT=production' 'REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres/tls-issue.result' >"${env_file}.legacy"
  redgres_ensure_domain_secret_env "${env_file}.legacy"
  grep -q '^REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres-tls/issue.result$' "${env_file}.legacy"
  ! grep -q '^REDGRES_TLS_ISSUE_RESULT_FILE=/var/lib/redgres/tls-issue.result$' "${env_file}.legacy"
  [[ -d "${secrets_dir}" ]]
) 2>&1" || domain_env_rc=$?
if [[ "${domain_env_rc}" -eq 0 ]]; then
  pass 'domain secret env paths append and never overwrite an existing key'
else
  fail "domain secret env ensure (rc=${domain_env_rc})"
  printf '%s\n' "${domain_env_err}" >&2
fi

expert_env_rc=0
expert_env_err="$(
  s2_lib_src
  env_file="${tmpdir}/redgres-expert.env"
  secrets_dir="${tmpdir}/redgres-expert-secrets"
  printf '%s\n' 'REDGRES_ENVIRONMENT=production' >"${env_file}"
  REDGRES_SECRETS_DIR="${secrets_dir}"
  redgres_ensure_expert_tool_env "${env_file}"
  grep -q '^REDGRES_PGADMIN_EMAIL=admin@redgres.com$' "${env_file}"
  grep -q '^REDGRES_PGADMIN_MASTER_PASSWORD_FILE=/var/lib/redgres/secrets/pgadmin.master$' "${env_file}"
  grep -q '^REDGRES_TOOL_GATE_PGADMIN_LISTEN=127.0.0.1:5050$' "${env_file}"
  grep -q '^REDGRES_TOOL_GATE_REDISINSIGHT_UPSTREAM=http://127.0.0.1:5542$' "${env_file}"
  printf '%s\n' 'REDGRES_PGADMIN_EMAIL=custom@example.com' >"${env_file}.keep"
  REDGRES_SECRETS_DIR="${secrets_dir}"
  redgres_ensure_expert_tool_env "${env_file}.keep"
  grep -q '^REDGRES_PGADMIN_EMAIL=custom@example.com$' "${env_file}.keep"
  ! grep -q '^REDGRES_PGADMIN_EMAIL=admin@redgres.com$' "${env_file}.keep"
  grep -q '^REDGRES_TOOL_GATE_PGADMIN_LISTEN=127.0.0.1:5050$' "${env_file}.keep"
)" 2>&1 || expert_env_rc=$?
if [[ "${expert_env_rc}" -eq 0 ]]; then
  pass 'expert-tool env keys append and never overwrite an existing key'
else
  fail "expert-tool env ensure (rc=${expert_env_rc})"
  printf '%s\n' "${expert_env_err}" >&2
fi

domain_runtime_rc=0
domain_runtime_err="$(
  s2_lib_src
  redgres_cloudflared_apt_source_line | grep -q 'https://pkg.cloudflare.com/cloudflared any main'
  units_src="${deploy_dir}/systemd"
  libexec="${tmpdir}/domain-libexec"
  sysd="${tmpdir}/domain-systemd"
  hook="${tmpdir}/certbot-hooks"
  REDGRES_DOMAIN_UNIT_SRC="${units_src}"
  REDGRES_LIBEXEC_DIR="${libexec}"
  REDGRES_SYSTEMD_UNIT_DIR="${sysd}"
  REDGRES_CERTBOT_DEPLOY_HOOK_DIR="${hook}"
  REDGRES_SKIP_DOMAIN_PACKAGES=1
  ! redgres_domain_runtime_is_managed
  REDGRES_DOMAIN_RUNTIME_IF_MANAGED=1 redgres_install_domain_runtime
  [[ ! -e "${libexec}/issue-tls.sh" ]]
  unset REDGRES_DOMAIN_RUNTIME_IF_MANAGED
  redgres_install_cloudflared_units
  redgres_install_tls_issue_helper
  redgres_domain_runtime_is_managed
  if REDGRES_DOMAIN_UNIT_SRC="${tmpdir}/missing-domain-units" \
    REDGRES_DOMAIN_RUNTIME_IF_MANAGED=1 \
    redgres_install_domain_runtime; then
    exit 1
  fi
  REDGRES_DOMAIN_UNIT_SRC="${units_src}"
  [[ -x "${libexec}/cloudflared-run.sh" ]]
  [[ -f "${sysd}/cloudflared-redgres.path" ]]
  grep -q 'LoadCredential=TUNNEL_TOKEN:' "${sysd}/cloudflared-redgres.service"
  grep -q 'ConditionPathExists=/var/lib/redgres/secrets/cloudflared-tunnel-token' "${sysd}/cloudflared-redgres.service"
  grep -q 'PathChanged=/var/lib/redgres/secrets/cloudflared-tunnel-token' "${sysd}/cloudflared-redgres.path"
  ! grep -q 'PathExists=/var/lib/redgres/secrets/cloudflared-tunnel-token' "${sysd}/cloudflared-redgres.path"
  grep -q 'systemctl enable cloudflared-redgres.service' "${deploy_dir}/lib/app_install.sh"
  grep -q 'systemctl start cloudflared-redgres.service' "${deploy_dir}/lib/app_install.sh"
  grep -q -- '--protocol http2' "${libexec}/cloudflared-run.sh"
  [[ -x "${libexec}/issue-tls.sh" ]]
  grep -q -- '--dns-cloudflare-propagation-seconds 60' "${libexec}/issue-tls.sh"
  grep -q '/etc/ssl/redgres' "${libexec}/issue-tls.sh"
  grep -q 'redgres-fullchain.pem' "${libexec}/issue-tls.sh"
  grep -q 'redgres_tls_install_owned' "${libexec}/issue-tls.sh"
  grep -q 'client_tls_cert_file' "${libexec}/issue-tls.sh"
  ! grep -q 'chown root:ssl-cert "${dir}" 2>/dev/null || true' "${libexec}/issue-tls.sh"
  [[ -f "${sysd}/redgres-tls-issue.path" ]]
  grep -q 'PathExists=/var/lib/redgres/tls-issue.request' "${sysd}/redgres-tls-issue.path"
  grep -q 'PathChanged=/var/lib/redgres/tls-issue.request' "${sysd}/redgres-tls-issue.path"
  ! grep -q '^EnvironmentFile=' "${sysd}/redgres-tls-issue.service"
  grep -q '^StateDirectory=redgres-tls$' "${sysd}/redgres-tls-issue.service"
  grep -q 'PathExists=/var/lib/redgres-tls/active.request' "${sysd}/redgres-tls-issue.path"
  grep -q '^ExecStart=/usr/libexec/redgres/issue-tls.sh$' "${sysd}/redgres-tls-issue.service"
  [[ -x "${hook}/redgres-copy-certs.sh" ]]
  grep -q '/etc/ssl/redgres' "${hook}/redgres-copy-certs.sh"
  ! grep -Eiq 'eyJ|BEGIN PRIVATE|canary-must-not-appear' "${libexec}/issue-tls.sh"
) 2>&1" || domain_runtime_rc=$?
if [[ "${domain_runtime_rc}" -eq 0 ]]; then
  pass 'domain runtime units install from deploy/systemd without embedding tokens'
else
  fail "domain runtime unit install (rc=${domain_runtime_rc})"
  printf '%s\n' "${domain_runtime_err}" >&2
fi

tls_helper_rc=0
(
  req="${tmpdir}/tls-issue.request"
  result="${tmpdir}/tls-issue.result"
  creds="${tmpdir}/certbot-dns.ini"
  targets="${tmpdir}/tls-targets"
  lineage_state="${tmpdir}/tls-lineage"
  live="${tmpdir}/le-live/db.example.com"
  certdir="${tmpdir}/svc-tls"
  fake="${tmpdir}/fake-certbot"
  fake_openssl="${tmpdir}/fake-openssl"
  mkdir -p "${live}"
  printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
  printf '%s\n' 'dns_cloudflare_api_token = canary-must-not-appear' >"${creds}"
  printf '%s\n' 'pgbouncer=1' 'pgbouncer_user=postgres' >"${targets}"
  printf '%s\n' '#!/bin/sh' "mkdir -p '${live}'" "printf 'chain' >'${live}/fullchain.pem'" "printf 'key' >'${live}/privkey.pem'" 'exit 0' >"${fake}"
  chmod +x "${fake}"
  printf '%s\n' '#!/bin/sh' \
    'case "$*" in *"subjectAltName"*) echo "X509v3 Subject Alternative Name: DNS:db.example.com, DNS:rs.example.com" ;; esac' \
    'exit 0' >"${fake_openssl}"
  chmod +x "${fake_openssl}"
  export REDGRES_TLS_ISSUE_REQUEST_FILE="${req}"
  export REDGRES_TLS_ISSUE_RESULT_FILE="${result}"
  export REDGRES_CERTBOT_DNS_TOKEN_FILE="${creds}"
  export REDGRES_CERT_LIVE_DIR="${tmpdir}/le-live"
  export REDGRES_TLS_CERT_DIR="${certdir}"
  export REDGRES_CERTBOT_BIN="${fake}"
  export REDGRES_OPENSSL_BIN="${fake_openssl}"
  export REDGRES_TLS_TARGETS_FILE="${targets}"
  export REDGRES_TLS_LINEAGE_FILE="${lineage_state}"
  export REDGRES_PGBOUNCER_INI="${tmpdir}/pgbouncer.ini"
  printf '%s\n' 'client_tls_sslmode = require' 'client_tls_cert_file = /etc/ssl/certs/ssl-cert-snakeoil.pem' 'client_tls_key_file = /etc/ssl/private/ssl-cert-snakeoil.key' >"${REDGRES_PGBOUNCER_INI}"
  bash "${deploy_dir}/systemd/issue-tls.sh"
  [[ -f "${certdir}/fullchain.pem" ]]
  grep -q "client_tls_cert_file = ${certdir}/fullchain.pem" "${REDGRES_PGBOUNCER_INI}"
  grep -q "client_tls_key_file = ${certdir}/privkey.pem" "${REDGRES_PGBOUNCER_INI}"
  grep -q '^issued$' "${result}"
  grep -q '^db_status=certificate_prepared$' "${result}"
  grep -q '^rs_status=certificate_prepared$' "${result}"
  grep -q "${live}" "${lineage_state}" || exit 1
  [[ ! -f "${req}" ]]
  ! grep -q 'canary-must-not-appear' "${result}"

  # Without the root-owned target manifest, service configuration is preserved.
  cp "${REDGRES_PGBOUNCER_INI}" "${tmpdir}/pgbouncer.before"
  printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
  export REDGRES_TLS_TARGETS_FILE="${tmpdir}/missing-targets"
  bash "${deploy_dir}/systemd/issue-tls.sh"
  cmp "${tmpdir}/pgbouncer.before" "${REDGRES_PGBOUNCER_INI}" || exit 1
  export REDGRES_TLS_TARGETS_FILE="${targets}"

  # A valid matching suffixed lineage must be copied without another public ACME order.
  mv "${tmpdir}/le-live/db.example.com" "${tmpdir}/le-live/db.example.com-0001"
  printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
  fake_certbot_guard="${tmpdir}/fake-certbot-guard"
  printf '%s\n' '#!/bin/sh' 'echo certbot-must-not-run >&2' 'exit 91' >"${fake_certbot_guard}"
  chmod +x "${fake_certbot_guard}"
  export REDGRES_OPENSSL_BIN="${fake_openssl}"
  export REDGRES_CERTBOT_BIN="${fake_certbot_guard}"
  bash "${deploy_dir}/systemd/issue-tls.sh"
  grep -q '^issued$' "${result}"
  [[ ! -f "${req}" ]]
  grep -q 'db.example.com-0001' "${lineage_state}" || exit 1

  # A matching suffixed lineage below the validity threshold is renewed by its
  # own Certbot name; canonical may belong to a different SAN set.
  expiry_marker="${tmpdir}/renewed-expiring-lineage"
  certbot_args="${tmpdir}/certbot-args"
  fake_expiring_openssl="${tmpdir}/fake-expiring-openssl"
  printf '%s\n' '#!/bin/sh' \
    "case \"\$*\" in *subjectAltName*) echo 'X509v3 Subject Alternative Name: DNS:db.example.com, DNS:rs.example.com'; exit 0 ;; *-checkend*) test -f '${expiry_marker}' ;; *) exit 0 ;; esac" >"${fake_expiring_openssl}"
  chmod +x "${fake_expiring_openssl}"
  fake_renew="${tmpdir}/fake-certbot-renew"
  printf '%s\n' '#!/bin/sh' "printf '%s\n' \"\$@\" >'${certbot_args}'" "touch '${expiry_marker}'" 'exit 0' >"${fake_renew}"
  chmod +x "${fake_renew}"
  printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
  export REDGRES_OPENSSL_BIN="${fake_expiring_openssl}"
  export REDGRES_CERTBOT_BIN="${fake_renew}"
  bash "${deploy_dir}/systemd/issue-tls.sh"
  grep -A1 -x -- '--cert-name' "${certbot_args}" | grep -qx 'db.example.com-0001'
  export REDGRES_OPENSSL_BIN="${fake_openssl}"
  export REDGRES_CERTBOT_BIN="${fake_certbot_guard}"

  # The global renewal hook ignores unrelated Certbot lineages.
  unrelated="${tmpdir}/le-live/unrelated.example.com"
  mkdir -p "${unrelated}"
  printf '%s\n' 'unrelated-chain' >"${unrelated}/fullchain.pem"
  printf '%s\n' 'unrelated-key' >"${unrelated}/privkey.pem"
  printf '%s\n' 'expected-chain' >"${certdir}/fullchain.pem"
  RENEWED_LINEAGE="${unrelated}" bash "${deploy_dir}/systemd/redgres-copy-certs.sh"
  grep -q '^expected-chain$' "${certdir}/fullchain.pem" || exit 1

  # A matching lineage updates only the explicitly selected PostgreSQL target.
  pgroot="${tmpdir}/postgresql"
  mkdir -p "${pgroot}/18/main/conf.d"
  printf '%s\n' "ssl = on" "ssl_cert_file = '/old/cert'" "ssl_key_file = '/old/key'" >"${pgroot}/18/main/conf.d/redgres-ssl.conf"
  printf '%s\n' 'postgres_cluster=18/main' 'pgbouncer=0' 'pgbouncer_user=postgres' >"${targets}"
  export REDGRES_POSTGRES_CONFIG_ROOT="${pgroot}"
  RENEWED_LINEAGE="${tmpdir}/le-live/db.example.com-0001" bash "${deploy_dir}/systemd/redgres-copy-certs.sh"
  grep -q "ssl_cert_file = '${certdir}/fullchain.pem'" "${pgroot}/18/main/conf.d/redgres-ssl.conf" || exit 1

  # A backup failure after earlier certificate writes restores every prior
  # destination. This exercises both the issue helper and the renewal hook.
  printf '%s\n' 'old-chain' >"${certdir}/fullchain.pem"
  printf '%s\n' 'old-key' >"${certdir}/privkey.pem"
  printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
  export REDGRES_TLS_TXN_BACKUP_FAIL_AFTER=2
  if bash "${deploy_dir}/systemd/issue-tls.sh" >/dev/null 2>&1; then
    exit 1
  fi
  grep -qx 'old-chain' "${certdir}/fullchain.pem" || exit 1
  grep -qx 'old-key' "${certdir}/privkey.pem" || exit 1
  if RENEWED_LINEAGE="${tmpdir}/le-live/db.example.com-0001" bash "${deploy_dir}/systemd/redgres-copy-certs.sh" >/dev/null 2>&1; then
    exit 1
  fi
  grep -qx 'old-chain' "${certdir}/fullchain.pem" || exit 1
  grep -qx 'old-key' "${certdir}/privkey.pem" || exit 1
  unset REDGRES_TLS_TXN_BACKUP_FAIL_AFTER

  # Real OpenSSL proves a mismatched key is not reused, while a matching SAN
  # pair is accepted without opening a public ACME order.
  if command -v openssl >/dev/null 2>&1; then
    real_live="${tmpdir}/real-le-live/db.example.com"
    mkdir -p "${real_live}"
    openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
      -subj '/CN=db.example.com' \
      -addext 'subjectAltName=DNS:db.example.com,DNS:rs.example.com' \
      -keyout "${tmpdir}/matching.key" -out "${real_live}/fullchain.pem" >/dev/null 2>&1
    openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "${real_live}/privkey.pem" >/dev/null 2>&1
    real_guard_marker="${tmpdir}/real-certbot-called"
    real_guard="${tmpdir}/real-certbot-guard"
    printf '%s\n' '#!/bin/sh' "printf called >'${real_guard_marker}'" 'exit 91' >"${real_guard}"
    chmod +x "${real_guard}"
    export REDGRES_CERT_LIVE_DIR="${tmpdir}/real-le-live"
    export REDGRES_OPENSSL_BIN="$(command -v openssl)"
    export REDGRES_CERTBOT_BIN="${real_guard}"
    export REDGRES_TLS_TARGETS_FILE="${tmpdir}/missing-targets"
    printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
    if bash "${deploy_dir}/systemd/issue-tls.sh" >/dev/null 2>&1; then
      exit 1
    fi
    [[ -f "${real_guard_marker}" ]]
    cp "${tmpdir}/matching.key" "${real_live}/privkey.pem"
    rm -f "${real_guard_marker}"
    printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
    bash "${deploy_dir}/systemd/issue-tls.sh" >/dev/null
    [[ ! -f "${real_guard_marker}" ]]
    grep -q '^issued$' "${result}"

    super_live="${tmpdir}/super-le-live/db.example.com"
    mkdir -p "${super_live}"
    openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
      -subj '/CN=db.example.com' \
      -addext 'subjectAltName=DNS:db.example.com,DNS:rs.example.com,DNS:unrelated.example.com' \
      -keyout "${super_live}/privkey.pem" -out "${super_live}/fullchain.pem" >/dev/null 2>&1
    export REDGRES_CERT_LIVE_DIR="${tmpdir}/super-le-live"
    for min_valid in 1 999999999; do
      export REDGRES_TLS_MIN_VALID_SECONDS="${min_valid}"
      rm -f "${real_guard_marker}"
      printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
      if bash "${deploy_dir}/systemd/issue-tls.sh" >/dev/null 2>&1; then
        exit 1
      fi
      [[ -f "${real_guard_marker}" ]]
    done
    mixed_live="${tmpdir}/mixed-le-live/db.example.com"
    mkdir -p "${mixed_live}"
    openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
      -subj '/CN=db.example.com' \
      -addext 'subjectAltName=DNS:db.example.com,DNS:rs.example.com,IP:127.0.0.1' \
      -keyout "${mixed_live}/privkey.pem" -out "${mixed_live}/fullchain.pem" >/dev/null 2>&1
    export REDGRES_CERT_LIVE_DIR="${tmpdir}/mixed-le-live"
    export REDGRES_TLS_MIN_VALID_SECONDS=1
    rm -f "${real_guard_marker}"
    printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
    if bash "${deploy_dir}/systemd/issue-tls.sh" >/dev/null 2>&1; then
      exit 1
    fi
    [[ -f "${real_guard_marker}" ]]
    unset REDGRES_TLS_MIN_VALID_SECONDS
    export REDGRES_CERT_LIVE_DIR="${tmpdir}/le-live"
    export REDGRES_OPENSSL_BIN="${fake_openssl}"
    export REDGRES_CERTBOT_BIN="${fake_certbot_guard}"
    export REDGRES_TLS_TARGETS_FILE="${targets}"
  fi

  # Malformed root state fails closed on the matching renewal path.
  printf '%s\n' 'pgbouncer=maybe' >"${targets}"
  if RENEWED_LINEAGE="${tmpdir}/le-live/db.example.com-0001" bash "${deploy_dir}/systemd/redgres-copy-certs.sh" 2>/dev/null; then
    exit 1
  fi
  printf '%s\n' 'pgbouncer=1' 'pgbouncer_user=postgres' >"${targets}"

  # Atomic result publication replaces a hostile symlink instead of following it.
  canary="${tmpdir}/result-canary"
  printf '%s\n' 'must-stay' >"${canary}"
  rm -f "${result}"
  ln -s "${canary}" "${result}"

  # Failures expose only a stable class and a normalized retry time.
  rm -rf "${tmpdir}/le-live"
  printf '%s\n' 'db.example.com' 'rs.example.com' >"${req}"
  printf '%s\n' '#!/bin/sh' 'exit 1' >"${fake_openssl}"
  fake_rate_limited="${tmpdir}/fake-certbot-rate-limited"
  printf '%s\n' '#!/bin/sh' \
    'echo "too many certificates already issued; retry after 2026-08-31 10:43:35 UTC; raw-canary-must-not-appear" >&2' \
    'exit 1' >"${fake_rate_limited}"
  chmod +x "${fake_rate_limited}"
  export REDGRES_CERTBOT_BIN="${fake_rate_limited}"
  if bash "${deploy_dir}/systemd/issue-tls.sh" 2>/dev/null; then
    exit 1
  fi
  grep -q '^failed$' "${result}"
  grep -q '^reason=rate_limited$' "${result}"
  grep -q '^retry_after=2026-08-31T10:43:35Z$' "${result}"
  ! grep -q 'raw-canary' "${result}"
  [[ ! -L "${result}" ]]
  grep -q '^must-stay$' "${canary}"
  grep -q 'redgres_tls_publish_result' "${deploy_dir}/systemd/issue-tls.sh"
) || tls_helper_rc=$?
if [[ "${tls_helper_rc}" -eq 0 ]]; then
  pass 'issue-tls helper copies certs and never prints the API token'
else
  fail "issue-tls helper (rc=${tls_helper_rc})"
fi

uninstall_tls_rc=0
(
  ! grep -q '/etc/letsencrypt/live/db.redgres.com' "${deploy_dir%/*}/uninstall.sh"
  grep -q '\[\[ "${KEEP_REMOTE}" -eq 1 \]\] && return 0' "${deploy_dir%/*}/uninstall.sh"
  ! grep -q 'REDGRES_CERTBOT_BIN' "${deploy_dir%/*}/uninstall.sh"
  grep -q '/usr/bin/certbot delete' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'purge_tls_certs || exit 1' "${deploy_dir%/*}/uninstall.sh"
  ! grep -q 'certbot delete.*|| true' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'trusted lineage evidence was preserved for retry' "${deploy_dir%/*}/uninstall.sh"
  ! grep -q 'source "${snap}"' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'json.dump' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'redgres_quiesce_domain_tls || exit 1' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'remote_cloudflare_disconnect || exit 1' "${deploy_dir%/*}/uninstall.sh"
  grep -q 'Cloudflare cleanup was not confirmed; local state and credentials were preserved for retry' "${deploy_dir%/*}/uninstall.sh"
) || uninstall_tls_rc=$?
if [[ "${uninstall_tls_rc}" -eq 0 ]]; then
  pass 'uninstall preserves Certbot lineage in keep-remote mode without hard-coded domains'
else
  fail "uninstall Certbot lineage preservation (rc=${uninstall_tls_rc})"
fi

summary_load_rc=0
summary_load_err="$(
  stale="${tmpdir}/stale-clone"
  mkdir -p "${stale}/scripts"
  printf '%s\n' 'stale_summary_loaded=1' >"${stale}/scripts/install-summary.sh"
  (
    cd "${stale}"
    _self=''
    _dir="$(cd "$(dirname "${_self}")" 2>/dev/null && pwd || true)"
    if [[ -n "${_self}" && -f "${_self}" && -n "${_dir}" && -f "${_dir}/scripts/install-summary.sh" ]]; then
      exit 2
    fi
    [[ -f "${stale}/scripts/install-summary.sh" ]]
  )
  grep -q '\[\[ -n "${_self}" && -f "${_self}" \]\]' "${deploy_dir%/*}/upgrade.sh"
  grep -q 'redgres_ensure_domain_secret_env' "${deploy_dir%/*}/upgrade.sh"
) 2>&1" || summary_load_rc=$?
if [[ "${summary_load_rc}" -eq 0 ]]; then
  pass 'curl|bash upgrade does not source cwd scripts/install-summary.sh'
else
  fail "curl|bash install-summary guard (rc=${summary_load_rc})"
  printf '%s\n' "${summary_load_err}" >&2
fi
# Live fresh as non-root must fail closed before inventory. Root is not exercised here
# (absolute /usr/bin/apt-get would bypass STUB_NAMES).
if [[ "${EUID}" -ne 0 ]]; then
  run_install \
    --non-interactive \
    --mode fresh-postgres \
    --postgres-version 18 \
    --redis-mode fresh \
    --redis-version 8.2 \
    --pgbouncer-mode disabled
  expect_status_keyword 'live fresh without root exits 1' 1 'live install requires root'
  case "${output}" in
    *'Inventory (read-only'*)
      fail 'live fresh without root must not inventory'
      ;;
    *)
      pass 'live fresh without root skips inventory'
      ;;
  esac
else
  pass 'live fresh without-root check skipped (dispatcher tests running as root)'
fi

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
