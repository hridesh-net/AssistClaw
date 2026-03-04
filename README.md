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
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go" alt="Go">
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
| **Semantic** | sqlite-vec | Vector similarity (finds related past conversations) |

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

### 🦾 Hardware Sensing
C++ bridge for Camera (OpenCV), Audio (PortAudio), GPIO (pigpio) — runs natively on Raspberry Pi 5.

---

## 🚀 Quick Start

### 1. Install

**One-liner (Linux / macOS):**
```bash
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.sh | bash
```

**Or build from source:**
```bash
git clone https://github.com/hridesh-net/AssistClaw.git
cd AssistClaw && make build
```

**Uninstall anytime:**
```bash
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/uninstall.sh | bash
```

### 2. Onboard
```bash
assistclaw onboard
```
Interactive wizard — picks your LLM provider, configures channels, sets up Plano routing if you want it.

### 3. Run
```bash
# Interactive REPL
assistclaw agent

# Background daemon
assistclaw start --daemon

# Single message
assistclaw agent --message "Summarize this repo"
```

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
| `assistclaw start --daemon` | Launch as background service |
| `assistclaw stop` | Stop background service |
| `assistclaw status` | Show PID, uptime, connected channels |
| `assistclaw restart` | Restart service |

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

| | OpenClaw | NanoClaw | ZeroClaw | **AssistClaw** |
|:--|:--|:--|:--|:--|
| **Language** | Python/Node | TypeScript | Rust | **Go** |
| **Footprint** | ~1.5 GB | Minimal | ~5 MB | **~40 MB** |
| **Providers** | Managed | Claude only | Trait-based | **15+** |
| **Smart Routing** | ❌ | ❌ | ❌ | **✅ Plano** |
| **MCP** | ❌ | ❌ | ❌ | **✅ Server + Client** |
| **Security Layer** | ❌ | ❌ | ❌ | **✅ Guardrail + Audit** |
| **Hardware** | Basic | None | None | **C++ Bridge** |
| **Channels** | Limited | None | None | **WA/TG/Discord/Slack** |
| **Raspberry Pi** | ❌ | ❌ | ❌ | **✅** |

---

## 📚 Documentation

| Doc | Description |
|-----|-------------|
| [ASSISTCLAW.md](ASSISTCLAW.md) | Full feature reference and architecture deep dive |
| [CHANGELOG.md](CHANGELOG.md) | What's new in each release |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Developer guide and architecture overview |
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
