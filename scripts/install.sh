#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${BIN_DIR:-${HOME}/.local/bin}"
TARGET="${BIN_DIR}/ccodex"
LEGACY_CURRENT_TARGET="${BIN_DIR}/cccodex"
LEGACY_TARGET="${BIN_DIR}/codexswitch"
LEGACY_ALIAS_TARGET="${BIN_DIR}/ccswitch"
GOCACHE_DIR="${GOCACHE:-${TMPDIR:-/tmp}/codexswitch-go-build}"
PLATFORM_LABEL="$(go env GOOS)-$(go env GOARCH)"
WSL_NOTE=""

detect_wsl() {
  if [[ -n "${WSL_DISTRO_NAME:-}" ]]; then
    return 0
  fi
  if [[ -r /proc/version ]] && grep -qiE 'microsoft|wsl' /proc/version; then
    return 0
  fi
  return 1
}

print_post_install() {
  echo ""
  echo "ccodex 安装完成"
  echo "  启动命令: ccodex"
  echo "  备用路径: ${TARGET}"
  echo "  安装位置: ${BIN_DIR}"
  echo "  当前平台: ${PLATFORM_LABEL}"
  echo "  账号仓库: ~/.codex-switch"
  echo "  生效目录: 自动探测 Codex 的 CODEX_HOME（通常是 ~/.codex）"
  if [[ -n "${WSL_NOTE}" ]]; then
    echo "  平台说明: ${WSL_NOTE}"
  fi
  echo ""
  echo "常用命令:"
  echo "  ccodex"
  echo "  ccodex current"
  echo "  ccodex doctor"
  echo "  ccodex update"
  echo "  ccodex uninstall"
  echo ""
  echo "首次使用:"
  echo "  1. 确保 codex CLI 已安装并完成登录"
  echo "  2. 运行 ccodex"
  echo "  3. 在界面里按 n 登录新账号，按 s 切换账号"
}

if detect_wsl; then
  PLATFORM_LABEL="${PLATFORM_LABEL} (wsl)"
  WSL_NOTE="WSL 环境会构建 Linux 二进制；请在 WSL 终端内运行 ccodex。"
fi

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

print_post_install
