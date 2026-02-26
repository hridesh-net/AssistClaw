#!/usr/bin/env bash
# AssistClaw uninstaller
# Removes the binary, bundled skills, system service, and optionally all user data.
#
# Usage:
#   bash uninstall.sh              # removes binary & service, keeps ~/.assistclaw data
#   bash uninstall.sh --purge      # removes everything including config & memory

set -eo pipefail

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
STATE_DIR="${STATE_DIR:-$HOME/.assistclaw}"
PURGE=false

# ─────────────────────────────────────────────
# Colors
# ─────────────────────────────────────────────
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

for arg in "$@"; do
  case "$arg" in
    --purge) PURGE=true ;;
    --help|-h)
      echo ""
      echo "Usage: bash uninstall.sh [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --purge   Also remove ~/.assistclaw (config, memory, skills, logs)"
      echo "  --help    Show this help"
      echo ""
      exit 0
      ;;
  esac
done

# ─────────────────────────────────────────────
# Header
# ─────────────────────────────────────────────
echo ""
echo -e "${BOLD}  ╔═══════════════════════════════╗${NC}"
echo -e "${BOLD}  ║   AssistClaw Uninstaller       ║${NC}"
echo -e "${BOLD}  ╚═══════════════════════════════╝${NC}"
echo ""

if $PURGE; then
  warn "PURGE mode — all config, memory, and skills data will be deleted"
  echo ""
fi

# ─────────────────────────────────────────────
# Step 1: Stop running daemon
# ─────────────────────────────────────────────
stop_daemon() {
  log "Stopping AssistClaw daemon (if running)..."

  # Try the built-in stop command first
  if command -v assistclaw >/dev/null 2>&1; then
    assistclaw stop 2>/dev/null && ok "Daemon stopped via assistclaw stop" && return
  fi

  # macOS launchd
  local plist="$HOME/Library/LaunchAgents/com.assistclaw.agent.plist"
  if [[ -f "$plist" ]]; then
    launchctl unload "$plist" 2>/dev/null && ok "Unloaded launchd service"
    rm -f "$plist"
    ok "Removed launchd plist: $plist"
  fi

  # Linux systemd (user)
  if command -v systemctl >/dev/null 2>&1; then
    if systemctl --user is-active --quiet assistclaw 2>/dev/null; then
      systemctl --user stop assistclaw 2>/dev/null
      ok "Stopped systemd user service"
    fi
    if systemctl --user is-enabled --quiet assistclaw 2>/dev/null; then
      systemctl --user disable assistclaw 2>/dev/null
      ok "Disabled systemd user service"
    fi
  fi

  # Linux systemd (system)
  if command -v systemctl >/dev/null 2>&1; then
    if systemctl is-active --quiet assistclaw 2>/dev/null; then
      sudo systemctl stop assistclaw 2>/dev/null
      sudo systemctl disable assistclaw 2>/dev/null
      ok "Stopped system-level systemd service"
    fi
    local service="/etc/systemd/system/assistclaw.service"
    if [[ -f "$service" ]]; then
      sudo rm -f "$service"
      sudo systemctl daemon-reload 2>/dev/null
      ok "Removed systemd service file"
    fi
  fi

  # Kill by process name as final fallback
  if pkill -f "assistclaw" 2>/dev/null; then
    ok "Killed remaining assistclaw processes"
  fi
}

# ─────────────────────────────────────────────
# Step 2: Remove binary and bundled skills
# ─────────────────────────────────────────────
remove_binary() {
  local candidates=(
    "$INSTALL_DIR/assistclaw"
    "$HOME/.local/bin/assistclaw"
    "/usr/bin/assistclaw"
    "/opt/assistclaw/assistclaw"
  )

  local found=false
  for bin in "${candidates[@]}"; do
    if [[ -f "$bin" ]]; then
      local dir
      dir="$(dirname "$bin")"
      rm -f "$bin"
      ok "Removed binary: $bin"
      # Remove bundled skills directory next to binary
      if [[ -d "$dir/skills" ]]; then
        rm -rf "$dir/skills"
        ok "Removed bundled skills: $dir/skills"
      fi
      found=true
    fi
  done

  if ! $found; then
    warn "Binary not found in standard locations (may already be removed)"
  fi
}

# ─────────────────────────────────────────────
# Step 3: Remove shell completion (if installed)
# ─────────────────────────────────────────────
remove_completion() {
  local completions=(
    "/usr/local/share/bash-completion/completions/assistclaw"
    "/usr/share/bash-completion/completions/assistclaw"
    "$HOME/.local/share/bash-completion/completions/assistclaw"
    "$HOME/.zsh/completions/_assistclaw"
  )
  for f in "${completions[@]}"; do
    if [[ -f "$f" ]]; then
      rm -f "$f"
      ok "Removed shell completion: $f"
    fi
  done
}

# ─────────────────────────────────────────────
# Step 4: Remove user data (--purge only)
# ─────────────────────────────────────────────
remove_user_data() {
  if [[ ! -d "$STATE_DIR" ]]; then
    warn "State directory not found: $STATE_DIR"
    return
  fi

  echo ""
  echo -e "${RED}${BOLD}⚠ This will permanently delete:${NC}"
  echo -e "  ${RED}• $STATE_DIR/assistclaw.yaml  (config)${NC}"
  echo -e "  ${RED}• $STATE_DIR/memory/          (agent memory)${NC}"
  echo -e "  ${RED}• $STATE_DIR/skills/          (installed skills)${NC}"
  echo -e "  ${RED}• $STATE_DIR/tools/           (generated tools)${NC}"
  echo -e "  ${RED}• $STATE_DIR/logs/            (logs)${NC}"
  echo ""
  read -r -p "Type 'yes' to confirm: " confirm
  if [[ "$confirm" != "yes" ]]; then
    warn "Skipped data deletion"
    return
  fi

  rm -rf "$STATE_DIR"
  ok "Removed user data: $STATE_DIR"
}

# ─────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────
stop_daemon
remove_binary
remove_completion

if $PURGE; then
  remove_user_data
else
  echo ""
  warn "Your config and data at ${BOLD}$STATE_DIR${NC} were kept."
  warn "To also remove them, run: ${BOLD}bash uninstall.sh --purge${NC}"
fi

echo ""
echo -e "${GREEN}${BOLD}AssistClaw uninstalled.${NC}"
echo ""
