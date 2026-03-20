#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-${ROOT_DIR}/dist}"

TARGETS=(
  "darwin arm64"
  "darwin amd64"
  "linux amd64"
  "linux arm64"
)

mkdir -p "${DIST_DIR}"

for target in "${TARGETS[@]}"; do
  read -r GOOS_VALUE GOARCH_VALUE <<<"${target}"
  echo "Packaging ${GOOS_VALUE}/${GOARCH_VALUE}..."
  (
    cd "${ROOT_DIR}"
    GOOS="${GOOS_VALUE}" GOARCH="${GOARCH_VALUE}" bash scripts/package.sh
  )
done

echo ""
echo "All archives are in ${DIST_DIR}"
