#!/usr/bin/env bash
# AssistClaw installer — installs the Go binary, optional Python venv,
# and creates the ~/.assistclaw config directory. No Docker required.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.sh | bash
#   bash install.sh
#   ASSISTCLAW_VERSION=v3.9.7 bash install.sh
#
# Environment:
#   ASSISTCLAW_VERSION   Git tag or "latest" (default: latest)
#   INSTALL_DIR          Binary destination (default: /usr/local/bin or ~/.local/bin if not writable)
#   STATE_DIR            Config/state root (default: ~/.assistclaw)
#   FORCE_BUILD=1        Build from source instead of downloading release
#   SKIP_VENV=1          Skip Python venv creation
#   SKIP_SENSING=1       Skip optional C++ sensing build

set -eo pipefail

# ─────────────────────────────────────────────
# Config
# ─────────────────────────────────────────────
ASSISTCLAW_VERSION="${ASSISTCLAW_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-}"
STATE_DIR="${STATE_DIR:-$HOME/.assistclaw}"
VENV_DIR="$STATE_DIR/venv"
REPO_OWNER_REPO="${ASSISTCLAW_REPO:-hridesh-net/AssistClaw}"
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
  REPO_ROOT="$PWD"
fi

# Prefer XDG-style user bin when /usr/local is not writable
default_install_dir() {
  if [[ -n "${INSTALL_DIR:-}" ]]; then
    echo "$INSTALL_DIR"
    return
  fi
  local d="/usr/local/bin"
  if [[ -w "$d" ]] || [[ -w "$(dirname "$d")" ]]; then
    echo "$d"
  else
    echo "${XDG_BIN_HOME:-$HOME/.local/bin}"
  fi
}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

log()  { echo -e "${BLUE}[assistclaw]${NC} $*"; }
ok()   { echo -e "${GREEN}[✓]${NC} $*"; }
warn() { echo -e "${YELLOW}[!]${NC} $*"; }
err()  { echo -e "${RED}[✗]${NC} $*" >&2; exit 1; }

# ─────────────────────────────────────────────
# OS / arch detection
# ─────────────────────────────────────────────
detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in
    x86_64)  arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    armv7*)  err "No pre-built ARMv7 release; install on arm64/amd64 or set FORCE_BUILD=1 with Go 1.24+." ;;
    *)       err "Unsupported architecture: $arch" ;;
  esac
  case "$os" in
    linux|darwin) ;;
    msys*|cygwin*|mingw*) os="windows" ;;
    *) err "Unsupported OS: $os" ;;
  esac
  echo "${os}-${arch}"
}

PLATFORM="$(detect_platform)"
log "Detected platform: $PLATFORM"

# ─────────────────────────────────────────────
# Dependency checks
# ─────────────────────────────────────────────
need_git() {
  [[ "${FORCE_BUILD:-0}" == "1" ]] || [[ -f "$REPO_ROOT/cmd/assistclaw/main.go" ]]
}

check_deps() {
  command -v curl >/dev/null 2>&1 || err "curl is required. Install curl and retry."
  if need_git; then
    command -v git >/dev/null 2>&1 || err "git is required for this install mode (source/build). Install git or use default release download."
  fi
}

curl_get() {
  local out="$1"
  local url="$2"
  curl -fsSL \
    --connect-timeout 25 \
    --retry 3 \
    --retry-delay 2 \
    --retry-all-errors \
    -o "$out" "$url"
}

# ─────────────────────────────────────────────
# Go binary: download pre-built or fallback to source
# ─────────────────────────────────────────────
install_binary() {
  INSTALL_DIR="$(default_install_dir)"
  export INSTALL_DIR

  if [[ "${FORCE_BUILD:-0}" == "1" ]]; then
    log "FORCE_BUILD=1: Building AssistClaw binary from source..."
    build_binary_from_source
    return
  fi

  local artifact="assistclaw-${PLATFORM}"
  if [[ "$PLATFORM" == windows-* ]]; then
    artifact="${artifact}.exe"
  fi

  local release_path="download/${ASSISTCLAW_VERSION}"
  if [[ "${ASSISTCLAW_VERSION}" == "latest" ]]; then
    release_path="latest/download"
  fi
  local download_url="https://github.com/${REPO_OWNER_REPO}/releases/${release_path}/${artifact}"
  local tmp_bin
  tmp_bin="$(mktemp "${TMPDIR:-/tmp}/assistclaw.XXXXXX")"

  log "Downloading pre-built binary for ${PLATFORM}..."
  mkdir -p "$INSTALL_DIR"

  if curl_get "$tmp_bin" "$download_url"; then
    chmod +x "$tmp_bin"
    mv -f "$tmp_bin" "$INSTALL_DIR/assistclaw"
    ok "Binary installed: $INSTALL_DIR/assistclaw"
    copy_skills_dir
  else
    rm -f "$tmp_bin"
    warn "Failed to download pre-built binary from $download_url"
    warn "Falling back to compiling from source..."
    build_binary_from_source
  fi
}

build_binary_from_source() {
  command -v go >/dev/null 2>&1 || err "Go 1.24+ not found. Install Go or use a release download (unset FORCE_BUILD)."
  local go_version
  go_version="$(go version | awk '{print $3}' | tr -d 'go')"
  log "Building with go $go_version..."
  INSTALL_DIR="$(default_install_dir)"
  export INSTALL_DIR
  mkdir -p "$INSTALL_DIR"

  local ver_ldflags=""
  if [[ "${ASSISTCLAW_VERSION}" != "latest" && -n "${ASSISTCLAW_VERSION}" ]]; then
    ver_ldflags="-X main.version=${ASSISTCLAW_VERSION}"
  fi
  local tmp_build
  tmp_build="$(mktemp "${TMPDIR:-/tmp}/assistclaw-build.XXXXXX")"
  (cd "$REPO_ROOT" && go build -mod=vendor -tags fts5 -ldflags "-s -w ${ver_ldflags}" -o "$tmp_build" ./cmd/assistclaw)
  install -m 0755 "$tmp_build" "$INSTALL_DIR/assistclaw"
  rm -f "$tmp_build"
  ok "Binary compiled and installed: $INSTALL_DIR/assistclaw"
  copy_skills_dir
}

# ─────────────────────────────────────────────
# Copy bundled skills next to binary (local repo only)
# ─────────────────────────────────────────────
copy_skills_dir() {
  local skills_src="$REPO_ROOT/skills"
  local skills_dest="$INSTALL_DIR/skills"

  if [[ ! -d "$skills_src" ]]; then
    warn "No local skills/ directory (normal for curl|bash install). Install skills with: assistclaw skills marketplace"
    return
  fi

  log "Copying bundled skills to $skills_dest..."
  rm -rf "$skills_dest"
  cp -R "$skills_src" "$skills_dest"
  ok "Bundled skills installed: $skills_dest"
}

# ─────────────────────────────────────────────
# Node.js + pnpm (optional TS layer)
# ─────────────────────────────────────────────
setup_node() {
  if [[ ! -f "$REPO_ROOT/package.json" ]]; then
    return
  fi

  if command -v node >/dev/null 2>&1; then
    local node_ver
    node_ver="$(node --version | tr -d 'v')"
    local node_major
    IFS='.' read -r node_major _ <<< "$node_ver"
    if [[ "$node_major" -ge 22 ]]; then
      ok "Node.js $node_ver found"
      if ! command -v pnpm >/dev/null 2>&1; then
        log "Installing pnpm..."
        npm install -g pnpm@10
      fi
      log "Installing Node dependencies..."
      (cd "$REPO_ROOT" && pnpm install --frozen-lockfile 2>/dev/null || pnpm install)
      log "Building TypeScript layer..."
      (cd "$REPO_ROOT" && pnpm build) && ok "TypeScript layer built"
    else
      warn "Node.js $node_ver < 22; TypeScript layer will not be built"
    fi
  else
    warn "Node.js not found; TypeScript layer will not be built (Go binary is fully functional)"
  fi
}

# ─────────────────────────────────────────────
# Python venv for tool sandbox
# ─────────────────────────────────────────────
setup_venv() {
  [[ "${SKIP_VENV:-0}" != "1" ]] || return 0

  if [[ -x "$VENV_DIR/bin/python" ]]; then
    ok "Python venv already exists: $VENV_DIR (skipping recreate)"
    "$VENV_DIR/bin/pip" install --quiet --upgrade pip 2>/dev/null || true
    return 0
  fi

  local python_bin=""
  for candidate in python3.12 python3.11 python3.10 python3; do
    if command -v "$candidate" >/dev/null 2>&1; then
      python_bin="$candidate"
      break
    fi
  done

  if [[ -z "$python_bin" ]]; then
    warn "Python 3 not found — auto-tool sandbox will use system Python if available"
    return
  fi

  local py_ver
  py_ver="$("$python_bin" --version 2>&1 | awk '{print $2}')"
  log "Creating Python venv at $VENV_DIR (Python $py_ver)..."
  "$python_bin" -m venv "$VENV_DIR"
  "$VENV_DIR/bin/pip" install --quiet --upgrade pip
  ok "Python venv ready: $VENV_DIR"
}

# ─────────────────────────────────────────────
# C++ sensing (optional)
# ─────────────────────────────────────────────
build_sensing() {
  [[ "${SKIP_SENSING:-0}" != "1" ]] || return 0
  if [[ ! -d "$REPO_ROOT/sensing" ]]; then
    return
  fi
  if ! command -v cmake >/dev/null 2>&1; then
    warn "cmake not found — C++ sensing layer will not be built"
    return
  fi

  local jobs
  jobs="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2)"

  log "Building C++ sensing layer (${jobs} parallel jobs)..."
  cmake -S "$REPO_ROOT/sensing" -B "$REPO_ROOT/sensing/build" \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_EXPORT_COMPILE_COMMANDS=ON >/dev/null
  cmake --build "$REPO_ROOT/sensing/build" --parallel "$jobs" >/dev/null
  ok "C++ sensing layer built"
}

# ─────────────────────────────────────────────
# Config directory scaffold
# ─────────────────────────────────────────────
setup_config() {
  log "Setting up config directory: $STATE_DIR"
  mkdir -p "$STATE_DIR"/{memory,tools,logs,security}
  mkdir -p "$STATE_DIR"/skills/{bundled,custom}
}

# ─────────────────────────────────────────────
# PATH hint
# ─────────────────────────────────────────────
path_hint() {
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) return 0 ;;
  esac
  echo ""
  warn "$INSTALL_DIR is not on your PATH."
  case "${SHELL:-}" in
    */zsh)
      echo -e "  Add to ${BOLD}~/.zshrc${NC}:"
      echo -e "    ${BOLD}export PATH=\"$INSTALL_DIR:\$PATH\"${NC}"
      ;;
    */bash|*)
      echo -e "  Add to ${BOLD}~/.bashrc${NC} or ${BOLD}~/.profile${NC}:"
      echo -e "    ${BOLD}export PATH=\"$INSTALL_DIR:\$PATH\"${NC}"
      ;;
  esac
}

# ─────────────────────────────────────────────
# Verify installation
# ─────────────────────────────────────────────
verify() {
  local bin="$INSTALL_DIR/assistclaw"
  if [[ ! -x "$bin" ]]; then
    warn "Expected binary missing or not executable: $bin"
    return
  fi
  if ver_out="$("$bin" version 2>&1)"; then
    ok "Installation verified: $ver_out"
  else
    warn "Binary present but 'assistclaw version' failed — try running: $bin version"
  fi

  if command -v assistclaw >/dev/null 2>&1; then
    local resolved_bin
    resolved_bin="$(command -v assistclaw)"
    if [[ "$resolved_bin" != "$bin" ]]; then
      echo ""
      warn "Another assistclaw on PATH shadows this install: $resolved_bin"
      warn "Remove the old binary or reorder PATH so $INSTALL_DIR comes first."
      echo ""
    fi
  else
    path_hint
  fi
}

# ─────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────
usage() {
  sed -n '2,22p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
  exit 0
}

main() {
  for arg in "$@"; do
    case "$arg" in
      -h|--help) usage ;;
    esac
  done

  echo ""
  echo -e "${BOLD}  ╔═══════════════════════════════╗${NC}"
  echo -e "${BOLD}  ║     AssistClaw Installer       ║${NC}"
  echo -e "${BOLD}  ╚═══════════════════════════════╝${NC}"
  echo ""

  check_deps
  install_binary
  setup_node
  setup_venv
  build_sensing
  setup_config
  verify

  echo ""
  echo -e "${GREEN}${BOLD}AssistClaw installed successfully!${NC}"
  echo ""
  echo -e "  Get started:"
  echo -e "    ${BOLD}assistclaw --help${NC}"
  echo -e "    ${BOLD}assistclaw onboard${NC}               (first-time setup)"
  echo -e "    ${BOLD}assistclaw skills marketplace${NC}    (browse available skills)"
  echo -e "    ${BOLD}assistclaw skills add github${NC}     (install a skill)"
  echo -e "    ${BOLD}assistclaw agent${NC}                 (interactive REPL)"
  echo ""
  echo -e "  Config: ${BOLD}$STATE_DIR/assistclaw.yaml${NC}"
  echo -e "  Binary: ${BOLD}$INSTALL_DIR/assistclaw${NC}"
  echo ""
}

main "$@"
