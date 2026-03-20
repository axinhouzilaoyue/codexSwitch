#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-${ROOT_DIR}/dist}"
GOCACHE_DIR="${GOCACHE:-${TMPDIR:-/tmp}/codexswitch-go-build}"
GOOS_VALUE="${GOOS:-$(go env GOOS)}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"
BIN_NAME="ccodex"
ARCHIVE_NAME="${BIN_NAME}-${GOOS_VALUE}-${GOARCH_VALUE}.tar.gz"
BUILD_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ccodex-dist-XXXXXX")"

cleanup() {
  rm -rf "${BUILD_DIR}"
}
trap cleanup EXIT

mkdir -p "${DIST_DIR}"
cd "${ROOT_DIR}"

env GOCACHE="${GOCACHE_DIR}" GOOS="${GOOS_VALUE}" GOARCH="${GOARCH_VALUE}" \
  go build -o "${BUILD_DIR}/${BIN_NAME}" ./cmd/codexswitch

chmod +x "${BUILD_DIR}/${BIN_NAME}"
tar -C "${BUILD_DIR}" -czf "${DIST_DIR}/${ARCHIVE_NAME}" "${BIN_NAME}"

if command -v shasum >/dev/null 2>&1; then
  shasum -a 256 "${DIST_DIR}/${ARCHIVE_NAME}" > "${DIST_DIR}/${ARCHIVE_NAME}.sha256"
elif command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${DIST_DIR}/${ARCHIVE_NAME}" > "${DIST_DIR}/${ARCHIVE_NAME}.sha256"
fi

echo "Created ${DIST_DIR}/${ARCHIVE_NAME}"
if [[ -f "${DIST_DIR}/${ARCHIVE_NAME}.sha256" ]]; then
  echo "Created ${DIST_DIR}/${ARCHIVE_NAME}.sha256"
fi
