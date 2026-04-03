# AssistClaw

AssistClaw is a polyglot, production-grade AI assistant built in Go.
It shares the broader "Claw" agent philosophy and extends it with native hardware sensing, autonomous tool generation, a skill-graph memory system, smart multi-model routing, and a token-efficient MCP integration.

---

## Features

### LLM Providers (15+)

OpenAI, Anthropic, AWS Bedrock, Google Vertex AI, Ollama (local), vLLM, LM Studio, OpenRouter, Groq, Mistral, Together AI, Cohere, HuggingFace, NVIDIA NIM, Azure OpenAI, DeepSeek, Perplexity.

### Three-Tiered Memory

| Tier | Store | Use |
|------|-------|-----|
| **Working** | In-RAM | Per-session context with token-budget compaction |
| **Episodic** | SQLite FTS5 | Full-text search across conversation history |
| **Semantic** | `sqlite-vec` | Sub-millisecond vector similarity search (no separate vector DB) |

### Skill Graph

Skills are organized as **lazy-loaded graph nodes** (markdown files). The agent starts with a compact **Map of Content** (~200 tokens) and traverses into skill nodes on demand via `read_skill_node`. This saves 90%+ of tool-context tokens compared to loading all tool specs upfront.

```
Map of Content (~200 tokens)
  ├─ nano-pdf:   Edit and extract PDF content
  ├─ discord:    Send messages, manage channels
  └─ mcp:filesystem  Read/write files (external MCP server)

Agent calls: read_skill_node("nano-pdf") → gets tools → calls tool
```

Bundled skills: `1password`, `apple-notes`, `coding-agent`, `discord`, `nano-pdf` (and more at [AssistClaw skill registry](https://github.com/hridesh-net/AssistClaw)).

### Plano Smart Routing *(v3.2.0+)*

[Plano](https://github.com/katanemo/plano) is an open-source AI proxy powered by `Arch-Router-1.5B` that auto-routes each prompt to the right model based on complexity:

- **Simple queries** go to cheap, fast models (e.g. GPT-4o mini, Llama3 8B).
- **Complex tasks** go to powerful models (e.g. GPT-4o, DeepSeek R1).
- Works with any OpenAI-compatible backend.
- Docker setup happens **automatically** during `assistclaw onboard`.
- Graceful fallback to your primary provider if Plano is down.

**Config:**
```yaml
plano:
  enabled: true
  endpoint: "http://localhost:12000/v1"
  fallback_provider: "openai"
  preferences:
    - description: "Simple chat → fast model"
      prefer_model: "openai/gpt-4o-mini"
    - description: "Code/reasoning → powerful model"
      prefer_model: "openai/gpt-4o"
```

**vs. Standard Multi-model Routing:**

| | Standard YAML routing | Plano |
|---|---|---|
| **Routing logic** | Rule-based (regex/keywords) | AI model (Arch-Router-1.5B) |
| **Setup** | Manual rule config | Plain-English preferences |
| **Cost** | Requires manual tuning | Automatic optimization |
| **Model support** | All providers | OpenAI-compatible only |

### MCP Integration *(v3.3.0+)*

AssistClaw implements the [Model Context Protocol](https://modelcontextprotocol.io) in both directions.

#### MCP Server — expose skills to any MCP client

Clients like Claude Desktop, Cursor, and custom apps can connect to AssistClaw and use all its skills.

```bash
# stdio transport (use in Claude Desktop / Cursor config)
assistclaw mcp serve

# HTTP-SSE transport
assistclaw mcp serve --transport http --port 5173
```

**Claude Desktop config (`claude_desktop_config.json`):**
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

**Token-efficient by design:**

| | Standard MCP | AssistClaw MCP |
|---|---|---|
| Per-request token cost | All tool specs (~8 k tokens) | Compact index only (~200 tokens) |
| Tool spec loading | Eager (always upfront) | Lazy (`read_skill_node` on demand) |
| 20-skill library | ~8,000 tokens / request | ~400 tokens / request |

#### MCP Client — consume external MCP servers

Register any external MCP server. Its tools appear in the agent's skill graph as lazy-loaded nodes — grouped by prefix for Map of Content.

```bash
# Filesystem MCP (stdio, spawns child process)
assistclaw mcp add --name filesystem \
  --command "npx @modelcontextprotocol/server-filesystem /home"

# Browser MCP (HTTP)
assistclaw mcp add --name browser --url http://localhost:5174

# List the compact tool index
assistclaw mcp list-tools

# Show status + connected servers
assistclaw mcp status
```

**Config (`assistclaw.yaml`):**
```yaml
mcp:
  server:
    enabled: true
    transport: stdio   # or http
    http_port: 5173
  clients:
    - name: filesystem
      command: "npx @modelcontextprotocol/server-filesystem /home"
    - name: browser
      url: "http://localhost:5174"
```

---

## Architecture

```
assistclaw (Go binary)
├── Agent Runner       — prompt → tool-call loop, memory writes, skill-graph context
├── Provider Registry  — 15+ LLM backends (openai, anthropic, bedrock, ollama …)
├── Plano Provider     — OpenAI-compat proxy with complexity-aware routing
├── MCP Server         — stdio / HTTP-SSE, skill-graph-optimized tool listing
├── MCP Clients        — connects to external servers, injects as skill nodes
├── Skill Registry     — lazy-loaded markdown node graphs
├── Memory System      — working / episodic (SQLite FTS5) / semantic (sqlite-vec)
├── Channel Layer      — WhatsApp, Telegram, Discord, Slack, Gateway (REST+WS)
├── Tool Registry      — built-in tools + autonomous Python tool factory
└── C++ Sensing        — Camera (OpenCV), Audio (PortAudio), GPIO (pigpio)
```

**Python (sandboxed)** — isolated `venv` for running autonomous tools.  
**C++** — optional sensing layer. Not required for core functionality.

---

## Setup & Installation

```bash
# Automated (Linux / macOS)
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.sh | bash

# Uninstall
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/uninstall.sh | bash
```

Or build from source:
```bash
git clone https://github.com/hridesh-net/AssistClaw.git
cd AssistClaw && make build
```

Run the interactive setup wizard:
```bash
assistclaw onboard
```

---

## Usage

```bash
# Interactive REPL
assistclaw agent

# Single message
assistclaw agent -m "Summarise today's Hacker News top 10"

# Background daemon
assistclaw start --daemon
assistclaw status
assistclaw stop

# Skills
assistclaw skills list
assistclaw skills install coding-agent

# MCP
assistclaw mcp serve               # start as MCP server
assistclaw mcp add --name fs \
  --command "npx @mcp/server-filesystem /home"
assistclaw mcp list-tools

# Providers & memory
assistclaw providers list
assistclaw memory search "docker"
```

---

## Autonomous Tool Creation

Ask the agent to build its own tools:
> "Create a tool that fetches the top 5 Hacker News stories and formats them as markdown."

The agent drafts Python code, validates it against a safety policy (blocks `sudo`, network exfil etc.), runs it in an isolated `venv`, and saves it to `~/.assistclaw/tools/` for future use.

---

## Resource Usage

AssistClaw is designed to be lightweight:

```bash
# Check memory and CPU of a running daemon
ps -p $(cat ~/.assistclaw/assistclaw.pid) -o pid,comm,%cpu,%mem,rss
# Typical: ~42 MB RSS, <0.1% CPU at idle
```
