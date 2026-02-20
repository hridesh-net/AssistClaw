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

---

## Quick Installation

AssistClaw distributes highly-optimized pre-compiled binaries for **Linux**, **macOS**, and **Windows**. You do not need Go installed on your machine.

### Linux / macOS
Run the automated installation script to download the latest binary for your architecture and set up your workspace:

```bash
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.sh | bash
```

### Windows
Open PowerShell as Administrator and run:

```powershell
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.ps1" -OutFile "install.ps1"; .\install.ps1
```

Once complete, the `assistclaw` command will be available in your PATH, and your configuration workspace will be generated automatically at `~/.assistclaw/`.

### 4. Configuration

AssistClaw uses a YAML configuration file to manage API keys, routing, channels, and working memory constraints. 

1. Create the application directory:
   ```bash
   mkdir -p ~/.assistclaw
   ```
2. Copy the template configuration file:
   ```bash
   cp .assistclaw.yaml.example ~/.assistclaw/assistclaw.yaml
   ```
3. Open `~/.assistclaw/assistclaw.yaml` and add your desired API keys (e.g., Anthropic, OpenAI) and set your default routing model. Local providers like Ollama will work immediately out-of-the-box on `http://127.0.0.1:11434` without an API key.

### 2. Verify the Installation

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
./bin/assistclaw agent --log-level debug
```
