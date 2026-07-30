#!/usr/bin/env bash
# pi_gemma_test.sh — build AssistClaw natively on a Raspberry Pi (64-bit) with
# local Gemma enabled, then run a standalone Gemma inference test.
#
# WHY NATIVE: `make pi` / `make cross` intentionally strip local Gemma — the
# gollama.cpp runtime loads llama.cpp via libffi/dlopen at runtime, which does
# not survive a portable static (musl) build. Local Gemma therefore requires a
# native build on the Pi with the assistclaw_localgemma tag (what `make build`
# does).
#
# FIRST-RUN NETWORK: the vendored gollama libs ship linux_amd64 but NOT
# linux_arm64, so the first inference downloads the aarch64 llama.cpp runtime
# from GitHub releases. Internet is required for the first run (one-time).
#
# Usage (on the Pi, from the repo root):
#   bash scripts/pi_gemma_test.sh
#   PROMPT="What is 2+2?" MAX_TOKENS=32 bash scripts/pi_gemma_test.sh
#   GGUF=/path/to/model.gguf bash scripts/pi_gemma_test.sh
set -euo pipefail

PROMPT="${PROMPT:-Say hello in one short sentence.}"
MAX_TOKENS="${MAX_TOKENS:-64}"
GGUF="${GGUF:-}"
# Let the go command auto-download the toolchain version pinned in go.mod
# (go 1.26.2) if the system Go is older but >= 1.21.
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

say() { printf '\n\033[0;34m== %s ==\033[0m\n' "$*"; }
die() { printf '\n\033[0;31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

say "AssistClaw local Gemma test — Raspberry Pi"

# 1) Architecture / OS check ------------------------------------------------
arch="$(uname -m)"
echo "arch: $arch"
case "$arch" in
	aarch64|arm64) ;;
	*) echo "WARNING: expected a 64-bit ARM OS (aarch64). Local Gemma needs 64-bit; a 32-bit OS will not work." ;;
esac

# 2) Toolchain check --------------------------------------------------------
say "Checking build toolchain"
command -v go >/dev/null 2>&1 || die "Go not found. Install Go >= 1.21 (apt install golang, or the arm64 tarball from https://go.dev/dl/). GOTOOLCHAIN=auto will fetch $(grep '^go ' go.mod | awk '{print $2}')."
echo "go: $(go version)"
command -v cargo >/dev/null 2>&1 || die "Rust/cargo not found (needed for the TUI staticlib). Install: curl https://sh.rustup.rs -sSf | sh && rustup default stable"
echo "cargo: $(cargo --version)"
command -v cc >/dev/null 2>&1 || command -v gcc >/dev/null 2>&1 || die "C compiler not found (CGO). Install: sudo apt install -y build-essential"

# 3) Build natively WITH local Gemma ---------------------------------------
say "Building (make build → tags fts5,assistclaw_localgemma, CGO on, + Rust TUI)"
echo "This compiles the Rust TUI staticlib then the Go binary; first build is slow on a Pi."
make build
BIN="./bin/assistclaw"
[ -x "$BIN" ] || die "build did not produce $BIN"

# 4) Locate or fetch a GGUF -------------------------------------------------
if [ -z "$GGUF" ]; then
	if [ -f models/gemma-2-2b-it-Q4_K_M.gguf ]; then
		GGUF="$(pwd)/models/gemma-2-2b-it-Q4_K_M.gguf"
	elif [ -f "$HOME/.assistclaw/models/gemma-4-e2b-it.gguf" ]; then
		GGUF="$HOME/.assistclaw/models/gemma-4-e2b-it.gguf"
	else
		say "No local GGUF found — downloading via 'local-intel setup' (needs internet, ~3GB for gemma-4-e2b)"
		echo "Tip: to use the smaller ~1.6GB gemma-2-2b, scp it to ./models/gemma-2-2b-it-Q4_K_M.gguf and re-run."
		"$BIN" local-intel setup
		GGUF="$HOME/.assistclaw/models/gemma-4-e2b-it.gguf"
	fi
fi
[ -f "$GGUF" ] || die "GGUF not found at: $GGUF"
echo "Using GGUF: $GGUF ($(du -h "$GGUF" | cut -f1))"

# 5) Inference test ---------------------------------------------------------
say "localgemma info"
"$BIN" localgemma info || true

say "localgemma run (first run downloads the aarch64 llama.cpp runtime — one-time, needs internet)"
echo "prompt: $PROMPT"
echo "max-tokens: $MAX_TOKENS"
echo "free memory before:"; free -h 2>/dev/null || true
echo "---"
time "$BIN" localgemma run --user "$PROMPT" --gguf "$GGUF" --max-tokens "$MAX_TOKENS"
echo "---"
say "Done. If you got a coherent reply above, local Gemma works on this Pi."
echo "Next: enable it for the agent by adding to ~/.assistclaw/assistclaw.yaml:"
cat <<YAML
  agent:
    local_intel:
      enabled: true
      gguf_path: "$GGUF"
      max_tokens: 256
YAML
