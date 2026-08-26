#!/usr/bin/env bash
# Shared installer helpers (OPS-001/002 Partial). Do not source operator --config.
set -euo pipefail

redgres_log() {
  printf '%s\n' "$*"
}

redgres_die() {
  printf '%s\n' "$*" >&2
  exit 1
}

redgres_not_implemented() {
  printf '%s\n' "$*" >&2
  exit 2
}

redgres_mode_is_group_or_world_writable() {
  local mode="$1"
  (( (8#${mode} & 8#022) != 0 ))
}

# Validate a path and every ancestor against the supported-host trust policy.
# The caller chooses the final object kind; symlinks are never accepted.
redgres_validate_trusted_path() {
  local label="$1"
  local candidate="$2"
  local kind="$3"
  local owner mode component
  local stat_bin='/usr/bin/stat'

  [[ "${candidate}" == /* ]] || redgres_die "${label} is not trusted"
  [[ -x "${stat_bin}" && ! -L "${stat_bin}" ]] || redgres_die "trusted stat is unavailable"
  case "${kind}" in
    executable)
      [[ ! -L "${candidate}" && -f "${candidate}" && -x "${candidate}" ]] || redgres_die "${label} is not trusted"
      ;;
    file)
      [[ ! -L "${candidate}" && -f "${candidate}" ]] || redgres_die "${label} is not trusted"
      ;;
    directory)
      [[ ! -L "${candidate}" && -d "${candidate}" ]] || redgres_die "${label} is not trusted"
      ;;
    *)
      redgres_die "invalid trusted path kind"
      ;;
  esac

  read -r owner mode < <("${stat_bin}" -Lc '%u %a' -- "${candidate}") || redgres_die "${label} is not trusted"
  if [[ "${EUID}" -eq 0 ]]; then
    [[ "${owner}" == '0' ]] || redgres_die "${label} is not trusted"
  else
    [[ "${owner}" == '0' || "${owner}" == "${EUID}" ]] || redgres_die "${label} is not trusted"
  fi
  redgres_mode_is_group_or_world_writable "${mode}" && redgres_die "${label} is not trusted"

  if [[ "${kind}" == 'directory' ]]; then
    component="${candidate}"
  else
    component="${candidate%/*}"
    [[ -n "${component}" ]] || component='/'
  fi
  while :; do
    [[ ! -L "${component}" && -d "${component}" ]] || redgres_die "${label} is not trusted"
    read -r owner mode < <("${stat_bin}" -Lc '%u %a' -- "${component}") || redgres_die "${label} is not trusted"
    if [[ "${EUID}" -eq 0 ]]; then
      [[ "${owner}" == '0' ]] || redgres_die "${label} is not trusted"
    else
      [[ "${owner}" == '0' || "${owner}" == "${EUID}" ]] || redgres_die "${label} is not trusted"
    fi
    redgres_mode_is_group_or_world_writable "${mode}" && redgres_die "${label} is not trusted"
    [[ "${component}" == '/' ]] && break
    component="${component%/*}"
    [[ -n "${component}" ]] || component='/'
  done
}

# Open one trusted regular file and pin the descriptor to the validated path.
# The path is never echoed. The caller owns the returned descriptor.
redgres_open_trusted_readonly() {
  local label="$1"
  local candidate="$2"
  local out_fd_name="$3"
  local out_size_name="$4"
  local trusted_fd path_identity fd_identity trusted_size

  redgres_validate_trusted_path "${label}" "${candidate}" file
  exec {trusted_fd}<"${candidate}" || redgres_die "${label} is not trusted"
  path_identity="$(/usr/bin/stat -Lc '%d:%i:%s' -- "${candidate}")" || redgres_die "${label} is not trusted"
  fd_identity="$(/usr/bin/stat -Lc '%d:%i:%s' -- "/proc/self/fd/${trusted_fd}")" || redgres_die "${label} is not trusted"
  [[ "${path_identity}" == "${fd_identity}" ]] || redgres_die "${label} is not trusted"
  trusted_size="${fd_identity##*:}"
  [[ "${trusted_size}" =~ ^[0-9]+$ ]] || redgres_die "${label} is not trusted"
  printf -v "${out_fd_name}" '%s' "${trusted_fd}"
  printf -v "${out_size_name}" '%s' "${trusted_size}"
}

redgres_close_trusted_fd() {
  local opened_fd="$1"
  exec {opened_fd}<&-
}

redgres_require_trusted_unread_file() {
  local label="$1"
  local candidate="$2"
  local opened_fd opened_size
  redgres_open_trusted_readonly "${label}" "${candidate}" opened_fd opened_size
  redgres_close_trusted_fd "${opened_fd}"
}
