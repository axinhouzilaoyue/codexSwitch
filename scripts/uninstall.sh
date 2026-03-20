#!/usr/bin/env bash
set -euo pipefail

BIN_DIR="${BIN_DIR:-${HOME}/.local/bin}"
TARGET="${BIN_DIR}/ccodex"
LEGACY_CURRENT_TARGET="${BIN_DIR}/cccodex"
LEGACY_TARGET="${BIN_DIR}/codexswitch"
LEGACY_ALIAS_TARGET="${BIN_DIR}/ccswitch"

rm -f "${TARGET}" "${LEGACY_CURRENT_TARGET}" "${LEGACY_TARGET}" "${LEGACY_ALIAS_TARGET}"
echo "Removed ${TARGET}"
echo "Removed ${LEGACY_CURRENT_TARGET}"
echo "Removed ${LEGACY_TARGET}"
echo "Removed ${LEGACY_ALIAS_TARGET}"
