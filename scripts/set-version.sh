#!/usr/bin/env bash
# Write the root VERSION file and sync dependent package versions.
# Usage:
#   ./scripts/set-version.sh 1.2.3
#   ./scripts/set-version.sh v1.2.3
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

raw="${1:-}"
if [[ -z "$raw" ]]; then
  echo "Usage: $0 X.Y.Z" >&2
  exit 1
fi

VERSION="${raw#v}"
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Invalid version (expected X.Y.Z or vX.Y.Z): '$raw'" >&2
  exit 1
fi

printf '%s\n' "$VERSION" >VERSION
echo "Wrote VERSION=$VERSION"
bash ./scripts/sync-version.sh
