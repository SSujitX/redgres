#!/usr/bin/env bash
# Embed scripts/install-summary.sh into install.sh, upgrade.sh, install-dev.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="${ROOT}/scripts/install-summary.sh"
BEGIN='# REDGRES_INSTALL_SUMMARY_BEGIN'
END='# REDGRES_INSTALL_SUMMARY_END'

[[ -f "${SRC}" ]] || { echo "missing ${SRC}" >&2; exit 1; }

embed_block() {
  printf '%s\n' "${BEGIN}"
  cat "${SRC}"
  printf '%s\n' "${END}"
}

sync_file() {
  local target="$1" block tmp
  block="$(mktemp)"
  tmp="$(mktemp)"
  embed_block >"${block}"
  awk -v b="${BEGIN}" -v e="${END}" -v bf="${block}" '
    $0 == b {
      while ((getline line < bf) > 0) print line
      close(bf)
      skip = 1
      next
    }
    skip {
      if ($0 == e) skip = 0
      next
    }
    { print }
  ' "${target}" >"${tmp}"
  mv "${tmp}" "${target}"
  rm -f "${block}"
  echo "synced ${target#${ROOT}/}"
}

for name in install.sh upgrade.sh install-dev.sh; do
  target="${ROOT}/${name}"
  [[ -f "${target}" ]] || { echo "missing ${target}" >&2; exit 1; }
  if ! grep -q "^${BEGIN}\$" "${target}"; then
    echo "${target}: missing ${BEGIN} marker" >&2
    exit 1
  fi
  sync_file "${target}"
done
