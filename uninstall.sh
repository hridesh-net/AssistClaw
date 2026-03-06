#!/usr/bin/env bash
# AssistClaw uninstaller — v3.4+
# Removes the binary, system service (launchd/systemd), Plano Docker container,
# and optionally all user data (config, memory, skills, cron, web UI state).
#
# Usage:
#   bash uninstall.sh              # removes binary & service, keeps ~/.assistclaw data
#   bash uninstall.sh --purge      # removes everything including config, memory & all data
#   bash uninstall.sh --keep-data  # same as default (explicit alias)

set -eo pipefail

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
STATE_DIR="${STATE_DIR:-$HOME/.assistclaw}"
PURGE=false
AUTO_CONFIRM=false

# ─────────────────────────────────────────────
# Colors
# ─────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

log()  { echo -e "${CYAN}[assistclaw]${NC} $*"; }
ok()   { echo -e "${GREEN}  ✓${NC} $*"; }
warn() { echo -e "${YELLOW}  !${NC} $*"; }
err()  { echo -e "${RED}  ✗${NC} $*" >&2; exit 1; }
step() { echo -e "\n${BOLD}$*${NC}"; }

for arg in "$@"; do
  case "$arg" in
    --purge)     PURGE=true ;;
    --keep-data) PURGE=false ;;
    -y|--yes)    AUTO_CONFIRM=true ;;
    --help|-h)
      echo ""
      echo "Usage: bash uninstall.sh [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --purge      Also remove ~/.assistclaw (config, memory, skills, MCP servers list)"
      echo "  --keep-data  Keep user data (default behaviour)"
      echo "  -y, --yes    Auto-confirm deletion without prompting"
      echo "  --help       Show this help"
      echo ""
      exit 0
      ;;
  esac
done

# ─────────────────────────────────────────────
# Header
# ─────────────────────────────────────────────
echo ""
echo -e "${BOLD}${CYAN}"
echo "  ╭─────────────────────────────────────╮"
echo "  │   AssistClaw Uninstaller  v3.4+     │"
echo "  │   The Autonomous Edge Intelligence  │"
echo -e "  ╰─────────────────────────────────────╯${NC}"
echo ""

if $PURGE; then
  warn "PURGE mode — config, memory, skills, MCP config, cron state and logs will be deleted"
  echo ""
fi

# ─────────────────────────────────────────────
# Step 1: Stop running daemon & remove service
# ─────────────────────────────────────────────
stop_daemon() {
  step "1. Stopping AssistClaw daemon and removing system service..."

  # Try the built-in service uninstall command first (v3.4+)
  if command -v assistclaw >/dev/null 2>&1; then
    assistclaw service uninstall 2>/dev/null && ok "Service removed via 'assistclaw service uninstall'"
    assistclaw stop 2>/dev/null && ok "Daemon stopped via 'assistclaw stop'" || true
  fi

  # ── macOS launchd ────────────────────────────────────────────────────────
  local plist="$HOME/Library/LaunchAgents/com.assistclaw.agent.plist"
  local sys_plist="/Library/LaunchDaemons/com.assistclaw.agent.plist"

  if [[ -f "$plist" ]]; then
    launchctl bootout "gui/$(id -u)/com.assistclaw.agent" 2>/dev/null || \
      launchctl unload "$plist" 2>/dev/null || true
    rm -f "$plist"
    ok "Removed macOS launchd agent: $plist"
  fi

  if [[ -f "$sys_plist" ]]; then
    sudo launchctl bootout "system/com.assistclaw.agent" 2>/dev/null || \
      sudo launchctl unload "$sys_plist" 2>/dev/null || true
    sudo rm -f "$sys_plist"
    ok "Removed macOS system launchd daemon: $sys_plist"
  fi

  # ── Linux systemd (user) ─────────────────────────────────────────────────
  local user_service="$HOME/.config/systemd/user/assistclaw.service"

  if command -v systemctl >/dev/null 2>&1; then
    if systemctl --user is-active --quiet assistclaw 2>/dev/null; then
      systemctl --user stop assistclaw 2>/dev/null && ok "Stopped systemd user service"
    fi
    if systemctl --user is-enabled --quiet assistclaw 2>/dev/null; then
      systemctl --user disable assistclaw 2>/dev/null && ok "Disabled systemd user service"
    fi
    if [[ -f "$user_service" ]]; then
      rm -f "$user_service"
      systemctl --user daemon-reload 2>/dev/null || true
      ok "Removed systemd user service file: $user_service"
    fi

    # ── Linux systemd (system-level fallback) ────────────────────────────
    if systemctl is-active --quiet assistclaw 2>/dev/null; then
      sudo systemctl stop assistclaw 2>/dev/null
      sudo systemctl disable assistclaw 2>/dev/null
      ok "Stopped system-level systemd service"
    fi
    local sys_service="/etc/systemd/system/assistclaw.service"
    if [[ -f "$sys_service" ]]; then
      sudo rm -f "$sys_service"
      sudo systemctl daemon-reload 2>/dev/null || true
      ok "Removed system-level systemd service file"
    fi
  fi

  # ── Kill any remaining processes ─────────────────────────────────────────
  if pkill -f "assistclaw" 2>/dev/null; then
    ok "Killed remaining assistclaw processes"
  fi
}


# ─────────────────────────────────────────────
# Step 2: Stop & remove Plano Docker container
# ─────────────────────────────────────────────
remove_plano() {
  step "2. Removing Plano smart routing proxy (if running)..."

  if ! command -v docker >/dev/null 2>&1; then
    warn "Docker not installed — skipping Plano cleanup"
    return
  fi

  # Stop and remove the plano container (named "plano" by our onboarding)
  if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -q '^plano$'; then
    docker stop plano 2>/dev/null && ok "Stopped Plano container"
    docker rm   plano 2>/dev/null && ok "Removed Plano container"
  else
    warn "No Plano container found (may already be removed)"
  fi

  # Optionally remove the image to free disk space (only with --purge)
  if $PURGE; then
    if docker images katanemo/plano --format '{{.ID}}' 2>/dev/null | grep -q .; then
      docker rmi katanemo/plano 2>/dev/null && ok "Removed Plano Docker image (katanemo/plano)"
    fi
  else
    warn "Plano Docker image kept (run with --purge to also remove it)"
  fi
}

# ─────────────────────────────────────────────
# Step 3: Remove binary and bundled skills
# ─────────────────────────────────────────────
remove_binary() {
  step "3. Removing AssistClaw binary..."

  local candidates=(
    "$INSTALL_DIR/assistclaw"
    "$HOME/.local/bin/assistclaw"
    "/usr/bin/assistclaw"
    "/usr/local/bin/assistclaw"
    "/opt/assistclaw/assistclaw"
  )

  local found=false
  for bin in "${candidates[@]}"; do
    if [[ -f "$bin" ]]; then
      local dir
      dir="$(dirname "$bin")"
      if [[ -w "$dir" && -w "$bin" ]]; then
        rm -f "$bin"
        ok "Removed binary: $bin"
        # Remove bundled skills directory next to binary (if present)
        if [[ -d "$dir/skills" ]]; then
          rm -rf "$dir/skills"
          ok "Removed bundled skills: $dir/skills"
        fi
      else
        if command -v sudo >/dev/null 2>&1; then
          sudo rm -f "$bin"
          ok "Removed binary (via sudo): $bin"
          if [[ -d "$dir/skills" ]]; then
            sudo rm -rf "$dir/skills"
            ok "Removed bundled skills (via sudo): $dir/skills"
          fi
        else
          warn "Cannot remove $bin - permission denied and sudo not available"
        fi
      fi
      found=true
    fi
  done

  if ! $found; then
    warn "Binary not found in standard locations (may already be removed)"
  fi
}

# ─────────────────────────────────────────────
# Step 4: Remove shell completions
# ─────────────────────────────────────────────
remove_completion() {
  step "4. Removing shell completions..."

  local completions=(
    "/usr/local/share/bash-completion/completions/assistclaw"
    "/usr/share/bash-completion/completions/assistclaw"
    "$HOME/.local/share/bash-completion/completions/assistclaw"
    "$HOME/.zsh/completions/_assistclaw"
    "$HOME/.config/fish/completions/assistclaw.fish"
  )
  local removed=false
  for f in "${completions[@]}"; do
    if [[ -f "$f" ]]; then
      rm -f "$f"
      ok "Removed: $f"
      removed=true
    fi
  done
  if ! $removed; then
    warn "No shell completions found (skipped)"
  fi
}

# ─────────────────────────────────────────────
# Step 5: Remove user data (--purge only)
# ─────────────────────────────────────────────
remove_user_data() {
  step "5. Removing user data..."

  if [[ ! -d "$STATE_DIR" ]]; then
    warn "State directory not found: $STATE_DIR"
    return
  fi

  echo ""
  echo -e "${RED}${BOLD}  ⚠ This will permanently delete:${NC}"
  echo -e "  ${RED}• $STATE_DIR/assistclaw.yaml     (config — providers, gateway token, MCP, Plano)${NC}"
  echo -e "  ${RED}• $STATE_DIR/memory/             (working + episodic + vector memory)${NC}"
  echo -e "  ${RED}• $STATE_DIR/skills/             (installed skills)${NC}"
  echo -e "  ${RED}• $STATE_DIR/tools/              (auto-generated Python tools)${NC}"
  echo -e "  ${RED}• $STATE_DIR/sessions/           (conversation session data)${NC}"
  echo -e "  ${RED}• $STATE_DIR/cron/               (scheduled cron job state)${NC}"
  echo     "  • $STATE_DIR/logs/               (log files)"
  echo     "  • $HOME/.cache/assistclaw/       (cache files)"
  echo ""

  if ! $AUTO_CONFIRM; then
    read -r -p "  Type 'yes' to confirm permanent deletion: " confirm
    if [[ "$confirm" != "yes" ]]; then
      warn "Skipped data deletion — your data is intact"
      return
    fi
  fi

  rm -rf "$STATE_DIR"
  rm -rf "$HOME/.cache/assistclaw" 2>/dev/null || true
  ok "Removed all user data: $STATE_DIR and caches"
}

# ─────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────
stop_daemon
remove_plano
remove_binary
remove_completion

if $PURGE; then
  remove_user_data
else
  echo ""
  warn "Your config and data at ${BOLD}$STATE_DIR${NC} were kept."
  warn "To also remove them:  ${BOLD}bash uninstall.sh --purge${NC}"
  warn "To remove Plano image: ${BOLD}docker rmi katanemo/plano${NC}"
  warn "Gateway token was stored in: ${BOLD}$STATE_DIR/assistclaw.yaml${NC}"
fi

echo ""
echo -e "${GREEN}${BOLD}  ✓ AssistClaw v3.4+ uninstalled successfully.${NC}"
echo ""
