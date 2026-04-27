<p align="center">
  <img src="https://github.com/hridesh-net/AssistClaw/blob/main/doc/assets/assistClaw.png" alt="AssistClaw Logo" width="320">
</p>

<h1 align="center">AssistClaw</h1>

<p align="center">
  <em>The Autonomous Edge Intelligence System — built in Go, runs anywhere.</em>
</p>

<p align="center">
  <a href="https://github.com/hridesh-net/AssistClaw/actions"><img src="https://img.shields.io/github/actions/workflow/status/hridesh-net/AssistClaw/ci.yml?branch=main&style=for-the-badge&logo=github&label=CI" alt="CI"></a>
  <a href="https://github.com/hridesh-net/AssistClaw/releases"><img src="https://img.shields.io/github/v/release/hridesh-net/AssistClaw?include_prereleases&style=for-the-badge&logo=github&color=6f42c1" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-22c55e?style=for-the-badge" alt="MIT"></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/Platforms-Linux%20•%20macOS%20•%20Windows%20•%20RPi-orange?style=for-the-badge" alt="Platforms">
</p>

<br>

```
╔══════════════════════════════════════════════════════════════╗
║  You  →  AssistClaw  →  Any LLM  →  Tools + Skills + Memory ║
║            ↑                                                  ║
║     WhatsApp · Telegram · Discord · Slack · Web UI           ║
╚══════════════════════════════════════════════════════════════╝
```

---

## ⚡ Why AssistClaw?

Most AI agents are either too simple or too heavy. AssistClaw hits the sweet spot:

| | AssistClaw | Typical Python Agent |
|---|---|---|
| **Startup time** | ~50ms | 2–5s |
| **Memory footprint** | ~40 MB | 400–1500 MB |
| **LLM providers** | 15+ | 1–3 |
| **Runs on Raspberry Pi** | ✅ | ❌ |
| **Token optimization** | Graph-first (~66% savings) | None |
| **Built-in security** | Guardrail + Audit log | None |
| **Self-hostable** | ✅ | ✅ |

---

## 🗺️ Architecture at a Glance

```
                    ┌─────────────────────────────────┐
                    │           AssistClaw             │
                    │                                  │
  Channels ──────►  │  ┌──────────┐  ┌─────────────┐  │
  WhatsApp          │  │  Runner  │  │   Security  │  │
  Telegram          │  │ (agent   │  │  Guardrail  │  │
  Discord           │  │  loop)   │  │  Audit Log  │  │
  Slack             │  └────┬─────┘  └─────────────┘  │
  Web/REST/WS       │       │                          │
                    │  ┌────▼──────────────────────┐   │
                    │  │        Tool Graph          │   │
                    │  │  bash · web · files · MCP │   │
                    │  └────┬──────────────────────┘   │
                    │       │                          │
                    │  ┌────▼──────────────────────┐   │
                    │  │      3-Tier Memory         │   │
                    │  │  Working│Episodic│Semantic  │   │
                    │  └───────────────────────────┘   │
                    └────────┬────────────────────────┘
                             │
              ┌──────────────▼──────────────┐
              │    Any LLM Provider          │
              │  OpenAI · Anthropic · Ollama │
              │  Bedrock · Groq · Mistral … │
              └──────────────────────────────┘
```

---

## ✨ Features

### 🧠 Three-Tier Memory — Zero Infrastructure
No vector databases, no cloud services. Everything local.

| Tier | Storage | What for |
|------|---------|----------|
| **Working** | In-RAM | Active conversation context |
| **Episodic** | SQLite FTS5 | Full-text search across all sessions |
| **Semantic** | sqlite-vec | Embeddings + similarity search (optional embedder) |

### 🕸️ Skill Graph — 66% Fewer Tokens
Skills aren't flat files — they're **lazy-loaded graphs**. The agent reads only the nodes it needs.

```
coding/
├── INDEX           ← agent reads this first (50 tokens)
├── python.md       ← loaded only if query is Python
├── debugging.md    ← loaded only if agent needs debug help
└── testing.md      ← loaded only if tests are mentioned
```

Traditional skills: send **all** skill content every turn.  
AssistClaw: send the **index** → agent traverses only what it needs. ~66% token reduction.

### 🔒 Security Layer *(v3.6.0)*
Production-grade runtime protection — no configuration needed to get started.

- **Guardrail**: pre/post/tool-call checks for prompt injection, PII leakage, dangerous bash commands
- **Audit Log**: every tool call + skill read logged with HMAC hash chain (tamper-evident)
- **`assistclaw security verify`**: detects exactly which log entry was tampered with

```yaml
security:
  mode: enforce      # monitor | enforce | strict
  pii_mask: true     # [REDACTED:email] in logs
```

### 🤖 Plano Smart Routing *(v3.2.0)*
Auto-routes each prompt to the right model by complexity.

```
Simple "what's 2+2?"  →  gpt-4o-mini  (fast, cheap)
Complex code review   →  claude-opus  (powerful)
```

### 🔌 MCP Integration *(v3.3.0)*
Works as **both** an MCP server (expose your agent to Claude Desktop / Cursor) and a client (consume external MCP servers as skill nodes).

### 📡 Multi-Channel — One Agent, Everywhere

| Channel | Notes |
|---------|-------|
| WhatsApp | Multi-device, no QR scanning |
| Telegram | Bot API |
| Discord | Bot |
| Slack | App |
| REST + WebSocket | Self-hosted gateway |
| Web UI | Built-in |

### 📧 Autonomous Email Assistant *(v3.10+)*

Watch one or more mailboxes (IMAP/SMTP, Gmail API, Microsoft Graph), summarize new mail, draft a reply, and post to a **configured chat channel**. Replies send **only after explicit approval**; **mailbox deletion is not supported** by design.

- **Setup wizard:** `assistclaw email setup` — writes `email:` into `assistclaw.yaml` and can run Gmail/Graph OAuth in the same flow.
- **Notify session:** use `--notify-session` (e.g. `tg:123`) or **`--from-channel telegram`** to pick the latest matching session from episodic history (send at least one message on that channel first).
- **OAuth:** `assistclaw email login-gmail --account <name>` / `assistclaw email login-graph --account <name>` (requires client env vars; see command help).
- **Ops:** `assistclaw email pending`, `approve|reject|edit`, `status`, `doctor`.

### 🦾 Hardware Sensing
C++ bridge for Camera (OpenCV) and Audio (PortAudio) — runs natively on Raspberry Pi 5.

### 🪶 Markdown workspace & autonomy *(v3.9+)*
The state directory under `~/.assistclaw` uses familiar agent-workspace files: **`SOUL.md`**, **`IDENTITY.md`**, **`USER.md`**, **`AGENTS.md`**, **`HEARTBEAT.md`**, daily **`memory/`** notes, and long-term **`MEMORY.md`**. Templates are seeded on first run.

- **Heartbeat**: optional periodic synthetic turns on a **dedicated session** (`agent.heartbeat`) when you run `assistclaw start` or any messaging channel — good for proactive checks without spamming chat.
- **Planning & reflection**: `agent.planning` defaults **on** (milestone-style plan at the start of a turn); `agent.reflection` is **opt-in** (self-critique + lesson hooks, extra LLM call).
- **Autonomous mode**: `assistclaw auto "<goal>"` or `/auto` in chat runs until the model calls **`finish_task`**, with an upfront plan when planning is enabled and **checkpoints every 25 iterations** on long runs.
- **Cron**: YAML `cron:` entries plus **`assistclaw cron add`** persist jobs in **`cron_jobs.json`**; scheduled runs use the **same runner** as the gateway (tool graph + guardrail + audit).
- **Local dashboards**: Put HTML/CSS/JS under **`~/.assistclaw/workspace/public/`**. With **`assistclaw start`**, the gateway serves them at **`http://<gateway.host>:<gateway.port>/workspace/...`** (default `http://127.0.0.1:18790/workspace/...`) so the agent can give you a normal browser URL. That tree is **not** protected by the gateway Bearer token—do not store secrets there.
- **Extensions (built-in)**: AssistClaw does not load third-party Node plugin bundles. Use **`assistclaw extensions list`** to see built-in coverage (channels, MCP, webhooks, cron, skills, browser, voice). Optional **`extensions.prompt_files`** in **`assistclaw.yaml`** appends extra markdown to the system prompt.

---

## 🚀 Quick Start

### 1. Install

**One-liner (Linux / macOS):**
```bash
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.sh | bash
```

Pin a release or build from source with environment variables:

| Variable | Purpose |
|----------|---------|
| `ASSISTCLAW_VERSION` | Git tag (e.g. `v3.10.4`) or `latest` (default) |
| `INSTALL_DIR` | Binary location (default: `/usr/local/bin`, or `~/.local/bin` if not writable) |
| `STATE_DIR` | Config root (default: `~/.assistclaw`) |
| `FORCE_BUILD=1` | Compile with Go instead of downloading a release asset |
| `SKIP_VENV=1` | Skip Python venv creation |
| `SKIP_SENSING=1` | Skip optional C++ sensing build |
| `ASSISTCLAW_REPO` | `owner/repo` override for forks |

**Windows (PowerShell):**
```powershell
Invoke-WebRequest -UseBasicParsing https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.ps1 -OutFile install.ps1
.\install.ps1
# Optional: .\install.ps1 -Version v3.10.4
```
Default install path is **`%USERPROFILE%\.local\bin`** (add that folder to your user `PATH` if `assistclaw` is not found).

**Or build from source:**
```bash
git clone https://github.com/hridesh-net/AssistClaw.git
cd AssistClaw && make build
```

GitHub **release assets** are built with `-tags fts5` (SQLite full-text search for episodic memory). If you need extra build tags (for example optional local-model integrations), compile from source with the tags your environment supports.

**Uninstall:**
```bash
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/uninstall.sh | bash
# Remove binary + services but keep ~/.assistclaw:
#   bash uninstall.sh
# Remove everything including config and memory:
#   bash uninstall.sh --purge
# Non-interactive purge:
#   bash uninstall.sh -y --purge
```
The uninstaller stops **`assistclaw`** using **`~/.assistclaw/assistclaw.pid`** when present, removes launchd/systemd units, optional Plano Docker container, shell completions, and (with **`--purge`**) state, **`cron_jobs.json`**, security logs, and cache.

### 2. Onboard
```bash
assistclaw onboard
```
Interactive wizard — picks your LLM provider, configures channels, sets up Plano routing if you want it.

### 3. Run
```bash
# Interactive REPL
assistclaw agent

# Background daemon (runs fast preflight first: same checks as `assistclaw doctor --skip-network`)
assistclaw start --daemon

# Single message
assistclaw agent --message "Summarize this repo"

# Continuous autonomous goal (until finish_task)
assistclaw auto "Monitor disk space and summarize weekly"

# Scheduled prompts (merged with static cron: in YAML)
assistclaw cron list
assistclaw cron add "@hourly" "Quick health check of this machine"
```

**Preflight before start:** `assistclaw start`, `assistclaw restart`, `assistclaw gateway start`, and `assistclaw gateway serve` run a **fast local check** (config validation + non-network doctor checks) before binding the gateway. Failures exit non-zero so the process does not listen on a broken config. Use **`--preflight-full`** to run the same network probes as full `assistclaw doctor` (LLM + channel APIs). Use **`--skip-preflight`** only if you accept the risk (a single warning is printed).

**Fresh install CI and TTFM:** CI runs **`scripts/ci-fresh-install.sh`** on **Linux and macOS** (source `install.sh` + `doctor` on a minimal config). Doctor **text** lines are **sorted by check id** for stable golden snapshots; refresh with **`UPDATE_SNAPSHOTS=1 go test ./cmd/assistclaw/ -run TestDoctorTextSnapshot_ValidMinimalSkipNetwork -count=1`** or the **`update-doctor-snapshot`** workflow. For **time-to-first-message** definitions, baselines, and flake policy, see [doc/runbooks/ttfm-measurement.md](doc/runbooks/ttfm-measurement.md).

---

## ⚙️ Configuration

**Location:** `~/.assistclaw/assistclaw.yaml`

```yaml
# ─── LLM Provider ────────────────────────────────────────────
providers:
  anthropic:
    api_key: "sk-ant-..."
    default_model: "claude-3-5-haiku-20241022"

  # Or OpenAI, Ollama, Bedrock, Groq, Mistral, DeepSeek ...

# ─── Agent behavior (optional) ───────────────────────────────
agent:
  # planning defaults ON when omitted (upfront milestones + autonomous plan)
  planning: true
  # reflection: false   # opt-in: self-critique after a completed turn
  heartbeat:
    enabled: true
    interval: 30m
    session_id: assistclaw:heartbeat   # dedicated session; do not use for normal chat
    # prompt: "..."                    # optional; defaults to HEARTBEAT.md instructions

# ─── Static cron jobs (optional; CLI also writes cron_jobs.json) ─
cron:
  - id: morning
    schedule: "0 9 * * *"
    prompt: "Summarize overnight logs and anything urgent"

# ─── Smart Routing (optional) ────────────────────────────────
plano:
  enabled: true
  endpoint: "http://localhost:12000/v1"
  preferences:
    - description: "Simple queries"
      prefer_model: "openai/gpt-4o-mini"
    - description: "Complex code/reasoning"
      prefer_model: "anthropic/claude-opus-4"

# ─── Security ────────────────────────────────────────────────
security:
  mode: enforce          # monitor | enforce | strict
  pii_mask: true
  profile: full          # full | coding — tool visibility profile

# ─── MCP (optional) ──────────────────────────────────────────
mcp:
  server:
    enabled: true
    transport: stdio
  clients:
    - name: filesystem
      command: "npx @modelcontextprotocol/server-filesystem /home"

# ─── Messaging Channels (optional) ──────────────────────────
channels:
  telegram:
    bot_token: "..."
  discord:
    bot_token: "..."

# ─── Autonomous email (optional) ────────────────────────────
# Prefer: assistclaw email setup
email:
  enabled: true
  notify:
    channel: telegram
    session_id: "tg:123456789"
  accounts:
    - name: personal
      backend: imap
      imap:
        host: imap.example.com:993
        username: "you@example.com"
        password: "${IMAP_APP_PASSWORD}"
      smtp:
        host: smtp.example.com
        port: 587
        username: "you@example.com"
        password: "${SMTP_PASSWORD}"
        starttls: true
```

---

## 🛠️ Commands

<details>
<summary><strong>Core</strong></summary>

| Command | What it does |
|---------|-------------|
| `assistclaw onboard` | Interactive setup wizard |
| `assistclaw agent` | Start REPL session |
| `assistclaw agent --message "..."` | Single-shot message |
| `assistclaw start --daemon` | Launch as background service (preflight runs first; see `--skip-preflight` / `--preflight-full`) |
| `assistclaw stop` | Stop background service |
| `assistclaw status` | Show PID, uptime, connected channels |
| `assistclaw restart` | Restart service |
| `assistclaw auto "<goal>"` | Autonomous loop until `finish_task` |
| `assistclaw gateway` | Gateway / web UI commands (see `--help`) |
| `assistclaw cron list` / `add` / `remove` | Persistent scheduled prompts |

</details>

<details>
<summary><strong>Email</strong></summary>

| Command | What it does |
|---------|-------------|
| `assistclaw email setup` | Guided wizard: enable email, set notify channel/session, add an account; optional `--from-channel` to auto-pick session |
| `assistclaw email login-gmail --account NAME` | Gmail OAuth; writes token under state dir |
| `assistclaw email login-graph --account NAME` | Microsoft Graph OAuth |
| `assistclaw email pending` | List pending draft approval tokens |
| `assistclaw email approve TOKEN` / `reject` / `edit TOKEN ...` | Approve send, reject, or replace draft body |
| `assistclaw email status` | Show `email.enabled` and notify settings |
| `assistclaw email doctor` | Email-focused checks (config + token paths) |

</details>

<details>
<summary><strong>Skills</strong></summary>

| Command | What it does |
|---------|-------------|
| `assistclaw skills list` | Show installed skills |
| `assistclaw skills install <name>` | Install a skill |
| `assistclaw skills remove <name>` | Remove a skill |

</details>

<details>
<summary><strong>MCP</strong></summary>

| Command | What it does |
|---------|-------------|
| `assistclaw mcp serve` | MCP server over stdio (for Claude Desktop / Cursor) |
| `assistclaw mcp serve --transport http` | MCP server over HTTP-SSE (port 5173) |
| `assistclaw mcp add --name n --command cmd` | Register external MCP server |
| `assistclaw mcp list-tools` | Compact tool index |
| `assistclaw mcp status` | Server + client status |

**Claude Desktop / Cursor config:**
```json
{
  "mcpServers": {
    "assistclaw": {
      "command": "assistclaw",
      "args": ["mcp", "serve"]
    }
  }
}
```

</details>

<details>
<summary><strong>Security</strong></summary>

| Command | What it does |
|---------|-------------|
| `assistclaw security status` | Guardrail mode, log size, event count |
| `assistclaw security verify` | Verify HMAC chain — detects any tampering |
| `assistclaw security report` | Events by type, tool, skill, and actor |
| `assistclaw security tail` | Live audit event stream |

</details>

<details>
<summary><strong>Providers & Memory</strong></summary>

| Command | What it does |
|---------|-------------|
| `assistclaw providers list` | List LLM providers + available models |
| `assistclaw memory search <query>` | Search conversation history |
| `assistclaw tools list` | List all agent tools |

</details>

---

## 🦞 The Claw Ecosystem

| | Node + plugins | NanoClaw | ZeroClaw | **AssistClaw** |
|:--|:--|:--|:--|:--|
| **Language** | TypeScript (gateway) | TypeScript | Rust | **Go** |
| **Footprint** | Large (Node + UI) | Minimal | Ultra-light (<5 MB) | **Light (~40 MB)** |
| **Providers** | Many (plugins) | Anthropic mostly | 22+ providers | **15+ providers** |
| **Smart Routing** | Varies | ❌ | ❌ | **✅ Plano proxy** |
| **MCP** | Ecosystem | ❌ | ❌ | **✅ Server + Client** |
| **Security** | Harden over time | ✅ Container isolation | ✅ Strict allowlists | **✅ Guardrail + Audit Log** |
| **Hardware** | Basic | ❌ | ❌ | **✅ C++ Sensing Bridge** |
| **Channels** | Many (Telegram, Slack, …) | ❌ | ❌ | **✅ WA/TG/Discord/Slack** |
| **Raspberry Pi** | Often VPS / desktop | ✅ | ✅ | **✅ (Native ARM64)** |

---

## 📚 Documentation

| Doc | Description |
|-----|-------------|
| [ASSISTCLAW.md](ASSISTCLAW.md) | Full feature reference and architecture deep dive |
| [CHANGELOG.md](CHANGELOG.md) | What's new in each release |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Developer guide and architecture overview |
| [Adapter Contributor Checklist](CONTRIBUTING.md#new-channel-adapter-checklist) | Merge checklist for new/updated channel adapters (Sprint-01 Stories 001-006) |
| [Channel Capability Matrix](doc/channels/capability-registry.md) | Source-of-truth capability table for channel behavior |
| [DLQ Runbook](doc/runbooks/dlq-inspection.md) | How to inspect and triage outbound delivery failures |
| [Internal SLOs & error budgets](doc/observability/internal-slo-error-budgets.md) | Reliability targets, metrics mapping, error-budget policy (internal) |
| [On-call runbook](doc/runbooks/on-call-dashboards-alerts.md) | Dashboards, alerts, triage paths (STORY-012) |
| [Runbooks index](doc/runbooks/README.md) | All operational runbooks in one list |
| [doc/](doc/) | Additional docs, assets, and guides |

---

## 👥 Contributors

<a href="https://github.com/hridesh-net/AssistClaw/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=hridesh-net/AssistClaw" />
</a>

Made with [contrib.rocks](https://contrib.rocks).

---

## 📄 License

Licensed under the **MIT License**. See [LICENSE](LICENSE) for details.

---

<p align="center">
  <strong>AssistClaw</strong> — The Autonomous Edge Intelligence System<br>
  <em>Built for the edge. Ready for production. Open forever.</em>
</p>
