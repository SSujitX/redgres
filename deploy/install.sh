#!/usr/bin/env bash
# Fail-closed installer dispatcher. OPS-001 / OPS-002 / OPS-003 / OPS-005 / OPS-006 Partial: no host mutation.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${script_dir}/lib/common.sh"
# shellcheck disable=SC1091
source "${script_dir}/lib/inventory.sh"
# shellcheck disable=SC1091
source "${script_dir}/lib/verify.sh"
# shellcheck disable=SC1091
source "${script_dir}/lib/release.sh"

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
  deploy/install.sh update --non-interactive --dry-run --release PATH
  deploy/install.sh rollback --non-interactive --dry-run --to VERSION

This Partial prints the planned stage list on --dry-run and inventories PATH
host --version for existing PostgreSQL/Redis/PgBouncer. verify --dry-run prints
a skip matrix (result=partial); it does not probe DNS/Cloudflare/public TLS.
update/rollback --dry-run print skip matrices (result=partial); they do not
extract, switch current, migrate SQLite, write systemd, or probe healthz.
It does not install packages, pull images, write systemd, open a firewall, or
change DNS/Cloudflare. It does not start servers, source --config, call curl,
or run SQL SHOW / Redis INFO.

Exit 0: --help, valid --non-interactive --dry-run plan, or skip matrix
Exit 1: unsupported, incomplete, missing, unparseable, or mismatched selection
Exit 2: mutation install, live verify/update/rollback, or other subcommand not implemented

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
8. TLS/DNS
9. Application release
10. Cloudflare
11. Firewall
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
  # Existence only. Never source, eval, or cat the file; never print contents.
  if [[ ! -f "${verify_config_path}" ]]; then
    redgres_die "--config must be an existing regular file"
  fi
  if [[ "${verify_dry_run}" -ne 1 ]]; then
    redgres_not_implemented "verify without --dry-run is not implemented"
  fi
  redgres_verify_dry_run
  exit 0
fi

if [[ "${1:-}" == "update" ]]; then
  shift
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
  # Existence only. Never source, eval, cat, extract, or print contents.
  if [[ ! -f "${update_release_path}" ]]; then
    redgres_die "--release must be an existing regular file"
  fi
  if [[ "${update_dry_run}" -ne 1 ]]; then
    redgres_not_implemented "update without --dry-run is not implemented"
  fi
  redgres_update_dry_run
  exit 0
fi

if [[ "${1:-}" == "rollback" ]]; then
  shift
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
  if [[ "${rollback_dry_run}" -ne 1 ]]; then
    redgres_not_implemented "rollback without --dry-run is not implemented"
  fi
  redgres_rollback_dry_run
  exit 0
fi

if [[ "${1:-}" == "backup" || "${1:-}" == "postgres-plan" || "${1:-}" == "postgres-extensions" ]]; then
  redgres_not_implemented "subcommand ${1} is not implemented"
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

if [[ "${non_interactive}" -ne 1 ]]; then
  redgres_die "--non-interactive is required"
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
  redgres_not_implemented "install mutation is not implemented"
fi

print_stages
redgres_inventory_dry_run
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
