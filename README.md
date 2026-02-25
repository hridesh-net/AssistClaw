<p align="center">
  <img src="https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/docs/assets/assistclaw-logo.png" alt="AssistClaw" width="200">
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

**AssistClaw** is a polyglot, production-grade AI assistant system written in **Go**. It is designed for low-latency, high-concurrency edge computing, extending the "Claw" philosophy with hardware sensing, autonomous tool generation, and a modular architecture that runs anywhere from a Raspberry Pi 5 to a high-end workstation.

## 🚀 Key Highlights

- **⚡ Native Performance**: Built in Go for near-instant startup, low memory footprint, and true multi-core concurrency.
- **🧠 Three-Tier Memory**: Local, zero-infrastructure memory system:
  - **Working**: Context-aware RAM.
  - **Episodic**: SQLite FTS5 for fast historical recall.
  - **Semantic**: `sqlite-vec` for native vector similarity search.
- **🛠️ Autonomous Intelligence**: The agent can autonomously write, validate, sandbox-execute, and persist its own Python tools.
- **📡 Multi-Channel Connectivity**: Native support for WhatsApp, Telegram, Discord, Slack, and a high-performance REST/WebSocket Gateway.
- **🦾 Hardware Sensing**: Integrated C++ layer for Camera (OpenCV), Audio (PortAudio), and GPIO (pigpio) over NDJSON IPC.
- **🎛️ Background Daemon**: Runs as a persistent service (`v3.1.0+`) with dedicated `start`, `stop`, and `status` management.

---

## 🦞 The Claw Ecosystem

AssistClaw is part of a broader lineage of "Claw" projects. Here is how it compares to its cousins:

| Feature | OpenClaw | NanoClaw | ZeroClaw | **AssistClaw** |
| :--- | :--- | :--- | :--- | :--- |
| **Language** | Python / Node.js | TypeScript | Rust | **Go (Golang)** |
| **Footprint** | Heavy (~1.5GB RAM) | Minimal | Extreme Low (~5MB) | **Balanced (~40MB)** |
| **Core Focus** | Breadth & Extensions | Security & Minimalism | Performance Framework | **Edge Intelligence** |
| **Tooling** | Managed Skills | Claude SDK only | Trait-based Rust | **Autonomous Python** |
| **Hardware** | Basic | None | None | **Native C++ Bridge** |

> **Why AssistClaw?** If you need the integration breadth of OpenClaw but with the performance profile of ZeroClaw and the ability to sense the physical world (GPIO/Camera) natively, AssistClaw is the choice.

---

## 📖 Documentation

- **[Master Guide](doc/master_guide.md)**: Full architecture and logic overview.
- **[The Autonomous Execution Loop](doc/deep-dives/execution-loop.md)**: How the agent thinks and acts.
- **[Dynamic Skills & Tools](doc/deep-dives/skills-and-tools.md)**: Extending the assistant.
- **[V3.1.x Walkthrough](file:///Users/elrosshinzo/.gemini/antigravity/brain/f0edca1f-cfa9-476d-9eed-2c3f7b3f74e7/walkthrough.md)**: Latest daemon and stability updates.

---

## 🛠️ Installation

### Automated (Linux / macOS)
```bash
curl -fsSL https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.sh | bash
```

### Manual (All Platforms)
1. Download the latest binary from [GitHub Releases](https://github.com/hridesh-net/AssistClaw/releases).
2. Move to your PATH and ensure it is executable.
3. Run `assistclaw onboard` to start the interactive setup.

---

## ⌨️ Basic Commands

- `assistclaw agent`: Start the interactive UI (or hatch into a running daemon).
- `assistclaw start --daemon`: Launch the assistant in the background.
- `assistclaw stop`: Safely shut down the background agent.
- `assistclaw status`: Check if the agent is running and its PID.

---

## 🏗️ Build from Source

```bash
git clone https://github.com/hridesh-net/AssistClaw.git
cd AssistClaw
make build
```

---

*AssistClaw — The Autonomous Edge Intelligence System.*
