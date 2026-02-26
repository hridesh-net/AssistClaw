<p align="center">
  <img src="https://github.com/hridesh-net/AssistClaw/blob/main/doc/assets/assistClaw.png" alt="AssistClaw" width="400">
</p>

<h1 align="center">AssistClaw</h1>

<p align="center">
  <strong>The Autonomous Edge Intelligence System</strong>
</p>

<p align="center">
  <a href="https://github.com/hridesh-net/AssistClaw/actions"><img src="https://img.shields.io/github/actions/workflow/status/hridesh-net/AssistClaw/ci.yml?branch=main&style=for-the-badge" alt="CI Status"></a>
  <a href="https://github.com/hridesh-net/AssistClaw/releases"><img src="https://img.shields.io/github/v/release/hridesh-net/AssistClaw?include_prereleases&style=for-the-badge" alt="GitHub release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg?style=for-the-badge" alt="MIT License"></a>
</p>

---

**AssistClaw** is a polyglot, production-grade AI assistant system written in **Go**. Designed for low-latency, high-concurrency edge computing — it runs anywhere from a Raspberry Pi 5 to a high-end workstation, on any LLM provider, across any messaging channel.

## 🚀 Key Highlights

- **⚡ Native Performance**: Built in Go for near-instant startup, low memory footprint (~40 MB), and true multi-core concurrency.
- **🧠 Three-Tier Memory**: Local, zero-infrastructure memory system.
  - **Working**: Context-aware in-RAM session memory.
  - **Episodic**: SQLite FTS5 for fast full-text historical recall.
  - **Semantic**: `sqlite-vec` for native vector similarity search.
- **🗺️ Skill Graph**: Modular skill system where each skill is a lazy-loaded graph of markdown nodes — the agent traverses only what it needs, saving 90%+ of tool-context tokens.
- **🤖 Plano Smart Routing** *(v3.2.0)*: Optional open-source AI proxy that auto-routes each prompt to the right model by complexity — cheap models for simple queries, powerful models for complex tasks.
- **🔌 MCP Integration** *(v3.3.0)*: Token-efficient [Model Context Protocol](https://modelcontextprotocol.io) server (works with Claude Desktop, Cursor, etc.) **and** client (consume external MCP servers — filesystem, browser, etc. — as skill nodes).
- **🛠️ Autonomous Intelligence**: The agent can autonomously write, validate, sandbox-execute, and persist its own Python tools.
- **📡 Multi-Channel Connectivity**: Native support for WhatsApp, Telegram, Discord, Slack, and a high-performance REST/WebSocket Gateway.
- **🦾 Hardware Sensing**: C++ bridge for Camera (OpenCV), Audio (PortAudio), and GPIO (pigpio).
- **🎛️ Background Daemon**: Persistent background service with `start`, `stop`, `status`, and `restart` management.

---

## ✅ Verified & Working

- **LLM Providers**: AWS Bedrock, OpenAI (GPT-4o / 4.1), Anthropic (Claude 3.x), Ollama, Groq, Mistral, Together AI, DeepSeek, Perplexity, OpenRouter, Cohere, HuggingFace, NVIDIA NIM, vLLM, LM Studio.
- **Smart Routing**: Plano (Docker-based, auto-setup during onboarding).
- **MCP**: stdio + HTTP-SSE server / client (tested with Claude Desktop and MCP Inspector).
- **Messaging**: WhatsApp (multi-device), Telegram (Bot API), Gateway (REST+WS).
- **Platforms**: Raspberry Pi 5 (ARM64), Linux (AMD64), macOS (Darwin), Windows.

---

## 🆕 What's New

### v3.3.0 — Skill-Graph MCP
- **MCP Server**: Expose your agent's skills to Claude Desktop, Cursor, and any MCP-compatible client via `assistclaw mcp serve` (stdio) or HTTP-SSE.
- **MCP Client**: Register external MCP servers (`assistclaw mcp add`). Their tools appear in the agent's skill graph as lazy-loaded nodes — same 90%+ token savings.
- **Token-efficient by design**: Standard MCP dumps all specs upfront. AssistClaw serves a compact Map of Content and `read_skill_node` — specs are only fetched when the model actually needs them.

### v3.2.0 — Plano Smart Routing
- Opt-in AI proxy that auto-routes prompts to fast or powerful models based on complexity.
- Docker setup happens automatically during `assistclaw onboard`.
- When Plano is enabled, onboarding filters to show only OpenAI-compatible providers.
- Fallback to your primary provider if Plano is unreachable.

### v3.1.x — Daemon & Stability
- Background daemon with PID management (`start --daemon`, `stop`, `restart`, `status`).
- Windows cross-compile support.
- Uninstall script (`uninstall.sh`).

---

## 🦞 The Claw Ecosystem

| Feature | OpenClaw | NanoClaw | ZeroClaw | **AssistClaw** |
| :--- | :--- | :--- | :--- | :--- |
| **Language** | Python / Node.js | TypeScript | Rust | **Go** |
| **Footprint** | Heavy (~1.5 GB) | Minimal | ~5 MB | **~40 MB** |
| **LLM Providers** | Managed | Claude only | Trait-based | **15+ providers** |
| **Smart Routing** | ❌ | ❌ | ❌ | **✅ Plano** |
| **MCP** | ❌ | ❌ | ❌ | **✅ Server + Client** |
| **Hardware** | Basic | None | None | **Native C++ Bridge** |
| **Channels** | Limited | None | None | **WA/TG/Discord/Slack** |

---

## 🛠️ Installation

### Automated (Linux / macOS)
```bash
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.sh | bash
```

### Manual (All Platforms)
1. Download the latest binary from [GitHub Releases](https://github.com/hridesh-net/AssistClaw/releases).
2. Move to your PATH and make it executable.
3. Run `assistclaw onboard` to start the interactive guided setup.

### Uninstall
```bash
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/uninstall.sh | bash
```

---

## ⌨️ Commands

### Core
| Command | Description |
|---|---|
| `assistclaw onboard` | Interactive setup wizard |
| `assistclaw agent` | Start interactive REPL session |
| `assistclaw start --daemon` | Launch as background service |
| `assistclaw stop` | Stop background service |
| `assistclaw status` | Show PID and uptime |
| `assistclaw restart` | Restart background service |

### Skills
| Command | Description |
|---|---|
| `assistclaw skills list` | Show installed skills |
| `assistclaw skills install <name>` | Install a bundled skill |
| `assistclaw skills remove <name>` | Remove a skill |

### MCP *(v3.3.0)*
| Command | Description |
|---|---|
| `assistclaw mcp serve` | Start MCP server (stdio, for Claude Desktop / Cursor) |
| `assistclaw mcp serve --transport http` | Start HTTP-SSE MCP server (port 5173) |
| `assistclaw mcp add --name <n> --command <cmd>` | Register external MCP server (stdio) |
| `assistclaw mcp add --name <n> --url <url>` | Register external MCP server (HTTP) |
| `assistclaw mcp list-tools` | Show compact tool index |
| `assistclaw mcp status` | Show server status + connected servers |
| `assistclaw mcp remove <name>` | Unregister external MCP server |

### Providers & Memory
| Command | Description |
|---|---|
| `assistclaw providers list` | List registered providers + models |
| `assistclaw memory search <query>` | Search conversation history |
| `assistclaw tools list` | List registered agent tools |

---

## ⚙️ Configuration (`~/.assistclaw/assistclaw.yaml`)

```yaml
# Primary LLM provider
providers:
  openai:
    api_key: "sk-..."
    default_model: "gpt-4o-mini"

# Optional: Smart routing via Plano (v3.2.0+)
plano:
  enabled: true
  endpoint: "http://localhost:12000/v1"
  fallback_provider: "openai"
  preferences:
    - description: "Simple chat → fast model"
      prefer_model: "openai/gpt-4o-mini"
    - description: "Code/reasoning → powerful model"
      prefer_model: "openai/gpt-4o"

# Optional: MCP server + external servers (v3.3.0+)
mcp:
  server:
    enabled: true
    transport: stdio       # or http
    http_port: 5173
  clients:
    - name: filesystem
      command: "npx @modelcontextprotocol/server-filesystem /home"
    - name: browser
      url: "http://localhost:5174"
```

### Claude Desktop / Cursor integration
Add to your MCP config:
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

---

## 🏗️ Build from Source

```bash
git clone https://github.com/hridesh-net/AssistClaw.git
cd AssistClaw
make build
```

---

## 📖 Documentation

- **[ASSISTCLAW.md](ASSISTCLAW.md)**: Feature reference and architecture.
- **[Changelog](CHANGELOG.md)**: Release history and milestones.
- **[Contributing](CONTRIBUTING.md)**: Developer guide and architecture overview.

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

*AssistClaw — The Autonomous Edge Intelligence System.*
