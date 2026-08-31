#!/usr/bin/env bash
set -euo pipefail

tests_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="${tests_dir%/deploy/tests}"
test_base="${HOME:-${repo_root}}"
test_root="$(mktemp -d "${test_base}/redgres-uninstall-cf-test.XXXXXX")"
python_bin=""
for candidate in "${REDGRES_TEST_PYTHON:-}" "$(command -v python3 2>/dev/null || true)" "$(command -v python 2>/dev/null || true)"; do
  [[ -n "${candidate}" ]] || continue
  if "${candidate}" -c '' >/dev/null 2>&1; then
    python_bin="${candidate}"
    break
  fi
done
[[ -n "${python_bin}" ]] || { printf 'python is required for uninstall Cloudflare tests\n' >&2; exit 1; }
cleanup() {
  case "${test_root}" in
    "${test_base}"/redgres-uninstall-cf-test.*) rm -rf -- "${test_root}" ;;
    *) printf 'refusing unsafe uninstall-test cleanup target\n' >&2 ;;
  esac
}
trap cleanup EXIT

# shellcheck disable=SC1091
REDGRES_UNINSTALL_FUNCTIONS_ONLY=1 source "${repo_root}/uninstall.sh"

clean_root="${test_root}/already-removed"
mkdir -p "${clean_root}"
! redgres_uninstall_path_footprint_present \
  "${clean_root}/opt" "${clean_root}/etc" "${clean_root}/var" "${clean_root}/unit"
mkdir -p "${clean_root}/etc"
redgres_uninstall_path_footprint_present \
  "${clean_root}/opt" "${clean_root}/etc" "${clean_root}/var" "${clean_root}/unit"
(
  docker() { return 1; }
  redgres_uninstall_local_footprint_present "${clean_root}/missing"
)
(
  id() { return 1; }
  getent() { return 2; }
  dpkg-query() { return 1; }
  docker() { return 0; }
  ss() { return 0; }
  ufw_mode=owned
  ufw() {
    [[ "${1:-}" == status ]] || return 1
    if [[ "${ufw_mode}" == owned ]]; then
      printf '%s\n' '8989/tcp ALLOW 198.51.100.2 # redgres-bootstrap'
    else
      printf '%s\n' '8989/tcp ALLOW Anywhere # another-application'
    fi
  }
  ! redgres_uninstall_local_footprint_present "${clean_root}/missing"
  ufw_mode=unowned
  redgres_uninstall_local_footprint_present "${clean_root}/missing"
)

already_removed_rc=0
already_removed_output="$(
  redgres_uninstall_local_footprint_present() { return 1; }
  redgres_uninstall_exit_if_already_removed 0 "${clean_root}/missing"
)" || already_removed_rc=$?
[[ "${already_removed_rc}" -eq 10 ]]
[[ "${already_removed_output}" == *'No installed Redgres footprint was detected'* ]]
[[ "${already_removed_output}" == *'Cloudflare ownership cannot be verified'* ]]
[[ "${already_removed_output}" == *'https://one.dash.cloudflare.com/networks/tunnels'* ]]
(
  redgres_uninstall_local_footprint_present() { return 0; }
  [[ -z "$(redgres_uninstall_exit_if_already_removed 0 "${clean_root}/etc")" ]]
)
[[ -z "$(redgres_uninstall_exit_if_already_removed 1 "${clean_root}/missing")" ]]

already_removed_line="$(grep -n 'redgres_uninstall_exit_if_already_removed' "${repo_root}/uninstall.sh" | tail -n1 | cut -d: -f1)"
confirm_line="$(grep -n '^[[:space:]]*confirm_uninstall$' "${repo_root}/uninstall.sh" | tail -n1 | cut -d: -f1)"
disconnect_line="$(grep -n 'remote_cloudflare_disconnect$' "${repo_root}/uninstall.sh" | tail -n1 | cut -d: -f1)"
[[ -n "${confirm_line}" && -n "${already_removed_line}" && -n "${disconnect_line}" ]]
(( confirm_line < already_removed_line ))
(( already_removed_line < disconnect_line ))
grep -q 'Warning: Cloudflare cleanup was not confirmed; continuing local purge' "${repo_root}/uninstall.sh"
grep -q 'CF_REMOTE_INCOMPLETE=1' "${repo_root}/uninstall.sh"
! grep -q 'remote_cloudflare_disconnect || exit 1' "${repo_root}/uninstall.sh"
! grep -q 'purge_tls_certs || exit 1' "${repo_root}/uninstall.sh"

systemctl_log="${test_root}/systemctl.log"
service_active=1
path_active=1
hang_unit=""
fail_unit=""
sleep() { :; }
systemctl() {
  case "${1:-}" in
    is-active)
      case "${3:-}" in
        cloudflared-redgres.service) [[ "${service_active}" -eq 1 ]] ;;
        cloudflared-redgres.path) [[ "${path_active}" -eq 1 ]] ;;
        *) return 1 ;;
      esac
      ;;
    is-failed) return 1 ;;
    stop)
      printf 'stop %s\n' "${2:-}" >>"${systemctl_log}"
      [[ "${2:-}" == cloudflared-redgres.service ]] && service_active=0
      [[ "${2:-}" == cloudflared-redgres.path ]] && path_active=0
      ;;
    --no-block)
      [[ "${2:-}" == start ]]
      unit="${3:-}"
      printf '%s start %s\n' "${1}" "${unit}" >>"${systemctl_log}"
      [[ "${unit}" == "${fail_unit}" ]] && return 1
      if [[ "${unit}" != "${hang_unit}" ]]; then
        [[ "${unit}" == cloudflared-redgres.service ]] && service_active=1
        [[ "${unit}" == cloudflared-redgres.path ]] && path_active=1
      fi
      return 0
      ;;
    *) return 0 ;;
  esac
}

redgres_uninstall_quiesce_cloudflare
[[ "${service_active}" -eq 0 ]]
[[ "${path_active}" -eq 0 ]]
grep -qx 'stop cloudflared-redgres.path' "${systemctl_log}"
grep -qx 'stop cloudflared-redgres.service' "${systemctl_log}"

quiesce_line="$(grep -n 'redgres_uninstall_quiesce_cloudflare || return 1' "${repo_root}/uninstall.sh" | head -n1 | cut -d: -f1)"
disconnect_line="$(grep -n 'remote_cloudflare_disconnect$' "${repo_root}/uninstall.sh" | head -n1 | cut -d: -f1)"
[[ -n "${quiesce_line}" && -n "${disconnect_line}" ]]
(( quiesce_line < disconnect_line ))

REDGRES_UNINSTALL_QUIESCE_GUARD=1
redgres_uninstall_restore_quiesced
grep -qx -- '--no-block start cloudflared-redgres.service' "${systemctl_log}"
grep -qx -- '--no-block start cloudflared-redgres.path' "${systemctl_log}"
[[ "${REDGRES_UNINSTALL_QUIESCE_GUARD}" == 0 ]]

service_active=0
path_active=0
hang_unit=cloudflared-redgres.service
REDGRES_UNINSTALL_QUIESCE_GUARD=1
REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE=1
REDGRES_UNINSTALL_TUNNEL_PATH_WAS_ACTIVE=0
restore_warning="$(redgres_uninstall_restore_quiesced 2>&1)"
[[ "${restore_warning}" == *'restoration was not confirmed within 3 seconds'* ]]

hang_unit=""
fail_unit=cloudflared-redgres.service
REDGRES_UNINSTALL_QUIESCE_GUARD=1
REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE=1
restore_warning="$(redgres_uninstall_restore_quiesced 2>&1)"
[[ "${restore_warning}" == *'could not request restart for cloudflared-redgres.service'* ]]

"${python_bin}" "${tests_dir}/uninstall_cloudflare_mock_test.py" "${repo_root}/uninstall.sh" "${test_root}"

REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE=1
REDGRES_UNINSTALL_TUNNEL_PATH_WAS_ACTIVE=1
redgres_uninstall_apply_cloudflare_result $'TUNNEL:deleted\nSTATUS:api_ok'
[[ "${CF_TUNNEL_STATUS}" == deleted && "${CF_API_STATUS}" == api_ok ]]
[[ "${REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE}" == 0 ]]
[[ "${REDGRES_UNINSTALL_TUNNEL_PATH_WAS_ACTIVE}" == 0 ]]

REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE=1
REDGRES_UNINSTALL_TUNNEL_PATH_WAS_ACTIVE=1
redgres_uninstall_apply_cloudflare_result $'TUNNEL:preserved\nSTATUS:api_partial'
[[ "${REDGRES_UNINSTALL_TUNNEL_WAS_ACTIVE}" == 1 ]]
[[ "${REDGRES_UNINSTALL_TUNNEL_PATH_WAS_ACTIVE}" == 1 ]]

printf 'uninstall_cloudflare_cleanup=pass\n'
