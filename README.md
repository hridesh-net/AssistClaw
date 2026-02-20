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

## Linux Setup & Installation Guide

Follow these steps to build and setup AssistClaw from source on a new Linux system.

### 1. Prerequisites

You will need the following installed on your system:
- **Go 1.22+** (for compiling the core orchestration binary)
- **Python 3.11+** & `venv` module (for sandboxing auto-generated tools)
- *(Optional)* **CMake, OpenCV, PortAudio** (if you intend to build the C++ hardware sensing components)

On Debian/Ubuntu, you can install the optional sensing dependencies via:
```bash
sudo apt update
sudo apt install build-essential cmake libopencv-dev portaudio19-dev python3.11-venv
```

### 2. Clone the Repository

```bash
git clone https://github.com/hridesh-net/AssistClaw.git
cd AssistClaw
```

### 3. Build the Core Binary

Compile the main Go orchestrator:

```bash
make build
```
*(Alternatively: `go build -o bin/assistclaw ./cmd/assistclaw`)*

This creates a self-contained binary at `./bin/assistclaw`.

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

### 5. Verify the Installation

Check the CLI and ensure the providers load correctly:

```bash
./bin/assistclaw --version
./bin/assistclaw providers list
```

---

## Basic Usage

### Interactive Agent Session
Start an interactive chat session with the assistant:
```bash
./bin/assistclaw agent
```

### Single Message Command
Fire off a single prompt to the assistant:
```bash
./bin/assistclaw agent -m "Summarize the capabilities of sqlite-vec"
```

### Memory Management
Query your episodic memory log across past sessions:
```bash
./bin/assistclaw memory search "sqlite-vec"
```

### Start the Gateway Server
Start the HTTP REST and WebSocket control plane for connecting external chat clients:
```bash
./bin/assistclaw gateway start
```

---

## Troubleshooting & Logs

To view verbose debug information, including exact LLM requests and memory append operations, run any command with the `--log-level debug` flag:

```bash
./bin/assistclaw agent --log-level debug
```
