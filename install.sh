#!/usr/bin/env bash
# AssistClaw installer — installs the Go binary, sets up Python venv,
# and creates the ~/.assistclaw config directory.
# No Docker required.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/assistclaw/assistclaw/main/install.sh | bash
#   OR: bash install.sh

set -eo pipefail

# ─────────────────────────────────────────────
# Config
# ─────────────────────────────────────────────
ASSISTCLAW_VERSION="${ASSISTCLAW_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
STATE_DIR="${STATE_DIR:-$HOME/.assistclaw}"
VENV_DIR="$STATE_DIR/venv"
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
  REPO_ROOT="$PWD"
fi

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
    armv7*)  arch="armv7" ;;
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
check_deps() {
  local missing=()
  for cmd in curl git; do
    command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    err "Missing required tools: ${missing[*]}"
  fi
}

# ─────────────────────────────────────────────
# Go binary: download pre-built or fallback to source
# ─────────────────────────────────────────────
install_binary() {
  if [[ "${FORCE_BUILD:-0}" == "1" ]]; then
    log "FORCE_BUILD=1: Building AssistClaw binary from source..."
    build_binary_from_source
    return
  fi

  local artifact="assistclaw-${PLATFORM}"
  if [[ "$PLATFORM" == windows-* ]]; then
    artifact="${artifact}.exe"
  fi

  local download_url="https://github.com/hridesh-net/AssistClaw/releases/latest/download/${artifact}"
  local tmp_bin="/tmp/assistclaw_bin_$$"

  log "Downloading pre-built binary for ${PLATFORM}..."
  
  # Ensure the directory exists and is writable
  if [[ ! -w "$INSTALL_DIR" && ! -w "$(dirname "$INSTALL_DIR")" ]]; then
    warn "Cannot write to $INSTALL_DIR. Changing INSTALL_DIR to $HOME/.local/bin"
    INSTALL_DIR="$HOME/.local/bin"
  fi
  mkdir -p "$INSTALL_DIR"

  if curl -fsSL -f -o "$tmp_bin" "$download_url"; then
    chmod +x "$tmp_bin"
    mv "$tmp_bin" "$INSTALL_DIR/assistclaw"
    ok "Binary installed: $INSTALL_DIR/assistclaw"
  else
    warn "Failed to download pre-built binary from $download_url (HTTP 404/Error)"
    warn "Falling back to compiling from source..."
    build_binary_from_source
  fi
}

build_binary_from_source() {
  if command -v go >/dev/null 2>&1; then
    local go_version
    go_version="$(go version | awk '{print $3}' | tr -d 'go')"
    log "Building with go $go_version..."
    (cd "$REPO_ROOT" && go build -ldflags "-s -w" -o /tmp/assistclaw-build ./cmd/assistclaw)
    
    if [[ ! -w "$INSTALL_DIR" ]]; then
      warn "Cannot write to $INSTALL_DIR (permission denied). Changing INSTALL_DIR to $HOME/.local/bin"
      INSTALL_DIR="$HOME/.local/bin"
    fi
    mkdir -p "$INSTALL_DIR"
    
    install -m 0755 /tmp/assistclaw-build "$INSTALL_DIR/assistclaw"
    rm -f /tmp/assistclaw-build
    ok "Binary compiled and installed: $INSTALL_DIR/assistclaw"
  else
    err "Go compiler not found! Unable to build from source, and pre-built binaries failed to download. Please install Go 1.24+."
  fi
}

# ─────────────────────────────────────────────
# Node.js + pnpm check (for TS layer)
# ─────────────────────────────────────────────
setup_node() {
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
  if [[ ! -d "$REPO_ROOT/sensing" ]]; then
    return
  fi
  if ! command -v cmake >/dev/null 2>&1; then
    warn "cmake not found — C++ sensing layer will not be built"
    return
  fi

  log "Building C++ sensing layer..."
  cmake -S "$REPO_ROOT/sensing" -B "$REPO_ROOT/sensing/build" \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_EXPORT_COMPILE_COMMANDS=ON \
    2>&1 | tail -5

  cmake --build "$REPO_ROOT/sensing/build" --parallel "$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2)" \
    2>&1 | tail -5

  ok "C++ sensing layer built"
}

# ─────────────────────────────────────────────
# Config directory scaffold
# ─────────────────────────────────────────────
setup_config() {
  log "Setting up ~/.assistclaw config directory..."
  mkdir -p "$STATE_DIR"/{memory,tools,skills,logs}

  local config_path="$STATE_DIR/assistclaw.yaml"
  if [[ ! -f "$config_path" ]]; then
    if [[ -f "$REPO_ROOT/.assistclaw.yaml.example" ]]; then
      cp "$REPO_ROOT/.assistclaw.yaml.example" "$config_path"
    else
      cat > "$config_path" <<'YAML'
# AssistClaw Configuration
# Docs: https://github.com/assistclaw/assistclaw#configuration
version: 1

# LLM Providers — configure at least one
providers:
  # OpenAI
  # openai:
  #   api_key: ${ASSISTCLAW_OPENAI_API_KEY}
  #   default_model: gpt-4o-mini

  # Anthropic
  # anthropic:
  #   api_key: ${ASSISTCLAW_ANTHROPIC_API_KEY}
  #   default_model: claude-haiku-3-5

  # Ollama (local — no key needed)
  ollama:
    base_url: http://localhost:11434
    default_model: llama3.2

  # vLLM (local OpenAI-compatible)
  # vllm:
  #   base_url: http://localhost:8000

  # LM Studio (local)
  # lm_studio:
  #   base_url: http://localhost:1234

  # Groq
  # groq:
  #   api_key: ${ASSISTCLAW_GROQ_API_KEY}

  # Mistral
  # mistral:
  #   api_key: ${ASSISTCLAW_MISTRAL_API_KEY}

  # OpenRouter (access 200+ models)
  # openrouter:
  #   api_key: ${ASSISTCLAW_OPENROUTER_API_KEY}

  # AWS Bedrock
  # bedrock:
  #   region: us-east-1
  #   profile: default

# Embedding Models
embeddings:
  priority: [openai, ollama, cohere, google, huggingface]
  # openai:
  #   api_key: ${ASSISTCLAW_OPENAI_API_KEY}
  ollama:
    base_url: http://localhost:11434

# Model routing — assign different models to task types
routing:
  default: ollama/llama3.2
  fallback: ollama/llama3.2
  rules:
    - task: code
      model: ollama/codellama
    - task: summarize
      model: ollama/llama3.2

# Memory (all local by default)
memory:
  working_token_budget: 100000

# Agent settings
agent:
  max_iterations: 64
  system_prompt_ext: ""

# Gateway (REST/WebSocket)
gateway:
  host: 127.0.0.1
  port: 18790
YAML
    fi
    ok "Config created: $config_path"
  else
    ok "Config already exists: $config_path (not overwritten)"
  fi
}

# ─────────────────────────────────────────────
# Verify installation
# ─────────────────────────────────────────────
verify() {
  if "$INSTALL_DIR/assistclaw" version >/dev/null 2>&1; then
    ok "Installation verified"
  else
    warn "Binary installed but 'assistclaw version' failed — check PATH"
  fi
}

# ─────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────
main() {
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
  echo -e "    ${BOLD}assistclaw providers list${NC}"
  echo -e "    ${BOLD}assistclaw agent --message \"Hello!\"${NC}"
  echo -e "    ${BOLD}assistclaw agent${NC}   (interactive REPL)"
  echo ""
  echo -e "  Config: ${BOLD}$STATE_DIR/assistclaw.yaml${NC}"
  echo ""
}

main "$@"
