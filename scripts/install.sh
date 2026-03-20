#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${BIN_DIR:-${HOME}/.local/bin}"
TARGET="${BIN_DIR}/ccodex"
LEGACY_CURRENT_TARGET="${BIN_DIR}/cccodex"
LEGACY_TARGET="${BIN_DIR}/codexswitch"
LEGACY_ALIAS_TARGET="${BIN_DIR}/ccswitch"
GOCACHE_DIR="${GOCACHE:-${TMPDIR:-/tmp}/codexswitch-go-build}"

mkdir -p "${BIN_DIR}"
cd "${ROOT_DIR}"
env GOCACHE="${GOCACHE_DIR}" go build -o "${TARGET}" ./cmd/codexswitch
chmod +x "${TARGET}"
rm -f "${LEGACY_CURRENT_TARGET}" "${LEGACY_TARGET}" "${LEGACY_ALIAS_TARGET}"

echo "Installed ccodex to ${TARGET}"
echo "Removed legacy commands ${LEGACY_CURRENT_TARGET}, ${LEGACY_TARGET}, and ${LEGACY_ALIAS_TARGET}"
case ":${PATH}:" in
  *":${BIN_DIR}:"*) ;;
  *)
    echo ""
    echo "Add ${BIN_DIR} to PATH if needed:"
    echo "  export PATH=\"${BIN_DIR}:\$PATH\""
    ;;
esac
