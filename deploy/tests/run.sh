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
) 2>&1" || uninstall_checkout_rc=$?
if [[ "${uninstall_checkout_rc}" -eq 0 ]]; then
  pass 'uninstall git-checkout detector matches clone layout only'
else
  fail "uninstall git-checkout detector (rc=${uninstall_checkout_rc})"
  printf '%s\n' "${uninstall_checkout_err}" >&2
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
  grep -q "purge -y postgresql 'postgresql-\*'" "${deploy_dir%/*}/uninstall.sh"
  grep -q 'APT::Get::Assume-Yes=true' "${deploy_dir%/*}/uninstall.sh"
  grep -F -q 'exec bash "${_uninstall_tmp}" "$@" </dev/null' "${deploy_dir%/*}/uninstall.sh"
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
  redgres_finish_report
) 2>&1" || true
if [[ "${finish_out}" == *'+-'* && "${finish_out}" == *'127.0.0.1:5432'* && "${finish_out}" == *'127.0.0.1:6380'* && "${finish_out}" == *'127.0.0.1:8790'* && "${finish_out}" == *'http://203.0.113.10:8989'* && "${finish_out}" == *'admin / once-owner-secret'* && "${finish_out}" == *'1.0.5'* && "${finish_out}" == *'UFW            off'* && "${finish_out}" == *'198.51.100.20'* && "${finish_out}" == *'fresh-postgres'* && "${finish_out}" == *'resolute'* ]]; then
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
  redgres_finish_report
) 2>&1" || true
if [[ "${hidden_out}" == *'once-owner-secret'* ]]; then
  fail "finish report printed owner password without a TTY (out=${hidden_out})"
elif [[ "${hidden_out}" == *'shown on this terminal only'* ]] && grep -q 'admin / once-owner-secret' "${hidden_tty}"; then
  pass 'finish report omits owner password from stdout and writes it to the TTY sink'
else
  fail "finish report TTY omission (out=${hidden_out})"
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
  redgres_ensure_domain_secret_env "${env_file}"
  grep -q '^REDGRES_CLOUDFLARE_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-api-token$' "${env_file}"
  grep -q '^REDGRES_TUNNEL_TOKEN_FILE=/var/lib/redgres/secrets/cloudflared-tunnel-token$' "${env_file}"
  grep -q '^REDGRES_ENVIRONMENT=production$' "${env_file}"
  printf '%s\n' 'REDGRES_CLOUDFLARE_TOKEN_FILE=/custom/token' >"${env_file}.keep"
  printf '%s\n' 'REDGRES_ENVIRONMENT=production' >>"${env_file}.keep"
  REDGRES_SECRETS_DIR="${secrets_dir}"
  redgres_ensure_domain_secret_env "${env_file}.keep"
  grep -q '^REDGRES_CLOUDFLARE_TOKEN_FILE=/custom/token$' "${env_file}.keep"
  ! grep -q '^REDGRES_CLOUDFLARE_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-api-token$' "${env_file}.keep"
  grep -q '^REDGRES_TUNNEL_TOKEN_FILE=/var/lib/redgres/secrets/cloudflared-tunnel-token$' "${env_file}.keep"
  ! redgres_ensure_domain_secret_env "${env_file}.keep"
  [[ -d "${secrets_dir}" ]]
) 2>&1" || domain_env_rc=$?
if [[ "${domain_env_rc}" -eq 0 ]]; then
  pass 'domain secret env paths append and never overwrite an existing key'
else
  fail "domain secret env ensure (rc=${domain_env_rc})"
  printf '%s\n' "${domain_env_err}" >&2
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
