#!/usr/bin/env bash
# Installer lifecycle config parser (OPS-001 Partial).
# Values are parsed as inert data. Never source, eval, export, or print them.
set -euo pipefail

redgres_config_validate_lifecycle() {
  local config_path="$1"
  local config_fd config_size snapshot raw line key value
  local -A seen=()

  redgres_open_trusted_readonly '--config' "${config_path}" config_fd config_size
  if [[ "${config_size}" -gt 65536 ]]; then
    redgres_close_trusted_fd "${config_fd}"
    redgres_die 'installer config is too large'
  fi

  snapshot="$(/usr/bin/head -c "${config_size}" <&"${config_fd}"; builtin printf 'x')" || redgres_die 'installer config is not readable'
  redgres_close_trusted_fd "${config_fd}"
  raw="${snapshot%x}"
  if [[ "${#raw}" -ne "${config_size}" ]]; then
    redgres_die 'installer config contains NUL'
  fi

  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line%$'\r'}"
    [[ "${line}" != *$'\r'* ]] || redgres_die 'installer config has invalid line endings'
    [[ -n "${line}" && "${line}" != \#* ]] || continue
    if [[ ! "${line}" =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]]; then
      redgres_die 'installer config has invalid syntax'
    fi
    key="${BASH_REMATCH[1]}"
    value="${BASH_REMATCH[2]}"
    case "${key}" in
      POSTGRES_MODE|POSTGRES_MAJOR|PGBOUNCER_MODE|POSTGRES_EXTENSION_POLICY|POSTGRES_EXTENSION_PLAN_FILE) ;;
      *) redgres_die 'installer config contains an unsupported key' ;;
    esac
    [[ -z "${seen[${key}]+x}" ]] || redgres_die 'installer config contains a duplicate key'
    seen["${key}"]="${value}"
  done <<< "${raw}"
}
