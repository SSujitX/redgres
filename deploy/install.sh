#!/bin/bash -p
# Fail-closed installer dispatcher. OPS-001 / OPS-002 / OPS-003 / OPS-004 / OPS-005 / OPS-006 / OPS-007 Partial.
set -euo pipefail

case "$-" in
  *p*) ;;
  *)
    builtin printf '%s\n' 'installer must be executed directly or with /bin/bash -p' >&2
    exit 1
    ;;
esac

# Capture the caller's PATH as inventory input only. Installer implementation
# commands must resolve from this fixed, administrator-owned system path.
REDGRES_HOST_SEARCH_PATH="${PATH-}"
readonly REDGRES_HOST_SEARCH_PATH
PATH='/usr/sbin:/usr/bin:/sbin:/bin'
LC_ALL='C'
IFS=$' \t\n'
umask 077
export LC_ALL PATH
builtin hash -r
builtin unset BASH_ENV ENV CDPATH GLOBIGNORE BASH_COMPAT POSIXLY_CORRECT
builtin unset LD_PRELOAD LD_LIBRARY_PATH LD_AUDIT

redgres_bootstrap_die() {
  builtin printf '%s\n' "$*" >&2
  exit 1
}

# This batched validator intentionally exists before common.sh is sourced. It
# uses a non-symlink GNU or rust-coreutils binary (Ubuntu 24.04 /usr/bin/stat,
# Ubuntu 26.04 /usr/bin/gnustat or /usr/lib/cargo/bin/coreutils/coreutils).
# Keep the candidate list in sync with redgres_pick_coreutils_applet in common.sh.
redgres_bootstrap_pick_stat() {
  local candidate base
  REDGRES_STAT_BIN=''
  REDGRES_STAT_PREFIX_ARGS=()
  for candidate in \
    /usr/bin/stat \
    /usr/bin/gnustat \
    /usr/lib/cargo/bin/coreutils/stat \
    /usr/lib/cargo/bin/coreutils/coreutils \
    /usr/bin/coreutils
  do
    [[ -x "${candidate}" && -f "${candidate}" && ! -L "${candidate}" ]] || continue
    REDGRES_STAT_BIN="${candidate}"
    base="${candidate##*/}"
    if [[ "${base}" == 'coreutils' ]]; then
      REDGRES_STAT_PREFIX_ARGS=(stat)
    fi
    return 0
  done
  return 1
}

redgres_bootstrap_stat() {
  "${REDGRES_STAT_BIN}" "${REDGRES_STAT_PREFIX_ARGS[@]}" "$@"
}

redgres_bootstrap_validate_source_tree() {
  local trusted_entrypoint="$1"
  local trusted_script_dir="$2"
  shift 2
  local candidate component extra kind line mode owner
  local candidate_index
  local -a paths=("${trusted_entrypoint}")
  local -a kinds=('file')
  local -a metadata=()

  component="${trusted_entrypoint%/*}"
  [[ -n "${component}" ]] || component='/'
  while :; do
    paths+=("${component}")
    kinds+=('directory')
    [[ "${component}" == '/' ]] && break
    component="${component%/*}"
    [[ -n "${component}" ]] || component='/'
  done
  paths+=("${trusted_script_dir}/lib")
  kinds+=('directory')
  for candidate in "$@"; do
    paths+=("${candidate}")
    kinds+=('file')
  done

  redgres_bootstrap_pick_stat || redgres_bootstrap_die 'trusted stat is unavailable'
  for ((candidate_index = 0; candidate_index < ${#paths[@]}; candidate_index++)); do
    candidate="${paths[${candidate_index}]}"
    kind="${kinds[${candidate_index}]}"
    [[ "${candidate}" == /* && ! -L "${candidate}" ]] || redgres_bootstrap_die 'installer source tree is not trusted'
    case "${kind}" in
      file) [[ -f "${candidate}" ]] || redgres_bootstrap_die 'installer source tree is not trusted' ;;
      directory) [[ -d "${candidate}" ]] || redgres_bootstrap_die 'installer source tree is not trusted' ;;
    esac
  done

  builtin mapfile -t metadata < <(redgres_bootstrap_stat -Lc '%u %a' -- "${paths[@]}")
  [[ "${#metadata[@]}" -eq "${#paths[@]}" ]] || redgres_bootstrap_die 'installer source tree is not trusted'
  for line in "${metadata[@]}"; do
    builtin read -r owner mode extra <<< "${line}"
    [[ -z "${extra-}" && "${owner}" =~ ^[0-9]+$ && "${mode}" =~ ^[0-7]+$ ]] || redgres_bootstrap_die 'installer source tree is not trusted'
    if [[ "${EUID}" -eq 0 ]]; then
      [[ "${owner}" == '0' ]] || redgres_bootstrap_die 'installer source tree is not trusted'
    else
      [[ "${owner}" == '0' || "${owner}" == "${EUID}" ]] || redgres_bootstrap_die 'installer source tree is not trusted'
    fi
    (( (8#${mode} & 8#022) == 0 )) || redgres_bootstrap_die 'installer source tree is not trusted'
  done
}

entrypoint="${BASH_SOURCE[0]}"
case "${entrypoint}" in
  /*) ;;
  ./*) entrypoint="$(builtin pwd -P)/${entrypoint#./}" ;;
  */*) entrypoint="$(builtin pwd -P)/${entrypoint}" ;;
  *) redgres_bootstrap_die 'installer must be invoked by path' ;;
esac
case "${entrypoint}" in
  */./*|*/../*|*/.|*/..)
    redgres_bootstrap_die "installer entrypoint is not trusted"
    ;;
esac
entrypoint_dir="${entrypoint%/*}"
[[ -n "${entrypoint_dir}" ]] || entrypoint_dir='/'
script_dir="$(builtin cd -- "${entrypoint_dir}" && builtin pwd -P)" || redgres_bootstrap_die "installer source tree is not trusted"
[[ "${script_dir}" == "${entrypoint_dir}" ]] || redgres_bootstrap_die "installer source tree is not trusted"

source_files=(
  "${script_dir}/lib/common.sh"
  "${script_dir}/lib/config.sh"
  "${script_dir}/lib/inventory.sh"
  "${script_dir}/lib/cloudflare_inventory.sh"
  "${script_dir}/lib/tls_inventory.sh"
  "${script_dir}/lib/verify.sh"
  "${script_dir}/lib/release.sh"
  "${script_dir}/lib/postgres_plan.sh"
  "${script_dir}/lib/backup.sh"
  "${script_dir}/lib/postgres_extensions.sh"
  "${script_dir}/lib/pins.sh"
  "${script_dir}/lib/mutate.sh"
  "${script_dir}/lib/app_install.sh"
)
redgres_bootstrap_validate_source_tree "${entrypoint}" "${script_dir}" "${source_files[@]}"

source_fds=()
source_identity_paths=()
for source_file in "${source_files[@]}"; do
  exec {source_fd}<"${source_file}" || redgres_bootstrap_die 'installer source tree is not trusted'
  source_fds+=("${source_fd}")
  source_identity_paths+=("${source_file}" "/proc/self/fd/${source_fd}")
done

source_identity_metadata=()
builtin mapfile -t source_identity_metadata < <(redgres_bootstrap_stat -Lc '%u %a %d:%i' -- "${source_identity_paths[@]}")
[[ "${#source_identity_metadata[@]}" -eq "${#source_identity_paths[@]}" ]] || redgres_bootstrap_die 'installer source tree is not trusted'
for ((source_index = 0; source_index < ${#source_identity_metadata[@]}; source_index += 2)); do
  builtin read -r source_path_owner source_path_mode source_path_identity source_extra <<< "${source_identity_metadata[${source_index}]}"
  builtin read -r source_fd_owner source_fd_mode source_fd_identity source_fd_extra <<< "${source_identity_metadata[$((source_index + 1))]}"
  [[ -z "${source_extra-}" && -z "${source_fd_extra-}" ]] || redgres_bootstrap_die 'installer source tree is not trusted'
  [[ "${source_path_owner}" =~ ^[0-9]+$ && "${source_path_mode}" =~ ^[0-7]+$ ]] || redgres_bootstrap_die 'installer source tree is not trusted'
  [[ "${source_fd_owner}" == "${source_path_owner}" && "${source_fd_mode}" == "${source_path_mode}" ]] || redgres_bootstrap_die 'installer source tree is not trusted'
  [[ "${source_fd_identity}" == "${source_path_identity}" ]] || redgres_bootstrap_die 'installer source tree is not trusted'
  if [[ "${EUID}" -eq 0 ]]; then
    [[ "${source_fd_owner}" == '0' ]] || redgres_bootstrap_die 'installer source tree is not trusted'
  else
    [[ "${source_fd_owner}" == '0' || "${source_fd_owner}" == "${EUID}" ]] || redgres_bootstrap_die 'installer source tree is not trusted'
  fi
  (( (8#${source_fd_mode} & 8#022) == 0 )) || redgres_bootstrap_die 'installer source tree is not trusted'
done

for source_fd in "${source_fds[@]}"; do
  # shellcheck disable=SC1090
  builtin source "/dev/fd/${source_fd}"
  exec {source_fd}<&-
done
unset source_fd source_fds source_file source_files source_identity_paths source_identity_metadata source_index
unset source_path_owner source_path_mode source_path_identity source_fd_owner source_fd_mode source_fd_identity source_extra source_fd_extra
unset entrypoint entrypoint_dir

usage() {
  cat <<'EOF'
Usage:
  deploy/install.sh --help
  deploy/install.sh --non-interactive --dry-run --mode existing-postgres|fresh-postgres
      --redis-mode existing|fresh --pgbouncer-mode existing|fresh|disabled
      [--postgres-version 17|18] [--expect-postgres-major 17|18]
      [--redis-version 8.2|8.8] [--expect-redis-series 8.2|8.8]
      [--config PATH] [--extension-plan PATH] [--approve-postgres-restart]
  deploy/install.sh verify --non-interactive --dry-run --config PATH
  deploy/install.sh update --non-interactive [--dry-run] --release PATH
  deploy/install.sh rollback --non-interactive [--dry-run] --to VERSION
  deploy/install.sh backup --non-interactive --dry-run --config PATH
  deploy/install.sh postgres-plan --config PATH --extension-plan PATH
  deploy/install.sh postgres-extensions apply --non-interactive --dry-run --config PATH --extension-plan PATH [--approve-postgres-restart]

This Partial prints the planned stage list on --dry-run and inventories PATH
host --version for existing PostgreSQL/Redis/PgBouncer. verify --dry-run prints
a skip matrix (result=partial); it does not probe DNS/Cloudflare/public TLS.
update/rollback --dry-run print skip matrices (result=partial). Live update
(without --dry-run) verifies adjacent SHA256SUMS, extracts under
/opt/redgres/releases/<VERSION>, switches current, writes systemd unit, and
probes healthz. Live rollback switches current to --to VERSION only. Neither
reverses PostgreSQL/Redis/vault/credentials/DNS/schema.
postgres-plan --config PATH --extension-plan PATH is a read-only extension-plan
validator: it checks policy, capability IDs, explicit databases, and scheduler
rules, then prints a plan preview and skip matrix (result=partial). It never
sources --config, never mutates, and never resolves packages/preload/restart.
backup --non-interactive --dry-run --config PATH prints a skip matrix
(result=partial); it does not invoke pg_dump/pg_restore, Redis BGSAVE/LASTSAVE,
SQLite backup, checksums, pruning, or off-host copy. Live backup/restore is
installer-recovery.
postgres-extensions apply --non-interactive --dry-run validates the extension
plan and prints a skip matrix (result=partial); it never resolves packages,
reads live cluster state, merges preload, restarts PostgreSQL, or runs DDL.
It does not install packages, pull images, write systemd, open a firewall, or
change DNS/Cloudflare on --dry-run. Live install without --dry-run (fresh-postgres
+ fresh Redis) uses Ubuntu 24.04 (noble) and 26.04 (resolute): PGDG named packages
resolved then installed as pkg=version, digest-pinned Redis images, then pg_isready
and redis PING. It then writes /etc/redgres/redgres.env, downloads the latest GitHub
release tarball + SHA256SUMS, applies it through update, and prints a boxed finish report
(bootstrap URL, versions, loopback listeners, UFW, owner login). Owner create-owner
--generate --password-fifo runs only when /dev/tty can be opened and the downloaded
binary advertises -password-fifo; otherwise --generate prints the password to the TTY.
The password is shown once in the finish box on a TTY, not in the install log.

Exit 0: --help, valid --non-interactive --dry-run plan, skip matrix, valid postgres-plan, backup or postgres-extensions apply skip matrix
Exit 1: unsupported, incomplete, missing, unparseable, mismatched selection, or invalid extension plan
Exit 2: live existing-mode install, live verify, or other subcommand not implemented
Live install without --dry-run is Partial: fresh-postgres + fresh Redis + application tarball + TTY-safe owner bootstrap + boxed finish report (URL, versions, listeners, UFW, owner login). Loopback DB listeners; no public UFW DB ports.

Majors/series only (not Hub tags). latest and latest-tested are rejected.
EOF
}

print_stages() {
  cat <<'EOF'
Planned stages (not executed):
1. Preflight
2. Inventory
3. Safety gate
4. Packages
5. Identity/filesystem
6. Redis
7. PostgreSQL/PgBouncer
8. TLS/DNS (deferred to Domain wizard / dry-run inventory only)
9. Application release
10. Cloudflare (deferred to Domain wizard / dry-run inventory only)
11. Firewall (deferred to bootstrap confirm / dry-run inventory only)
12. End-to-end verify
13. Report
EOF
}

dry_run=0
non_interactive=0
mode=''
postgres_version=''
expect_postgres_major=''
redis_mode=''
redis_version=''
expect_redis_series=''
pgbouncer_mode=''
config_path=''
extension_plan=''
approve_restart=0

is_pg_major() {
  case "$1" in
    17|18) return 0 ;;
    *) return 1 ;;
  esac
}

is_redis_series() {
  case "$1" in
    8.2|8.8) return 0 ;;
    *) return 1 ;;
  esac
}

if [[ "${1:-}" == "verify" ]]; then
  shift
  if [[ "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi
  verify_dry_run=0
  verify_non_interactive=0
  verify_config_path=''
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --non-interactive)
        verify_non_interactive=1
        shift
        ;;
      --dry-run)
        verify_dry_run=1
        shift
        ;;
      --config)
        [[ $# -ge 2 ]] || redgres_die "missing value for --config"
        verify_config_path="$2"
        shift 2
        ;;
      -*)
        redgres_die "unknown flag: $1"
        ;;
      *)
        redgres_die "unknown argument: $1"
        ;;
    esac
  done
  if [[ "${verify_non_interactive}" -ne 1 ]]; then
    redgres_die "--non-interactive is required"
  fi
  if [[ -z "${verify_config_path}" ]]; then
    redgres_die "--config is required"
  fi
  # Trusted-path/identity check only. Never source, eval, or print contents.
  redgres_require_trusted_unread_file '--config' "${verify_config_path}"
  if [[ "${verify_dry_run}" -ne 1 ]]; then
    redgres_not_implemented "verify without --dry-run is not implemented"
  fi
  redgres_verify_dry_run
  exit 0
fi

if [[ "${1:-}" == "update" ]]; then
  shift
  if [[ "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi
  update_dry_run=0
  update_non_interactive=0
  update_release_path=''
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --non-interactive)
        update_non_interactive=1
        shift
        ;;
      --dry-run)
        update_dry_run=1
        shift
        ;;
      --release)
        [[ $# -ge 2 ]] || redgres_die "missing value for --release"
        update_release_path="$2"
        shift 2
        ;;
      -*)
        redgres_die "unknown flag: $1"
        ;;
      *)
        redgres_die "unknown argument: $1"
        ;;
    esac
  done
  if [[ "${update_non_interactive}" -ne 1 ]]; then
    redgres_die "--non-interactive is required"
  fi
  if [[ -z "${update_release_path}" ]]; then
    redgres_die "--release is required"
  fi
  # Trusted-path/identity check only. Never source, eval, or print contents.
  redgres_require_trusted_unread_file '--release' "${update_release_path}"
  if [[ "${update_dry_run}" -eq 1 ]]; then
    redgres_update_dry_run "${update_release_path}"
    exit 0
  fi
  if [[ "${EUID}" -ne 0 && -z "${REDGRES_OPT_ROOT:-}" ]]; then
    redgres_die 'live update requires root (or REDGRES_OPT_ROOT for tests)'
  fi
  redgres_update_apply "${update_release_path}"
  exit 0
fi

if [[ "${1:-}" == "rollback" ]]; then
  shift
  if [[ "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi
  rollback_dry_run=0
  rollback_non_interactive=0
  rollback_to=''
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --non-interactive)
        rollback_non_interactive=1
        shift
        ;;
      --dry-run)
        rollback_dry_run=1
        shift
        ;;
      --to)
        [[ $# -ge 2 ]] || redgres_die "missing value for --to"
        rollback_to="$2"
        shift 2
        ;;
      -*)
        redgres_die "unknown flag: $1"
        ;;
      *)
        redgres_die "unknown argument: $1"
        ;;
    esac
  done
  if [[ "${rollback_non_interactive}" -ne 1 ]]; then
    redgres_die "--non-interactive is required"
  fi
  if [[ -z "${rollback_to}" ]]; then
    redgres_die "--to is required"
  fi
  # Path-safe token only. Never print VERSION.
  if ! redgres_rollback_version_ok "${rollback_to}"; then
    redgres_die "--to must be a path-safe version token"
  fi
  if [[ "${rollback_dry_run}" -eq 1 ]]; then
    redgres_rollback_dry_run
    exit 0
  fi
  if [[ "${EUID}" -ne 0 && -z "${REDGRES_OPT_ROOT:-}" ]]; then
    redgres_die 'live rollback requires root (or REDGRES_OPT_ROOT for tests)'
  fi
  redgres_rollback_apply "${rollback_to}"
  exit 0
fi

# OPS-007 postgres-extensions apply: fail-closed dry-run skip matrix. Never
# sources --config, never resolves packages/preload/restart, never runs DDL.
# Live apply without --dry-run is not implemented (exit 2).
if [[ "${1:-}" == "postgres-extensions" ]]; then
  shift
  if [[ "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi
  if [[ "${1:-}" == "apply" ]]; then
    shift
  else
    redgres_die "postgres-extensions requires the apply subcommand"
  fi
  ext_dry_run=0
  ext_non_interactive=0
  ext_config_path=''
  ext_extension_path=''
  ext_approve_restart=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --non-interactive)
        ext_non_interactive=1
        shift
        ;;
      --dry-run)
        ext_dry_run=1
        shift
        ;;
      --config)
        [[ $# -ge 2 ]] || redgres_die "missing value for --config"
        ext_config_path="$2"
        shift 2
        ;;
      --extension-plan)
        [[ $# -ge 2 ]] || redgres_die "missing value for --extension-plan"
        ext_extension_path="$2"
        shift 2
        ;;
      --approve-postgres-restart)
        ext_approve_restart=1
        shift
        ;;
      --help)
        usage
        exit 0
        ;;
      -*)
        redgres_die "unknown flag: $1"
        ;;
      *)
        redgres_die "unknown argument: $1"
        ;;
    esac
  done
  if [[ "${ext_non_interactive}" -ne 1 ]]; then
    redgres_die "--non-interactive is required"
  fi
  if [[ -z "${ext_config_path}" ]]; then
    redgres_die "--config is required"
  fi
  if [[ -z "${ext_extension_path}" ]]; then
    redgres_die "--extension-plan is required"
  fi
  redgres_require_trusted_unread_file '--config' "${ext_config_path}"
  redgres_require_trusted_unread_file '--extension-plan' "${ext_extension_path}"
  if [[ "${ext_dry_run}" -ne 1 ]]; then
    redgres_not_implemented "postgres-extensions apply without --dry-run is not implemented"
  fi
  redgres_extensions_apply_dry_run "${ext_config_path}" "${ext_extension_path}"
  exit 0
fi

# OPS-004 backup: fail-closed dry-run skip matrix. Never sources --config,
# never invokes pg_dump/pg_restore/BGSAVE/SQLite backup, never mutates (Partial).
if [[ "${1:-}" == "backup" ]]; then
  shift
  backup_dry_run=0
  backup_non_interactive=0
  backup_config_path=''
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --help)
        usage
        exit 0
        ;;
      --non-interactive)
        backup_non_interactive=1
        shift
        ;;
      --dry-run)
        backup_dry_run=1
        shift
        ;;
      --config)
        [[ $# -ge 2 ]] || redgres_die "missing value for --config"
        backup_config_path="$2"
        shift 2
        ;;
      -*)
        redgres_die "unknown flag: $1"
        ;;
      *)
        redgres_die "unknown argument: $1"
        ;;
    esac
  done
  if [[ "${backup_non_interactive}" -ne 1 ]]; then
    redgres_die "--non-interactive is required"
  fi
  if [[ -z "${backup_config_path}" ]]; then
    redgres_die "--config is required"
  fi
  redgres_require_trusted_unread_file '--config' "${backup_config_path}"
  if [[ "${backup_dry_run}" -ne 1 ]]; then
    redgres_not_implemented "backup without --dry-run is not implemented"
  fi
  redgres_backup_dry_run
  exit 0
fi

# OPS-007 postgres-plan: read-only extension-plan validation. Never sources
# --config, never mutates, never resolves packages/preload/restart (Partial).
if [[ "${1:-}" == "postgres-plan" ]]; then
  shift
  plan_config_path=''
  plan_extension_path=''
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --help)
        usage
        exit 0
        ;;
      --config)
        [[ $# -ge 2 ]] || redgres_die "missing value for --config"
        plan_config_path="$2"
        shift 2
        ;;
      --extension-plan)
        [[ $# -ge 2 ]] || redgres_die "missing value for --extension-plan"
        plan_extension_path="$2"
        shift 2
        ;;
      -*)
        redgres_die "unknown flag: $1"
        ;;
      *)
        redgres_die "unknown argument: $1"
        ;;
    esac
  done
  if [[ -z "${plan_config_path}" ]]; then
    redgres_die "--config is required"
  fi
  if [[ -z "${plan_extension_path}" ]]; then
    redgres_die "--extension-plan is required"
  fi
  redgres_require_trusted_unread_file '--config' "${plan_config_path}"
  redgres_require_trusted_unread_file '--extension-plan' "${plan_extension_path}"
  redgres_postgres_plan_dry_run "${plan_config_path}" "${plan_extension_path}"
  exit 0
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --help)
      usage
      exit 0
      ;;
    --dry-run)
      dry_run=1
      shift
      ;;
    --non-interactive)
      non_interactive=1
      shift
      ;;
    --mode)
      [[ $# -ge 2 ]] || redgres_die "missing value for --mode"
      mode="$2"
      shift 2
      ;;
    --postgres-version)
      [[ $# -ge 2 ]] || redgres_die "missing value for --postgres-version"
      postgres_version="$2"
      shift 2
      ;;
    --expect-postgres-major)
      [[ $# -ge 2 ]] || redgres_die "missing value for --expect-postgres-major"
      expect_postgres_major="$2"
      shift 2
      ;;
    --redis-mode)
      [[ $# -ge 2 ]] || redgres_die "missing value for --redis-mode"
      redis_mode="$2"
      shift 2
      ;;
    --redis-version)
      [[ $# -ge 2 ]] || redgres_die "missing value for --redis-version"
      redis_version="$2"
      shift 2
      ;;
    --expect-redis-series)
      [[ $# -ge 2 ]] || redgres_die "missing value for --expect-redis-series"
      expect_redis_series="$2"
      shift 2
      ;;
    --pgbouncer-mode)
      [[ $# -ge 2 ]] || redgres_die "missing value for --pgbouncer-mode"
      pgbouncer_mode="$2"
      shift 2
      ;;
    --config)
      [[ $# -ge 2 ]] || redgres_die "missing value for --config"
      config_path="$2"
      shift 2
      ;;
    --extension-plan)
      [[ $# -ge 2 ]] || redgres_die "missing value for --extension-plan"
      extension_plan="$2"
      shift 2
      ;;
    --approve-postgres-restart)
      approve_restart=1
      shift
      ;;
    -*)
      redgres_die "unknown flag: $1"
      ;;
    *)
      redgres_die "unknown argument: $1"
      ;;
  esac
done

: "${config_path:=}"
: "${extension_plan:=}"
: "${approve_restart:=0}"

# Interactive selection (S1): without --non-interactive, prompt on a terminal for
# the same choices the flags express. Answers flow through the identical validation
# below (is_pg_major / is_redis_series / mode case statements), so an invalid answer
# is rejected exactly like a bad flag. Only the tested matrix is offered; there is
# no floating "latest".
redgres_ask() {
  local prompt="$1" default="$2" answer line
  line="${prompt}"
  [[ -n "${default}" ]] && line="${prompt} [${default}]"
  read -r -p "  ${line}: " answer </dev/tty || redgres_die "interactive install requires a terminal; pass --non-interactive with explicit flags"
  answer="$(printf "%s" "${answer}" | tr -d "[:space:]")"
  [[ -n "${answer}" ]] || answer="${default}"
  printf "%s" "${answer}"
}

redgres_interactive_selections() {
  if [[ -z "${mode}" ]]; then
    mode="$(redgres_ask "PostgreSQL mode (fresh-postgres|existing-postgres)" "fresh-postgres")"
  fi
  case "${mode}" in
    fresh-postgres)
      [[ -z "${postgres_version}" ]] && postgres_version="$(redgres_ask "PostgreSQL major (17|18)" "18")"
      ;;
    existing-postgres)
      [[ -z "${expect_postgres_major}" ]] && expect_postgres_major="$(redgres_ask "Expected PostgreSQL major (17|18; blank to detect)" "")"
      ;;
  esac
  [[ -z "${pgbouncer_mode}" ]] && pgbouncer_mode="$(redgres_ask "PgBouncer mode (fresh|existing|disabled)" "fresh")"
  [[ -z "${redis_mode}" ]] && redis_mode="$(redgres_ask "Redis mode (fresh|existing)" "fresh")"
  if [[ "${redis_mode}" == "fresh" ]]; then
    [[ -z "${redis_version}" ]] && redis_version="$(redgres_ask "Redis series (8.2|8.8)" "8.2")"
  else
    [[ -z "${expect_redis_series}" ]] && expect_redis_series="$(redgres_ask "Expected Redis series (8.2|8.8; blank to detect)" "")"
  fi
}

if [[ "${non_interactive}" -ne 1 ]]; then
  redgres_interactive_selections
fi

case "${mode}" in
  existing-postgres|fresh-postgres) ;;
  '') redgres_die "--mode is required" ;;
  *) redgres_die "unsupported --mode ${mode}" ;;
esac

case "${redis_mode}" in
  existing|fresh) ;;
  '') redgres_die "--redis-mode is required" ;;
  *) redgres_die "unsupported --redis-mode ${redis_mode}" ;;
esac

case "${pgbouncer_mode}" in
  existing|fresh|disabled) ;;
  '') redgres_die "--pgbouncer-mode is required" ;;
  *) redgres_die "unsupported --pgbouncer-mode ${pgbouncer_mode}" ;;
esac

if [[ "${mode}" == "fresh-postgres" ]]; then
  [[ -n "${postgres_version}" ]] || redgres_die "--postgres-version is required for --mode fresh-postgres"
  is_pg_major "${postgres_version}" || redgres_die "unsupported --postgres-version ${postgres_version}"
  [[ -z "${expect_postgres_major}" ]] || redgres_die "--expect-postgres-major is not valid with --mode fresh-postgres"
else
  [[ -z "${postgres_version}" ]] || redgres_die "--postgres-version is not valid with --mode existing-postgres"
  if [[ -n "${expect_postgres_major}" ]]; then
    is_pg_major "${expect_postgres_major}" || redgres_die "unsupported --expect-postgres-major ${expect_postgres_major}"
  fi
fi

if [[ "${redis_mode}" == "fresh" ]]; then
  [[ -n "${redis_version}" ]] || redgres_die "--redis-version is required for --redis-mode fresh"
  is_redis_series "${redis_version}" || redgres_die "unsupported --redis-version ${redis_version}"
  [[ -z "${expect_redis_series}" ]] || redgres_die "--expect-redis-series is not valid with --redis-mode fresh"
else
  [[ -z "${redis_version}" ]] || redgres_die "--redis-version is not valid with --redis-mode existing"
  if [[ -n "${expect_redis_series}" ]]; then
    is_redis_series "${expect_redis_series}" || redgres_die "unsupported --expect-redis-series ${expect_redis_series}"
  fi
fi

if [[ "${dry_run}" -ne 1 ]]; then
  if [[ -n "${extension_plan}" || -n "${config_path}" ]]; then
    redgres_not_implemented "live install with --config or --extension-plan is not implemented"
  fi
  redgres_live_install
  exit 0
fi

# OPS-007: when an optional extension plan is supplied, validate it exactly
# like postgres-plan (policy, capability registry, explicit non-empty
# identifier-safe databases, pg_cron one control db, one scheduler identity).
# It is never applied during --dry-run; package/preload/restart stay skipped.
if [[ -n "${extension_plan}" ]]; then
  redgres_require_trusted_unread_file '--extension-plan' "${extension_plan}"
  redgres_plan_validate "${extension_plan}"
fi

# OPS-001: --config is optional on the main path. When supplied, parse only the
# documented lifecycle keys as inert data; never source, eval, or export it.
if [[ -n "${config_path}" ]]; then
  redgres_config_validate_lifecycle "${config_path}"
fi

print_stages
redgres_inventory_dry_run
redgres_inventory_cloudflare
redgres_inventory_tls
redgres_log "Selection (redacted): mode=${mode} redis-mode=${redis_mode} pgbouncer-mode=${pgbouncer_mode}"
if [[ -n "${postgres_version}" ]]; then
  redgres_log "postgres-version=${postgres_version}"
fi
if [[ -n "${expect_postgres_major}" ]]; then
  redgres_log "expect-postgres-major=${expect_postgres_major}"
fi
if [[ -n "${redis_version}" ]]; then
  redgres_log "redis-version=${redis_version}"
fi
if [[ -n "${expect_redis_series}" ]]; then
  redgres_log "expect-redis-series=${expect_redis_series}"
fi
exit 0
