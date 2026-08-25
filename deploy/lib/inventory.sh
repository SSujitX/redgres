#!/usr/bin/env bash
# Host --version inventory for existing-mode --dry-run (OPS-002 Partial).
# Resolve PATH candidates once, validate them, then execute only the absolute
# trusted path. Do not start servers, source --config, or mutate.
# SQL SHOW / Redis INFO / PgBouncer SHOW VERSION are deferred.
set -euo pipefail

redgres_mode_is_group_or_world_writable() {
  local mode="$1"
  (( (8#${mode} & 8#022) != 0 ))
}

redgres_validate_host_binary() {
  local name="$1"
  local candidate="$2"
  local owner mode component
  local stat_bin='/usr/bin/stat'

  [[ "${candidate}" == /* ]] || redgres_die "${name} is not trusted"
  [[ ! -L "${candidate}" && -f "${candidate}" && -x "${candidate}" ]] || redgres_die "${name} is not trusted"
  [[ -x "${stat_bin}" && ! -L "${stat_bin}" ]] || redgres_die "trusted stat is unavailable"
  read -r owner mode < <("${stat_bin}" -Lc '%u %a' -- "${candidate}") || redgres_die "${name} is not trusted"
  if [[ "${EUID}" -eq 0 ]]; then
    [[ "${owner}" == "0" ]] || redgres_die "${name} is not trusted"
  else
    [[ "${owner}" == "0" || "${owner}" == "${EUID}" ]] || redgres_die "${name} is not trusted"
  fi
  redgres_mode_is_group_or_world_writable "${mode}" && redgres_die "${name} is not trusted"

  if [[ "${EUID}" -ne 0 ]]; then
    return 0
  fi
  component="${candidate%/*}"
  [[ -n "${component}" ]] || component='/'
  while :; do
    [[ ! -L "${component}" && -d "${component}" ]] || redgres_die "${name} is not trusted"
    read -r owner mode < <("${stat_bin}" -Lc '%u %a' -- "${component}") || redgres_die "${name} is not trusted"
    [[ "${owner}" == "0" ]] || redgres_die "${name} is not trusted"
    redgres_mode_is_group_or_world_writable "${mode}" && redgres_die "${name} is not trusted"
    [[ "${component}" == "/" ]] && break
    component="${component%/*}"
    [[ -n "${component}" ]] || component='/'
  done
}

redgres_resolve_host_binary() {
  local name="$1"
  local candidate
  candidate="$(command -v -- "${name}")" || redgres_die "${name} not found"
  redgres_validate_host_binary "${name}" "${candidate}"
  printf '%s' "${candidate}"
}

redgres_read_host_version() {
  local bin="$1"
  local bin_path
  local out status
  bin_path="$(redgres_resolve_host_binary "${bin}")"
  set +e
  out="$("${bin_path}" --version)"
  status=$?
  set -e
  out="${out//$'\r'/}"
  if [[ "${status}" -ne 0 || -z "${out}" ]]; then
    redgres_die "${bin} --version unparseable"
  fi
  printf '%s' "${out}"
}

# Sets redgres_detected (matching line) and redgres_unit (PG major).
redgres_parse_postgres_version() {
  local raw="$1"
  local line rest token
  redgres_detected=''
  redgres_unit=''
  while IFS= read -r line || [[ -n "${line}" ]]; do
    case "${line}" in
      *'(PostgreSQL) '*)
        rest="${line#*'(PostgreSQL) '}"
        token="${rest%% *}"
        if [[ "${token}" =~ ^([0-9]+)\.([0-9]+)$ ]]; then
          redgres_detected="${line}"
          redgres_unit="${BASH_REMATCH[1]}"
          return 0
        fi
        ;;
    esac
  done <<< "${raw}"
  return 1
}

# Sets redgres_detected (matching line) and redgres_unit (Redis X.Y series).
redgres_parse_redis_version() {
  local raw="$1"
  local line rest token
  redgres_detected=''
  redgres_unit=''
  while IFS= read -r line || [[ -n "${line}" ]]; do
    case "${line}" in
      *'Redis server v='*)
        rest="${line#*Redis server v=}"
        token="${rest%% *}"
        if [[ "${token}" =~ ^([0-9]+)\.([0-9]+)$ ]]; then
          redgres_detected="${line}"
          redgres_unit="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
          return 0
        fi
        if [[ "${token}" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
          redgres_detected="${line}"
          redgres_unit="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
          return 0
        fi
        ;;
    esac
  done <<< "${raw}"
  return 1
}

# Sets redgres_detected (matching line). No supported-version allow-list.
redgres_parse_pgbouncer_version() {
  local raw="$1"
  local line token
  local -a tokens
  redgres_detected=''
  while IFS= read -r line || [[ -n "${line}" ]]; do
    IFS=$' \t' read -r -a tokens <<< "${line}"
    for token in "${tokens[@]}"; do
      if [[ "${token}" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
        redgres_detected="${line}"
        return 0
      fi
    done
  done <<< "${raw}"
  return 1
}

redgres_inventory_postgres() {
  local raw expect_label
  if [[ "${mode}" != "existing-postgres" ]]; then
    redgres_log "postgres: skipped (${mode})"
    return 0
  fi
  raw="$(redgres_read_host_version postgres)"
  if ! redgres_parse_postgres_version "${raw}"; then
    redgres_die "postgres --version unparseable"
  fi
  case "${redgres_unit}" in
    17|18) ;;
    *) redgres_die "postgres major ${redgres_unit} is unsupported" ;;
  esac
  expect_label="${expect_postgres_major:-unset}"
  if [[ -n "${expect_postgres_major}" && "${redgres_unit}" != "${expect_postgres_major}" ]]; then
    redgres_die "postgres major ${redgres_unit} does not match expect ${expect_postgres_major} (mismatch)"
  fi
  redgres_log "postgres: detected=${redgres_detected} major=${redgres_unit} expect=${expect_label} result=ok"
}

redgres_inventory_redis() {
  local raw expect_label
  if [[ "${redis_mode}" != "existing" ]]; then
    redgres_log "redis: skipped (${redis_mode})"
    return 0
  fi
  raw="$(redgres_read_host_version redis-server)"
  if ! redgres_parse_redis_version "${raw}"; then
    redgres_die "redis-server --version unparseable"
  fi
  case "${redgres_unit}" in
    8.2|8.8) ;;
    *) redgres_die "redis series ${redgres_unit} is unsupported" ;;
  esac
  expect_label="${expect_redis_series:-unset}"
  if [[ -n "${expect_redis_series}" && "${redgres_unit}" != "${expect_redis_series}" ]]; then
    redgres_die "redis series ${redgres_unit} does not match expect ${expect_redis_series} (mismatch)"
  fi
  redgres_log "redis: detected=${redgres_detected} series=${redgres_unit} expect=${expect_label} result=ok"
}

redgres_inventory_pgbouncer() {
  local raw
  case "${pgbouncer_mode}" in
    existing) ;;
    *)
      redgres_log "pgbouncer: skipped (${pgbouncer_mode})"
      return 0
      ;;
  esac
  raw="$(redgres_read_host_version pgbouncer)"
  if ! redgres_parse_pgbouncer_version "${raw}"; then
    redgres_die "pgbouncer --version unparseable"
  fi
  redgres_log "pgbouncer: detected=${redgres_detected} result=recorded"
}

redgres_inventory_dry_run() {
  redgres_log 'Inventory (read-only, host --version; not SQL SHOW/INFO):'
  redgres_inventory_postgres
  redgres_inventory_redis
  redgres_inventory_pgbouncer
}
