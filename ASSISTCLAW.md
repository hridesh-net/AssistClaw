# AssistClaw

AssistClaw is a polyglot, production-grade AI assistant system built primarily in Go. 
It takes inspiration from the OpenClaw feature set and extends it with hardware sensing, 
autonomous tool generation, full embedding model support, and a fast, concurrent Go-first architecture.

## Features

- **15+ LLM Providers**: Support for OpenAI, Anthropic, AWS Bedrock, Google Vertex AI, Ollama (local), vLLM, LM Studio, OpenRouter, Groq, Mistral, Together AI, Cohere, HuggingFace, and NVIDIA NIM.
- **5 Embedding Providers**: Semantic vector memory powered by OpenAI, Cohere, Google, Ollama, or HuggingFace embeddings.
- **Three-Tiered Memory**:
  - *Working Memory*: In-RAM session context with token-budget compaction.
  - *Episodic Memory*: SQLite FTS5 database for full-text search across conversation history.
  - *Semantic Memory*: Vector similarity search using sqlite-vec.
- **Autonomous Tool Generation**: The agent can write its own Python tools, validate them against safety policies (no `sudo`, no network exfil without permission), sandbox them in a disposable `venv`, and persist them for future use.
- **Hardware Integration (C++)**: High-performance, low-latency C++ processes for camera capture (OpenCV) and audio streaming (PortAudio) that communicate cleanly with the Go orchestrator via JSON streaming.
- **Multi-Model Routing**: YAML-based routing engine to direct specific tasks (e.g., coding, vision, summarization) to the most cost-effective or capable models.

## Architecture

At its core, AssistClaw is a **Go binary (`assistclaw`)**. 

- **Go**: The primary orchestrator, CLI, task router, and API gateway.
- **Python (Sandboxed)**: Used exclusively for running auto-generated tools and complex ML scripts in a secure virtual environment.
- **C++**: Optional high-performance sensing layer (Camera, Audio, GPIO).
- **TypeScript**: A minimal shim layer is included to run OpenClaw-compatible subagents using `@mariozechner/pi-agent-core` if desired, but is not required for the core Go assistant.

## Setup & Installation

The easiest way to install AssistClaw (including compiling the Go binary and setting up the Python sandbox) is using the installer script:

```bash
curl -fsSL https://raw.githubusercontent.com/assistclaw/assistclaw/main/install.sh | bash
```

Or clone the repo and build manually:

```bash
git clone https://github.com/assistclaw/assistclaw.git
cd assistclaw
make deps
make build      # Builds the Go binary
make install    # Moves binary to /usr/local/bin
```

## Configuration

AssistClaw uses a YAML configuration file, usually located at `~/.assistclaw/assistclaw.yaml`. Environment variables can override any setting (e.g., `ASSISTCLAW_OPENAI_API_KEY`).

Copy the template to get started:
```bash
cp .env.example .env
# Edit .env with your provider keys
```

## Usage

Start an interactive REPL session:
```bash
assistclaw agent
```

Send a single message:
```bash
assistclaw agent --message "What's the weather like?" --model anthropic/claude-3-5-sonnet-20241022-v2:0
```

List registered providers and available models:
```bash
assistclaw providers list
```

Search your conversation history using SQLite FTS5:
```bash
assistclaw memory search "docker commands"
```

## Creating Autonomous Tools

AssistClaw can build its own tools! Simply ask the agent:
> "Create a tool that fetches the top 5 HackerNews stories and formats them as markdown."

The agent will draft the Python code, the system will validate its safety (blocking dangerous imports), and then run it in an isolated Python `venv`. If it works, it is saved to `~/.assistclaw/tools/` for future use by the agent.
