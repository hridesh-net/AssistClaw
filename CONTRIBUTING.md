# Contributing to AssistClaw

Thank you for your interest in AssistClaw! We welcome contributions to make this edge intelligence system even better.

## 🛠️ Development Environment

### Prerequisites
- **Go**: 1.24+
- **Python**: 3.10+ (for the Autonomous Tool Factory)
- **CGO**: Required for `sqlite-vec` and hardware sensing (OpenCV, PortAudio, pigpio).
- **Libraries**: `libsqlite3-dev` (Linux) or `sqlite` (Homebrew/macOS).

### Recommended Setup
1. Clone the repo: `git clone https://github.com/hridesh-net/AssistClaw.git`
2. Install dependencies: `go mod tidy`
3. Build the binary: `make build`

## 🏗️ Architecture Layout

- `cmd/assistclaw/`: CLI entrypoints and daemon management.
- `internal/agent/`: Core reasoning loop (Planning, Execution, Reflection).
- `internal/channels/`: Integration layers (WhatsApp, Telegram, Discord, Slack).
- `internal/provider/`: LLM client implementations.
- `internal/memory/`: Semantic (vector) and Episodic (FTS5) persistence.
- `bridge/`: C++ layer for hardware sensing (Camera, GPIO).

## 🧪 Coding Standards

- **Go**: Follow standard `gofmt` and `golangci-lint` patterns.
- **Privacy**: Never log raw API keys or user message content unless `DEBUG` is explicitly enabled.
- **Performance**: Use buffered channels and non-blocking I/O for hardware-sensing loops.

## 🚀 Pull Request Process

1. Create a descriptive branch: `feat/new-sensing-protocol` or `fix/whatsapp-timeout`.
2. Ensure `go build ./...` passes.
3. Update `CHANGELOG.md` under the `[Unreleased]` section.
4. Submit your PR and describe the testing steps (e.g., "Tested on RPi 5 with Bedrock").

---

*AssistClaw — Empowering Edge Intelligence Together.*
