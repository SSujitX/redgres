#!/usr/bin/env bash
# Build a Redgres application release tarball + adjacent SHA256SUMS (OPS-005).
# Usage (from repository root, Linux/amd64 CI or cross-compile):
#   ./deploy/build-release.sh [VERSION]
# When VERSION is omitted, reads the root VERSION file (required for releases).
# Emits dist/release/redgres_${VERSION}_linux_amd64.tar.gz and SHA256SUMS.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

VERSION="${1:-}"
if [[ -z "${VERSION}" ]]; then
  if [[ -f VERSION ]]; then
    VERSION="$(tr -d '[:space:]' <VERSION)"
  elif git describe --tags --exact-match >/dev/null 2>&1; then
    VERSION="$(git describe --tags --exact-match)"
  else
    VERSION="0.0.0-dev.$(git rev-parse --short HEAD)"
  fi
fi
# Strip leading v for VERSION file contents but keep tag-friendly names.
VERSION_FILE="${VERSION#v}"
if [[ ! "${VERSION_FILE}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$ ]]; then
  echo "Invalid VERSION: '${VERSION_FILE}'" >&2
  exit 1
fi

ASSET="redgres_${VERSION_FILE}_linux_amd64.tar.gz"
OUT_DIR="${ROOT}/dist/release"
STAGE="${OUT_DIR}/stage"
LDFLAGS="-s -w -X github.com/SSujitX/redgres/internal/buildinfo.Version=${VERSION_FILE}"

rm -rf "${STAGE}"
mkdir -p "${STAGE}" "${OUT_DIR}"

echo "Building web assets..."
(cd web && npm ci && npm run build)

echo "Building linux/amd64 binary (version ${VERSION_FILE})..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o "${STAGE}/redgres" ./cmd/redgres
printf '%s\n' "${VERSION_FILE}" >"${STAGE}/VERSION"
chmod 0755 "${STAGE}/redgres"

# Ship the transaction that installs this binary in the same checksummed
# archive. upgrade.sh must not maintain a second, weaker update/rollback path.
mkdir -p "${STAGE}/installer"
cp -a deploy/install.sh deploy/lib deploy/systemd "${STAGE}/installer/"
chmod 0755 "${STAGE}/installer/install.sh"
find "${STAGE}/installer/lib" -type f -name '*.sh' -exec chmod 0644 {} +
find "${STAGE}/installer/systemd" -type f -exec chmod 0644 {} +
find "${STAGE}/installer/systemd" -type f -name '*.sh' -exec chmod 0755 {} +

tar -C "${STAGE}" -czf "${OUT_DIR}/${ASSET}" redgres VERSION installer
(
  cd "${OUT_DIR}"
  sha256sum "${ASSET}" | awk '{print tolower($1) "  " $2}' >SHA256SUMS
)

echo "Wrote ${OUT_DIR}/${ASSET}"
echo "Wrote ${OUT_DIR}/SHA256SUMS"
cat "${OUT_DIR}/SHA256SUMS"
