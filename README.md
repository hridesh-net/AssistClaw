# AssistClaw

AssistClaw is a polyglot, production-grade AI assistant system written in Go. It extends the feature set of OpenClaw with hardware sensing, autonomous tool generation, full embedding model support, background scheduling, and a modular architecture.

## Key Features

- **15+ LLM Providers**: Support for OpenAI, Anthropic, Bedrock, Vertex, Ollama, vLLM, Groq, Mistral, and more.
- **Three-Tier Memory**: Local, zero-infrastructure memory system including Working (RAM), Episodic (SQLite FTS5), and Semantic (sqlite-vec vector search) tiers.
- **Autonomous Tool Generation**: The agent can write its own Python tools, validate them against a strict safety policy, sandbox execute them, and persist them for future use.
- **Hardware Integration**: C++ sensing layer bridging internal logic with Camera (OpenCV), Audio (PortAudio), and GPIO (pigpio) over NDJSON IPC.
- **Multi-Channel Messaging**: Native connections for Telegram, Discord, Slack, and a REST/WebSocket Gateway.
- **Skills System**: Reads `SKILL.md` files dynamically to inject specialized prompts and custom tools globally into the assistant context.
- **Browser Automation**: True headless browsing and page screenshot capability using `chromedp`.
- **Background Cron**: Schedule the agent to execute autonomous jobs periodically.

## Documentation

For a comprehensive understanding of the system architecture, logic, and how to replicate it, please refer to our **[Master Guide](file:///Users/elrosshinzo/Projects/Personal/AssistClaw/doc/master_guide.md)**.

Additionally, explore our technical deep-dive series:
- **[The Autonomous Execution Loop](file:///Users/elrosshinzo/Projects/Personal/AssistClaw/doc/deep-dives/execution-loop.md)**
- **[Dynamic Skills & Autonomous Tools](file:///Users/elrosshinzo/Projects/Personal/AssistClaw/doc/deep-dives/skills-and-tools.md)**
- **[Providers, Routing & Channels](file:///Users/elrosshinzo/Projects/Personal/AssistClaw/doc/deep-dives/providers-and-routing.md)**

---

### Automated Installation (Linux / macOS)
The fastest way to install AssistClaw is via our one-liner script. This detects your architecture, downloads the correct binary, and sets up your workspace:

```bash
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.sh | bash
```

### Manual Installation (All Platforms)
If you prefer to install manually or are on a restricted environment:

1. **Download**: Grab the latest binary for your platform from the [GitHub Releases](https://github.com/hridesh-net/AssistClaw/releases) page.
2. **Rename & Move**:
   - **Linux/macOS**: Rename to `assistclaw`, make it executable (`chmod +x`), and move it to your path (e.g., `/usr/local/bin` or `~/.local/bin`).
   - **Windows**: Rename to `assistclaw.exe` and move it to a folder in your PATH.
3. **Verify**: Run `assistclaw version` to ensure it's installed correctly.

### ARM64 / Raspberry Pi Optimized
AssistClaw is optimized for the Raspberry Pi 5. Our ARM64 binaries are statically linked with `musl`, ensuring they run on any Linux distribution (Ubuntu, Debian, Raspberry Pi OS) without managed library dependencies.

### Interactive Setup

On your very first run, AssistClaw will automatically launch an interactive onboarding wizard to configure your preferred LLM provider (Anthropic, OpenAI, or a local provider like Ollama). It will securely save your API keys and automatically generate your `~/.assistclaw` workspace.

If you want to manually run the onboarding wizard again at any time, run:

```bash
assistclaw onboard
```

### Verify the Installation

Check the CLI and ensure the providers load correctly:

```bash
assistclaw --version
assistclaw providers list
```

---

## Basic Usage

### Interactive Agent Session
Start an interactive chat session with the assistant:
```bash
assistclaw agent
```

### Single Message Command
Fire off a single prompt to the assistant:
```bash
assistclaw agent -m "Summarize the capabilities of sqlite-vec"
```

### Memory Management
Query your episodic memory log across past sessions:
```bash
assistclaw memory search "sqlite-vec"
```

### Start the Gateway Server
Start the HTTP REST and WebSocket control plane for connecting external chat clients:
```bash
assistclaw gateway start
```

---

## Building from Source

If you wish to modify AssistClaw or build extensions:

1. Install **Go 1.24+**
2. Clone the repository: `git clone https://github.com/hridesh-net/AssistClaw.git`
3. Build the binary: `make build`
4. Use it directly: `./bin/assistclaw`

---

## Troubleshooting & Logs

To view verbose debug information, including exact LLM requests and memory append operations, run any command with the `--log-level debug` flag:

```bash
assistclaw agent --log-level debug
```

---

## Uninstallation

To completely remove AssistClaw from your system, follow these steps to delete the binary and your local state data.

### 1. Remove Executable
Find where the AssistClaw binary is located:
```bash
which assistclaw
```
Remove the file (use `sudo` if installed in a system directory like `/usr/local/bin`):
```bash
# macOS / Linux
rm $(which assistclaw)
```

### 2. Wipe Configuration & State
Delete the state directory. **WARNING**: This action is irreversible and will permanently delete your conversation history, memories, and saved tools.

By default, data is stored in `~/.assistclaw`. If you have configured a custom path via `ASSISTCLAW_STATE_DIR`, remove that directory instead.

```bash
# macOS / Linux
rm -rf ~/.assistclaw

# Windows (PowerShell)
Remove-Item -Recurse -Force "$env:USERPROFILE\.assistclaw"
```

---

*AssistClaw — The Autonomous Edge Intelligence System.*
