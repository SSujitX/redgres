#!/usr/bin/env bash
# Interactive selection logic test (S1). Extracts the real redgres_ask /
# redgres_interactive_selections from deploy/install.sh (no drift), overrides
# redgres_ask with a stdin-reading stub so the mapping is testable without a TTY,
# and verifies the prompt-to-flag mapping against piped answers.
set -euo pipefail

fail() { echo "FAIL: $*" >&2; exit 1; }

extract="$(awk '/^redgres_ask\(\) \{/{f=1} f{print} f && /^if \[\[/{exit}' deploy/install.sh | sed '$d')"
[[ -n "${extract}" ]] || fail "could not extract interactive functions from deploy/install.sh"

check() {
  local desc="$1" answers="$2" expected="$3"
  local mode="" postgres_version="" expect_postgres_major="" pgbouncer_mode="" redis_mode="" redis_version="" expect_redis_series=""
  eval "${extract}"
  redgres_ask() {
    local default="$2" answer
    read -r answer || answer=""
    answer="$(printf "%s" "${answer}" | tr -d "[:space:]")"
    [[ -n "${answer}" ]] || answer="${default}"
    printf "%s" "${answer}"
  }
  redgres_interactive_selections <<< "${answers}"
  local got="mode=${mode} pg=${postgres_version} expect_pg=${expect_postgres_major} pb=${pgbouncer_mode} redis=${redis_mode} rv=${redis_version} expect_redis=${expect_redis_series}"
  if [[ "${got}" != "${expected}" ]]; then
    fail "${desc}: got '${got}', want '${expected}'"
  fi
  echo "ok - ${desc}"
}

check "fresh defaults" $'\n\n\n\n' "mode=fresh-postgres pg=18 expect_pg= pb=fresh redis=fresh rv=8.2 expect_redis="
check "explicit pgbouncer disabled" $'fresh-postgres\n\ndisabled\nfresh\n' "mode=fresh-postgres pg=18 expect_pg= pb=disabled redis=fresh rv=8.2 expect_redis="
check "existing explicit" $'existing-postgres\n17\nexisting\nexisting\n8.8\n' "mode=existing-postgres pg= expect_pg=17 pb=existing redis=existing rv= expect_redis=8.8"

echo "interactive logic passed"