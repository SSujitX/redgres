#!/usr/bin/env bash
# Read-only postgres-plan extension-plan validator (OPS-007 Partial).
# Validates the non-secret extension plan JSON without mutating anything:
# policy, capability registry, explicit database names, scheduler rules.
# Never sources operator --config. This Partial is not Complete.
set -euo pipefail

# Release-owned capability registry from POSTGRESQL_PROVISIONING.md section 5.
# Capability identifiers come from this frozen list; unknown names fail before
# any package or SQL change. Scheduler values are the pg_partman options.
REDGRES_PLAN_CAPABILITIES='pg_stat_statements pg_repack pg_buffercache vector pg_trgm postgis timescaledb pg_partman pg_cron pgcrypto pgaudit'
REDGRES_PLAN_SCHEDULERS='pg_partman_bgw pg_cron external'
REDGRES_PLAN_PROTECTED_DATABASES='postgres template0 template1 database_console_vault'

redgres_plan_has() {
  local needle="$1"
  shift
  local item
  for item in "$@"; do
    [[ "${item}" == "${needle}" ]] && return 0
  done
  return 1
}

# Strip whitespace outside strings so field extraction uses a canonical form.
redgres_plan_compact() {
  local input="$1" out='' ch i n in_string=0 escaped=0
  n="${#input}"
  for (( i=0; i<n; i++ )); do
    ch="${input:i:1}"
    if [[ "${in_string}" -eq 1 ]]; then
      out+="${ch}"
      if [[ "${escaped}" -eq 1 ]]; then
        escaped=0
      elif [[ "${ch}" == '\' ]]; then
        escaped=1
      elif [[ "${ch}" == '"' ]]; then
        in_string=0
      fi
      continue
    fi
    case "${ch}" in
      '"') in_string=1; out+="${ch}" ;;
      ' '|$'\t'|$'\n'|$'\r') ;;
      *) out+="${ch}" ;;
    esac
  done
  printf '%s' "${out}"
}

# Validates the whole plan file. On success sets:
#   redgres_plan_policy, redgres_plan_preview (selection summary lines)
redgres_plan_validate() {
  local plan_path="$1"
  local raw='' compact='' policy='' sel_content='' rest='' obj='' cap='' dbs='' db='' sched='' expected=''
  local -a selections=() dbs_list=() cap_ids sched_ids protected_ids
  local sched_count=0 pg_cron_count=0
  read -r -a cap_ids <<< "${REDGRES_PLAN_CAPABILITIES}"
  read -r -a sched_ids <<< "${REDGRES_PLAN_SCHEDULERS}"
  read -r -a protected_ids <<< "${REDGRES_PLAN_PROTECTED_DATABASES}"

  # NUL detection before bash variable read (variables cannot hold NUL).
  if [[ "$(wc -c <"${plan_path}")" -ne "$(tr -d '\000' <"${plan_path}" | wc -c)" ]]; then
    redgres_die "extension plan contains NUL"
  fi
  if [[ "$(wc -c <"${plan_path}")" -gt 65536 ]]; then
    redgres_die "extension plan is too large"
  fi
  raw="$(<"${plan_path}")" || redgres_die "extension plan is not readable"
  # A UTF-8 BOM may prefix operator-edited files; strip it defensively.
  if [[ "${raw}" == $'\xEF\xBB\xBF'* ]]; then
    raw="${raw#$'\xEF\xBB\xBF'}"
  fi
  compact="$(redgres_plan_compact "${raw}")"

  if [[ "${compact}" != '{"policy":"'* || "${compact}" != *']}' ]]; then
    redgres_die "extension plan shape is invalid"
  fi

  policy="${compact#*\"policy\":\"}"
  policy="${policy%%\"*}"
  if ! redgres_plan_has "${policy}" preserve apply-selected; then
    redgres_die "policy must be preserve or apply-selected"
  fi

  sel_content="${compact#*\"selections\":[}"
  sel_content="${sel_content%]\}}"

  # Reconstruct top level to reject unknown keys.
  expected="{\"policy\":\"${policy}\",\"selections\":[${sel_content}]}"
  if [[ "${compact}" != "${expected}" ]]; then
    redgres_die "extension plan shape is invalid"
  fi

  if [[ -n "${sel_content}" ]]; then
    # Each selection object has no nested braces, so this is exact. The
    # reconstructed join below rejects stray content between objects.
    while IFS= read -r obj; do
      [[ -n "${obj}" ]] || continue
      selections+=("${obj}")
    done < <(printf '%s' "${sel_content}" | grep -oE '\{[^{}]*\}')
    if [[ "${sel_content}" != "$(printf '%s,' "${selections[@]}" | sed 's/,$//')" ]]; then
      redgres_die "extension plan selections array is invalid"
    fi
  fi

  redgres_plan_preview=''
  for obj in "${selections[@]}"; do
    if [[ "${obj}" != '{"capability":"'* || "${obj}" != *'}' ]]; then
      redgres_die "extension plan selection is invalid"
    fi

    cap="${obj#*\"capability\":\"}"
    cap="${cap%%\"*}"
    if ! redgres_plan_has "${cap}" "${cap_ids[@]}"; then
      redgres_die "unknown capability ${cap}"
    fi

    dbs="${obj#*\"databases\":[}"
    dbs="${dbs%%]*}"
    if [[ -z "${dbs}" ]]; then
      redgres_die "databases must not be empty (never implies all databases)"
    fi

    sched=''
    if [[ "${obj}" == *'"scheduler":"'* ]]; then
      sched="${obj#*\"scheduler\":\"}"
      sched="${sched%%\"*}"
      if ! redgres_plan_has "${sched}" "${sched_ids[@]}"; then
        redgres_die "invalid scheduler ${sched}"
      fi
      if [[ "${cap}" != "pg_partman" ]]; then
        redgres_die "scheduler is only valid for capability pg_partman"
      fi
      sched_count=$((sched_count + 1))
    fi
    if [[ "${cap}" == "pg_cron" ]]; then
      pg_cron_count=$((pg_cron_count + 1))
    fi

    # Exact reconstruction rejects unknown keys inside the selection.
    expected="{\"capability\":\"${cap}\",\"databases\":[${dbs}]"
    if [[ -n "${sched}" ]]; then
      expected+=",\"scheduler\":\"${sched}\""
    fi
    expected+="}"
    if [[ "${obj}" != "${expected}" ]]; then
      redgres_die "extension plan selection is invalid"
    fi

    dbs_list=()
    dbs="${dbs#\"}"
    dbs="${dbs%\"}"
    while [[ -n "${dbs}" ]]; do
      if [[ "${dbs}" == *'","'* ]]; then
        db="${dbs%%'","'*}"
        dbs="${dbs#*'","'}"
      else
        db="${dbs}"
        dbs=''
      fi
      dbs_list+=("${db}")
    done
    for db in "${dbs_list[@]}"; do
      if [[ ! "${db}" =~ ^[A-Za-z_][A-Za-z0-9_]{0,62}$ ]]; then
        redgres_die "invalid database name ${db}"
      fi
      if redgres_plan_has "${db}" "${protected_ids[@]}"; then
        redgres_die "protected database ${db} cannot be selected"
      fi
    done

    redgres_plan_preview+="${cap} databases=[$(printf '%s,' "${dbs_list[@]}" | sed 's/,$//')]"
    if [[ -n "${sched}" ]]; then
      redgres_plan_preview+=" scheduler=${sched}"
    fi
    redgres_plan_preview+=$'\n'
  done

  if [[ "${sched_count}" -gt 1 ]]; then
    redgres_die "extension plan cannot declare two schedulers"
  fi
  if [[ "${pg_cron_count}" -gt 1 ]]; then
    redgres_die "pg_cron can select at most one control database per cluster"
  fi
  redgres_plan_policy="${policy}"
}

# Read-only plan preview + skip matrix (OPS-007 Partial).
redgres_postgres_plan_dry_run() {
  local config_path="$1" plan_path="$2"
  local line
  redgres_plan_validate "${plan_path}"
  redgres_log 'postgres-plan (read-only; not Complete):'
  redgres_log 'config: path-ok (unread, not sourced)'
  redgres_log 'plan: path-ok (validated, not applied)'
  redgres_log "policy: ${redgres_plan_policy}"
  if [[ -n "${redgres_plan_preview}" ]]; then
    while IFS= read -r line; do
      [[ -n "${line}" ]] && redgres_log "selection: ${line}"
    done <<< "${redgres_plan_preview}"
  else
    redgres_log 'selection: (none)'
  fi
  redgres_log 'package_resolution: skipped (no release manifest in this Partial)'
  redgres_log 'inventory: skipped (live cluster state not probed)'
  redgres_log 'backup_verification: skipped (backup evidence not checked)'
  redgres_log 'preload_merge: skipped (shared_preload_libraries not read)'
  redgres_log 'restart_approval: skipped (--approve-postgres-restart is apply-time)'
  redgres_log 'extension_ddl: skipped (CREATE EXTENSION not executed)'
  redgres_log 'verification: skipped (capability smoke checks deferred)'
  redgres_log 'result=partial'
}