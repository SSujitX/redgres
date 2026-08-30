#!/usr/bin/env bash
# Sync package versions from the root VERSION file.
# Usage:
#   ./scripts/sync-version.sh          # write versions into package files
#   ./scripts/sync-version.sh check    # exit 1 if any file drifts from VERSION
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="$(tr -d '[:space:]' < VERSION)"
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Invalid VERSION (expected X.Y.Z): '$VERSION'" >&2
  exit 1
fi

read_web_package_version() {
  node -p "require('./web/package.json').version"
}

read_web_lock_version() {
  node -p "require('./web/package-lock.json').version"
}

mode="${1:-sync}"

if [[ "$mode" == "check" ]]; then
  pkg_version="$(read_web_package_version)"
  ok=1
  if [[ "$pkg_version" != "$VERSION" ]]; then
    echo "web/package.json is $pkg_version, expected $VERSION" >&2
    ok=0
  fi
  if [[ -f web/package-lock.json ]]; then
    lock_version="$(read_web_lock_version)"
    if [[ "$lock_version" != "$VERSION" ]]; then
      echo "web/package-lock.json is $lock_version, expected $VERSION" >&2
      ok=0
    fi
  fi
  if [[ "$ok" -ne 1 ]]; then
    exit 1
  fi
  echo "OK: package files match VERSION $VERSION"
  exit 0
fi

if [[ "$mode" != "sync" ]]; then
  echo "Usage: $0 [sync|check]" >&2
  exit 1
fi

echo "Syncing product version $VERSION"

# web/package.json + web/package-lock.json root version fields
(
  cd web
  npm version "$VERSION" --no-git-tag-version --allow-same-version
)

echo "Done: package files now at $VERSION"
bash ./scripts/sync-version.sh check
