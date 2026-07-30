# AssistClaw on Raspberry Pi 5 — always-on Friday

The Pi 5 is the recommended always-on home for the daemon: email goals,
proactive rules, channels (Telegram/WhatsApp/Discord/Slack), the gateway web
UI, and the awareness store all run there 24/7, reachable from your phone and
laptop anywhere.

## 1. Get a binary

**Option A — cross-compile from your dev machine (fastest):**

```bash
make cross-deps   # once: checks zig, adds rustup targets
make pi           # builds dist/assistclaw-linux-arm64 (portable, static, fts5)
scp dist/assistclaw-linux-arm64 pi@<pi-host>:/tmp/assistclaw
```

Notes:
- The cross build is fully static (musl) — works on Raspberry Pi OS (64-bit),
  Ubuntu Server, or DietPi with zero runtime deps.
- Local Gemma (`assistclaw_localgemma`) is intentionally excluded from cross
  builds; build natively on the Pi if you want on-device fallback inference
  (Option B).

**Option B — build natively on the Pi:**

```bash
# on the Pi (64-bit OS required)
sudo apt install -y golang rustup build-essential && rustup default stable
git clone <your repo> && cd AssistClaw
make build        # builds Rust TUI staticlib + Go binary with fts5
```

## 2. Install + onboard

```bash
sudo install -m755 /tmp/assistclaw /usr/local/bin/assistclaw
assistclaw onboard          # providers, channels, email accounts
assistclaw persona friday   # optional: install the Friday persona
```

## 3. Run as a service (survives reboots and crashes)

```bash
assistclaw service install   # systemd user unit, Restart=on-failure, RestartSec=5
loginctl enable-linger $USER # REQUIRED on a headless Pi: start at boot without login
systemctl --user status assistclaw
```

## 4. Reach it from anywhere

### Phone + laptop, any network (recommended): Tailscale

The gateway embeds Tailscale (tsnet) — no separate tailscaled needed. In
`~/.assistclaw/config.yaml`:

```yaml
gateway:
  port: 8080
  token: "<long random string>"   # bearer token for the web UI / API
  bind: tailnet
  tailscale:
    mode: serve        # private to your tailnet, HTTPS automatic
    # mode: funnel     # public internet — only if you know what you're doing
```

First start prints a Tailscale auth URL in the logs — open it once to join
your tailnet. After that:

- **Web UI / chat:** `https://<pi-hostname>.<tailnet>.ts.net` from your phone
  or laptop. Add it to the phone home screen for an app-like experience.
- **Context signals:** the phone can post awareness signals (location,
  battery) to `POST /api/context/signal` with the bearer token.
- **Health:** `/livez`, `/readyz` for monitoring.

### Zero-setup fallback: chat channels

Telegram / WhatsApp / Discord / Slack channels are outbound connections from
the Pi — they work from anywhere with no networking setup at all, including
email-goal approvals (`approve TOKEN` buttons).

### LAN only

```yaml
gateway:
  bind: lan
  port: 8080
```

Then `http://<pi-ip>:8080` from devices on your network.

## 5. Quick health checklist

```bash
assistclaw doctor            # config, providers, channels
assistclaw status            # live TUI dashboard (CPU/RAM/uptime)
assistclaw email status      # mail watcher state
assistclaw goal list         # open email goals
journalctl --user -u assistclaw -f
```

## Testing local Gemma on the Pi

Local Gemma (on-device inference, no cloud) is the edge fallback. To test it:

```bash
# One-shot helper (native build + GGUF + inference test):
bash scripts/pi_gemma_test.sh
# or a custom prompt:
PROMPT="What is the capital of France?" MAX_TOKENS=32 bash scripts/pi_gemma_test.sh
```

What it does and the two things to know:

1. **Must be a native build.** `make pi` / `make cross` deliberately OMIT local
   Gemma (the gollama.cpp runtime loads llama.cpp via libffi/dlopen at runtime,
   which does not survive a portable static musl build). Local Gemma needs
   `make build` on the Pi (tags `fts5,assistclaw_localgemma`, CGO on) — which
   the helper script runs. Requires Go ≥1.21 (GOTOOLCHAIN=auto fetches the pinned
   version), Rust/cargo (for the TUI staticlib), and `build-essential`.

2. **First run downloads the aarch64 llama.cpp runtime.** No `linux_arm64`
   llama.cpp libs are vendored, so on first inference gollama downloads the
   aarch64 runtime from GitHub releases into `~/.cache/gollama/`. Internet is
   required for that first run only; afterwards it is cached and offline. (This
   is why local Gemma is not yet fully offline on a fresh Pi — to pre-stage it,
   copy a `libllama.so` + `libggml*.so` set into `libs/linux_arm64/` next to the
   binary, which the loader searches.)

Manual equivalent:

```bash
# copy your GGUF over (smaller gemma-2-2b is ideal for a Pi), or let setup fetch one:
scp models/gemma-2-2b-it-Q4_K_M.gguf pi@<host>:~/AssistClaw/models/
./bin/assistclaw localgemma info
./bin/assistclaw localgemma run --user "Say hi in one sentence." \
    --gguf ~/AssistClaw/models/gemma-2-2b-it-Q4_K_M.gguf --max-tokens 64
```

Then wire it into the agent (used as the offline fallback provider):

```yaml
agent:
  local_intel:
    enabled: true
    gguf_path: "/home/pi/AssistClaw/models/gemma-2-2b-it-Q4_K_M.gguf"
    max_tokens: 256
```

Model choice: **gemma-2-2b Q4_K_M (~1.6 GB)** is the sweet spot for a Pi 5;
gemma-4-e2b Q4 (~3 GB) needs 8 GB+. A Pi Zero 2W / ≤1 GB board cannot run either.
The engine is CPU-only (greedy decode) — expect modest tokens/sec.

## Known limits on the Pi

- The awareness idle-presence probe is macOS-only today; the Pi daemon still
  injects time-of-day + calendar context, and phones can post signals.
- Voice STT/TTS expects the external voice service (`scripts/voice_server.py`)
  — runs on the Pi but Whisper is slow on CPU; consider a small model.
- sensing/ (camera/audio C++ capture) builds on the Pi via `make build-sensing`.
