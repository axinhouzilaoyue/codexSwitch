#!/usr/bin/env bash
set -euo pipefail

BIN_NAME="ccodex"
BIN_DIR="${BIN_DIR:-${HOME}/.local/bin}"
TARGET="${BIN_DIR}/${BIN_NAME}"
LEGACY_CURRENT_TARGET="${BIN_DIR}/cccodex"
LEGACY_TARGET="${BIN_DIR}/codexswitch"
LEGACY_ALIAS_TARGET="${BIN_DIR}/ccswitch"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ccodex-install-XXXXXX")"
ARCHIVE_URL="${CCODEX_ARCHIVE_URL:-}"
BASE_URL=""
DEFAULT_GITHUB_REPO="axinhouzilaoyue/codexSwitch"
GITHUB_REPO="${CCODEX_GITHUB_REPO:-${DEFAULT_GITHUB_REPO}}"
PLATFORM_LABEL=""
WSL_NOTE=""

cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

step() {
  echo "[ccodex] $1"
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

usage() {
  cat <<'EOF'
Usage:
  install-release.sh
  install-release.sh --repo owner/repo
  install-release.sh --url https://host/path/ccodex-darwin-arm64.tar.gz
  install-release.sh --base-url https://host/path/to/releases

Options:
  --repo      GitHub repository override. Default is the official ccodex repo
  --url       Full archive URL
  --base-url  Base URL; installer appends ccodex-<os>-<arch>.tar.gz
EOF
}

detect_wsl() {
  if [[ -n "${WSL_DISTRO_NAME:-}" ]]; then
    return 0
  fi
  if [[ -r /proc/version ]] && grep -qiE 'microsoft|wsl' /proc/version; then
    return 0
  fi
  return 1
}

install_binary() {
  local source_path="$1"
  local target_path="$2"
  if command -v install >/dev/null 2>&1; then
    install -m 0755 "${source_path}" "${target_path}"
    return
  fi
  cp "${source_path}" "${target_path}"
  chmod 0755 "${target_path}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --url)
      ARCHIVE_URL="${2:-}"
      shift 2
      ;;
    --base-url)
      BASE_URL="${2:-}"
      shift 2
      ;;
    --repo)
      GITHUB_REPO="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

case "$(uname -s)" in
  Darwin) OS_NAME="darwin" ;;
  Linux) OS_NAME="linux" ;;
  *)
    echo "Unsupported OS: $(uname -s). Use macOS, Linux, or a WSL terminal on Windows." >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  arm64|aarch64) ARCH_NAME="arm64" ;;
  x86_64|amd64) ARCH_NAME="amd64" ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

PLATFORM_LABEL="${OS_NAME}-${ARCH_NAME}"
if detect_wsl; then
  WSL_NOTE="WSL 环境会使用 Linux 发行包；请在 WSL 终端内运行安装命令。"
  PLATFORM_LABEL="${PLATFORM_LABEL} (wsl)"
fi

step "检测到平台 ${PLATFORM_LABEL}"

if [[ -z "${ARCHIVE_URL}" && -n "${BASE_URL}" ]]; then
  ARCHIVE_URL="${BASE_URL%/}/${BIN_NAME}-${OS_NAME}-${ARCH_NAME}.tar.gz"
fi

if [[ -z "${ARCHIVE_URL}" && -n "${GITHUB_REPO}" ]]; then
  ARCHIVE_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/${BIN_NAME}-${OS_NAME}-${ARCH_NAME}.tar.gz"
fi

if [[ -z "${ARCHIVE_URL}" ]]; then
  echo "Missing archive URL. Use the default repo, or override with --repo, --url, or --base-url." >&2
  usage >&2
  exit 1
fi

ARCHIVE_PATH="${TMP_DIR}/${BIN_NAME}.tar.gz"

step "下载发布包"
echo "  ${ARCHIVE_URL}"

if command -v curl >/dev/null 2>&1; then
  if [[ -t 1 ]]; then
    curl -fL --progress-bar "${ARCHIVE_URL}" -o "${ARCHIVE_PATH}"
  else
    curl -fsSL "${ARCHIVE_URL}" -o "${ARCHIVE_PATH}"
  fi
elif command -v wget >/dev/null 2>&1; then
  if [[ -t 1 ]]; then
    wget --show-progress -qO "${ARCHIVE_PATH}" "${ARCHIVE_URL}"
  else
    wget -qO "${ARCHIVE_PATH}" "${ARCHIVE_URL}"
  fi
else
  echo "curl or wget is required" >&2
  exit 1
fi

step "解压发布包"
tar -xzf "${ARCHIVE_PATH}" -C "${TMP_DIR}"

if [[ ! -f "${TMP_DIR}/${BIN_NAME}" ]]; then
  echo "Archive does not contain ${BIN_NAME}" >&2
  exit 1
fi

step "安装二进制到 ${TARGET}"
mkdir -p "${BIN_DIR}"
install_binary "${TMP_DIR}/${BIN_NAME}" "${TARGET}"
step "清理旧命令"
rm -f "${LEGACY_CURRENT_TARGET}" "${LEGACY_TARGET}" "${LEGACY_ALIAS_TARGET}"

echo "Installed ${BIN_NAME} to ${TARGET}"
echo "Removed legacy commands ${LEGACY_CURRENT_TARGET}, ${LEGACY_TARGET}, and ${LEGACY_ALIAS_TARGET}"

if ! command -v codex >/dev/null 2>&1; then
  echo ""
  echo "Warning: codex CLI is not installed."
  echo "Install it first:"
  echo "  npm install -g @openai/codex"
  echo "or"
  echo "  brew install --cask codex"
fi

case ":${PATH}:" in
  *":${BIN_DIR}:"*) ;;
  *)
    echo ""
    echo "Add ${BIN_DIR} to PATH if needed:"
    echo "  export PATH=\"${BIN_DIR}:\$PATH\""
    ;;
esac

print_post_install
