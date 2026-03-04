# Changelog

All notable changes to this project will be documented in this file.

## [v3.6.0] - 2026-03-03
### Added — Runtime Security Layer
**I/O Safety Guardrail** (`internal/security/guardrail.go` + `pii.go`)
- Pre-check on every user input: detects 14 prompt injection patterns (jailbreaks, DAN, `[INST]`, `<<SYS>>`, etc.) at HIGH/MEDIUM/LOW severity
- Tool-check before every tool execution: blocks dangerous bash patterns (`rm -rf /`, `/etc/shadow`, fork bombs, pipe-to-shell), and system file paths
- Post-check on LLM output: detects PII (email, phone, SSN, credit cards via Luhn algorithm, OpenAI/Anthropic/AWS/GitHub API keys) and exfiltration patterns
- 3 configurable modes: `monitor` (log only, default), `enforce` (block HIGH), `strict` (block MEDIUM+HIGH)
- PII masking: replaces detected PII with `[REDACTED:<type>]` when `security.pii_mask: true`

**Tamper-Evident Audit Log** (`internal/security/audit.go` + `verify.go`)
- Every tool call, skill node read, skill index call, and guardrail event logged to `~/.assistclaw/security/audit.ndjson`
- HMAC-SHA256 hash chain: each entry includes `prev_hash` + its own `entry_hash = HMAC(secret, prev_hash+content)` — tampering breaks the chain
- Machine-scoped HMAC secret auto-generated at first run (`audit.secret`, mode 0600)
- Input/output stored as SHA-256 hashes only — no plain-text data in audit log

**`assistclaw security` Subcommand**
- `security status` — guardrail mode, PII masking, log path, size, event count
- `security verify` — verifies full HMAC chain; prints first violation location if tampered
- `security report` — summary by event type, top tools, skill node reads, actor activity, guardrail counts
- `security tail` — streams live events (`tail -f` style)

**Config** (`assistclaw.yaml`):
```yaml
security:
  mode: enforce          # monitor | enforce | strict
  pii_mask: true
  block_patterns: []     # custom regex patterns
```

## [v3.5.1] - 2026-03-03
### Fixed — Web Search & Fetch
- **`web_search`**: Root cause confirmed — DDG HTML returns a CAPTCHA challenge for server-side Go HTTP requests, and the Instant Answer API only works for Wikipedia-level entities (not niche queries like "openclaw"). Rewrote to use **SearXNG public JSON API** as primary backend (12 instances tried in order, 5s timeout each, 20s overall). Falls back to DDG Instant Answer, then returns a direct search URL for `web_fetch` as a last resort.
- **`web_fetch`**: Upgraded from bare `curl` wrapper to a proper tool — adds `max_chars` (default 8000, max 32000), `raw` param, and automatic HTML tag stripping. Better description tells the agent to use it after `web_search`. Pairs cleanly as "search → fetch" workflow.

## [v3.5.0] - 2026-03-03
### Added — Graph-First Context Engineering (~66% token reduction)
- **`internal/graph/tool_graph.go`** — Weighted directed ToolGraph with typed edges (`COMPANION`, `PREREQUISITE`, `DOMAIN`, `FALLBACK`), BFS traversal from keyword intent-matched seed nodes, and session inertia (recently-used tools boost their neighbours next turn). No embeddings required.
- **`internal/graph/skill_graph.go`** — SkillGraph with typed wikilink edges (`ENTRY`, `EXTENDS`, `REQUIRES`, `EXAMPLE`, `TOOL`), wikilink parser, compact index generator, and formatted edge output for natural graph traversal.
- **`internal/tools/catalog.go`** — `Catalog` wraps all registered tools and exposes `SelectForRequest(query, providerCaps)` — returns only the core 8 tools + up to 6 graph-traversed relevant tools per request instead of all 21.
- **`internal/tools/find_tool.go`** — `find_tools` agent tool: discovers tools by keyword, returns plain text — works with any LLM including those without native tool-use support.
- **`internal/provider/caps.go`** — `ProviderCaps` + `CapsFor()`: provider capability profiles for Anthropic, OpenAI, AWS Bedrock, Ollama, Gemini/Vertex. Bedrock gets all tools upfront; others get graph-filtered subset.
- **`internal/skills/graph_index.go`** — `skill_graph_index` agent tool: returns compact wikilink-format index of all active skill nodes on demand.
### Changed
- **`BuildContext()`** slimmed from full XML node-summary blob (~800 tokens) to a 3-line header (~40 tokens). Agent discovers nodes via `skill_graph_index()`.
- **`read_skill_node`** now appends outgoing `[[wikilink]]` edges after the node body so the agent can traverse the graph without an extra index call.
- **`runner.buildRequestV3()`** now uses `Catalog.SelectForRequest()` for per-request tool filtering. Falls back to `tools.Definitions()` if no catalog configured (backward compatible).
- **`runner.executeTool()`** calls `Catalog.RecordUsage()` to update session inertia weights after each tool call.
- **`agent.Config`** gains `ProviderName` field for capability detection.
### Token Budget (5 active skills, 21+ tools)
| | Before | After |
|--|--------|-------|
| Tool schemas/turn | ~1,050 tokens | ~550 tokens |
| Skill context | ~800 tokens | ~40 tokens |
| **Total overhead** | **~2,150 tokens** | **~730 tokens** (~66% ↓) |

## [v3.4.2] - 2026-03-03
### Added
- **21 built-in tools** (up from 10) — full OpenClaw tool parity:
  - **Tier 1**: `edit` (str-replace in-place edits), `web_search` (DuckDuckGo, no API key), `process` (start/stop/logs background processes), `apply_patch` (unified diff), `env` (read/write .env + OS env)
  - **Tier 2**: `image_understand` (vision model for screenshots/OCR), `sessions_list` + `sessions_history` (episodic memory browsing), `cron` (schedule recurring tasks), `message` (proactive channel sends)
- `tools.Default()` now accepts episodic memory, vision provider, and channel senders for full wiring
- `robfig/cron` v3 dependency for the cron scheduler

## [v3.4.1] - 2026-03-03
### Changed
- **Daemon auto-starts after onboarding**: `assistclaw onboard` now launches the background daemon automatically on completion (no "Start now?" confirm) and displays a launch summary banner with the Web UI URL and token.
- **`assistclaw gateway` is now a subcommand group**:
  - `gateway start` — start daemon in background (web UI + agent + channels)
  - `gateway stop` — stop the running daemon
  - `gateway restart` — restart the daemon
  - `gateway status` — show PID and status
  - `gateway serve` — foreground mode (blocks terminal, for dev/debug)

## [v3.4.0] - 2026-03-03
### Added
- **Embedded Web UI**: Full dark-themed chat interface (`/`) served directly from the binary via `go:embed` — no external build step. Supports SSE streaming, tool-call indicators, session management, and Bearer token auth.
- **`/api/chat` SSE endpoint**: Gateway now streams agent responses token-by-token over Server-Sent Events for low-latency web interaction.
- **`/api/status` endpoint**: Returns JSON with version, PID, and active model.
- **`assistclaw service` command**: Register AssistClaw as a system service that auto-starts on login and survives reboots.
  - macOS: `~/Library/LaunchAgents/com.assistclaw.agent.plist` via `launchctl` (`KeepAlive: true`)
  - Linux: `~/.config/systemd/user/assistclaw.service` via `systemctl --user`
  - `service install` / `service uninstall` / `service logs`
- **Onboarding parity with OpenClaw**: Added NVIDIA NIM, Together AI, HuggingFace, OpenRouter (now primary), and Cohere to the provider list (17 providers total).
- **Auto-start on login prompt**: `assistclaw onboard` now asks "Start automatically on login?" and runs `service install` if confirmed.
- **LAN mode**: Gateway bind `lan` exposes the web UI on `0.0.0.0` for device-local network access.

## [v3.1.6] - 2026-02-25
### Added
- MIT License.
- Contributors gallery in README.
- `CHANGELOG.md` and `CONTRIBUTING.md`.

## [v3.1.5] - 2026-02-25
### Added
- "Verified & Working" section in README (AWS Bedrock, WhatsApp).
- Detailed technical comparison with the "Claw" ecosystem (OpenClaw, NanoClaw, ZeroClaw).
- Highlighted unique Go-native features (Concurrency, Tool Factory, C++ Bridge).

## [v3.1.4] - 2026-02-25
### Changed
- Major README overhaul with premium branding and status badges.
- Improved project identity as "Autonomous Edge Intelligence System".

## [v3.1.3] - 2026-02-25
### Fixed
- Increased WhatsApp pairing stabilization delay to 10s for large accounts.
- Suppressed noisy `Error sending close: EOF` warnings in WhatsApp channel.

## [v3.1.2] - 2026-02-25
### Fixed
- WhatsApp reliability: Switched to SQLite WAL mode and added redundant connection bypass during onboarding.

## [v3.1.1] - 2026-02-25
### Fixed
- Windows compatibility: Refactored daemon logic and added non-CGO `sqlite-vec` stubs.

## [v3.1.0] - 2026-02-25
### Added
- **Daemon Mode**: Background service management with `start`, `stop`, `status`.
- Detached onboarding flow.

## [v3.0.2] - 2026-02-25
### Fixed
- WhatsApp pairing persistence stability.

## [v3.0.0] - 2026-02-24
### Added
- **Major Intelligence Release**: Plan-Execute-Reflect loop.
- Semantic Memory (Lessons Learned) with vector search.
- Native multi-channel security and access control.
- Universal embedding support.

---
*For older releases, see [GitHub Tags](https://github.com/hridesh-net/AssistClaw/tags).*
