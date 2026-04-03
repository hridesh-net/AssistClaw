# Changelog

All notable changes to this project will be documented in this file.

## [v3.10.3] - 2026-04-03
### Added
- **Onboarding — Bedrock models:** `ListFoundationModels` (AWS SDK `service/bedrock`) for **on-demand text** models in the chosen region, merged with the built-in catalog; embedding models listed the same way when the primary LLM is Bedrock.

### Fixed
- **`assistclaw onboard` re-runs:** Pre-fill Plano (no variable shadowing), gateway `bind` normalization (`tailnet` → wizard values), WhatsApp session id (stop overwriting saved), embeddings YAML **`default_model`** (was `model`), Ollama embedding base URL reset, skills multi-select from **`enabled_skills`**, and **“Current — …”** select rows for saved model IDs not in short lists.
- **Bedrock wizard:** Restore region and infer auth mode (API key / profile / IAM) from existing config instead of forcing `us-east-1` and blank auth.

### Changed
- **Config preload:** Together, NVIDIA, Cohere, Hugging Face, and Voyage primary/secondary fields load from existing `assistclaw.yaml`.
- **Bedrock provider:** Shared **`loadAWSConfig`** in `aws_config.go` used by `New()` and foundation-model listing.

## [v3.10.2] - 2026-04-02
### Fixed
- **Messaging slash commands:** Detect the first line that starts with `/` (after optional BOM/bidi marks), including when bridges prepend metadata (e.g. `[CHAT INFO]…`) or WhatsApp puts quoted reply text above `/new`. Previously routing required the entire `msg.Text` to start with `/`, so commands were ignored and handled by the LLM instead.
- **Fallback text:** If `Message.Text` is empty, derive slash detection from the first text **Part**.

### Changed
- **System prompt:** Reinforce **AssistClaw** product identity in Critical Rules (do not call the product **OpenClaw** unless workspace identity files say so).

## [v3.10.1] - 2026-04-05
### Added
- **Messaging slash commands (OpenClaw parity layer):** `/whoami` (`/id`), `/context`, `/compact`, `/model`, `/models [filter]`, `/tools verbose`, `/new`; **`WithModelRegistry`** so `/models` lists the live catalog on channels.
- **OpenClaw command names** (`/stop`, `/usage`, `/think`, `/config`, `/steer`, `/export`, …) return short stubs pointing to AssistClaw equivalents (yaml, restart, `/reset`, logs).
- **Aliases:** `/thinking`→`/think`, `/t`, `/v`, `/reason`, `/elev`, `/tell`→`/steer`, `/export`→`/export-session`, `/plugin`→`/plugins`.

### Changed
- **System prompt + templates:** Clarify state-dir vs `workspace/public/`, and require **persisting** identity/user facts to **IDENTITY.md** / **USER.md** with tools (not chat-only).

## [v3.10.0] - 2026-04-04
### Added
- **Provider model catalogs** (`internal/provider/catalogs/`): OpenClaw-aligned static lists for **xAI (Grok)**, **Groq**, **Mistral**, **Together**, **OpenRouter**, **NVIDIA**, **Cohere**, **DeepSeek**, and **Perplexity**, merged with live `/v1/models` discovery when enabled.
- **xAI default model**: `providers.xai.default_model` falls back to **`grok-4`** when unset.

### Fixed
- **OpenAI-compatible requests**: Avoid panics when a message has **empty** `content` parts; handle **tool results** before generic content shaping.
- **Provider registry**: **Deterministic** resolution when the same model id exists on multiple providers (sorted provider names); **sorted** `ListModels()` output.

### Changed
- **OpenAI-compat validation**: With discovery enabled, unknown model IDs are **allowed** after catalog lookup (vendors add models faster than static lists).

## [v3.9.9] - 2026-04-03
### Fixed
- **Bedrock parallel tool calls**: Converse/ConverseStream now merges consecutive tool-result turns into **one** user message containing all `tool_result` blocks. Previously one message per tool violated the Bedrock contract after a multi-`tool_use` assistant turn and triggered `ValidationException: Expected toolResult blocks at messages.*.content for the following Ids: ...`.

## [v3.9.8] - 2026-04-02
### Added
- **Sub-agents (OpenClaw-style delegation)**: New tools **`subagent_create`**, **`subagent_list`**, **`subagent_run`**, **`subagent_remove`**. Each specialist gets a workspace under `<stateDir>/subagents/<id>/` with **`SOUL.md`**, a JSON registry at **`subagents/registry.json`**, and a child runner that shares the parent tool stack but **cannot nest** further sub-agents. Wired after the security layer so children inherit guardrail and audit logging.
- **Tool graph**: Intent and edges for delegation keywords (`subagent`, `delegate`, `specialist`, etc.) so the catalog surfaces sub-agent tools when relevant.
- **Channel command `/subagents`**: Lists registered sub-agents via the same path as `/skills` (executes `subagent_list`).
- **Local static dashboards**: Gateway serves **`~/.assistclaw/workspace/public/`** at **`/workspace/`** (no Bearer token—do not store secrets there). **`Config.PublicGatewayBaseURL()`** supplies links for prompts; workspace init creates **`workspace/public`**; README and **`AGENTS.md`** template document usage.

### Changed
- **Template `IDENTITY.md`**: Avatar example path now references **`assistclaw.png`** instead of OpenClaw.

## [v3.9.7] - 2026-04-02
### Added
- **Heartbeat scheduler**: Optional `agent.heartbeat` config (interval, dedicated session, prompt) for OpenClaw-style periodic proactive turns when running `assistclaw start` or any messaging channel.
- **`HEARTBEAT.md` workspace template** for first-time workspace seeding.
- **Smarter autonomous mode**: Upfront planning phase in `RunAutonomous` when planning is enabled; checkpoint system messages every 25 iterations on long runs.
- **Agent planning / reflection YAML**: `agent.planning` (default on when unset) and `agent.reflection` (default off when unset) wired into the runner for interactive and background sessions.

### Fixed
- **Channel sessions missing security**: `WithSession` now copies guardrail, audit log, and command map so Telegram/Discord/Slack/WhatsApp use the same enforcement as the main runner.
- **Cron jobs missing catalog and security**: Scheduled jobs now clone the fully configured gateway runner (tool graph, guardrail, audit) instead of a minimal runner.

## [v3.9.6] - 2026-04-01
### Fixed
- **Workspace Templates Actually Written Now**: The root cause of templates never appearing on disk was `InitializeWorkspace` being called with the raw `--config` flag value which is an empty string when the flag isn't passed. `filepath.Dir("")` resolves to `"."` (current directory), not `~/.assistclaw`. Now uses `cfg.StateDir` (the already-resolved state directory) to guarantee templates land in the correct location.
- **Agent Persona Now Properly Overridden**: `SOUL.md`, `IDENTITY.md`, `AGENTS.md`, `USER.md` now become the primary identity block when present, replacing the hardcoded fallback text instead of being appended below it where they had no effect.

## [v3.9.5] - 2026-04-01
### Fixed
- **Workspace Identity Now Active**: Fixed two critical bugs that prevented `SOUL.md`, `IDENTITY.md`, `AGENTS.md`, `USER.md` etc. from having any effect:
  1. `InitializeWorkspace` was defined but never called — templates were never seeded to disk on first run. Now called at startup.
  2. Workspace identity files were appended *after* the hardcoded `"You are AssistClaw"` block, effectively ignoring them. Now when workspace files exist they *replace* the hardcoded identity (exact OpenClaw parity). The hardcoded block is only a fallback for bare installs.

## [v3.9.4] - 2026-04-01
### Fixed
- **Bedrock Validation Exception**: Handled edge cases where AWS Bedrock `ConverseStream` crashes with `ValidationException (The value at messages...toolUse.input is empty)` if an LLM calls a parameter-less tool (e.g., `skill_graph_index`) and returns a blank JSON payload. Strict coercion to an empty map `{}` prevents this strict Bedrock schema error.

## [v3.9.3] - 2026-04-01
### Added
- **Dynamic Workspace Bootstrapping**: Added logic to automatically seed the `~/.assistclaw` state directory with standard OpenClaw markdown templates (`SOUL.md`, `IDENTITY.md`, `AGENTS.md`, `USER.md`, `BOOTSTRAP.md`, `TOOLS.md`) via Go embedding if they do not exist.

## [v3.9.2] - 2026-04-01
### Fixed
- **Bedrock New Models Registration**: Bypassed strict local catalog checking in `ValidateModel` to allow any valid AWS Bedrock model IDs prefix (e.g. `qwen.*`, `meta.*`, `amazon.*`) so that newly available AWS models (like Qwen 3 Coder 30B) don't trigger "model not found" errors before the CLI is updated.
- Added `qwen.qwen3-coder-30b-a3b-v1:0` and `qwen.qwen3-235b-a22b-2507-v1:0` to officially listed models.

## [v3.9.1] - 2026-04-01
### Added
- **OpenClaw Parity**: Full integration of the workspace memory system mapping `SOUL.md`, `IDENTITY.md`, `USER.md`, and `AGENTS.md` exactly as OpenClaw handles injection.
- **Security Profile Controls**: Implementation of the `tools.profile` setting mapping string options like `coding` to strictly filter agent tool registries (e.g. blocking web browsing or proactive messaging for clean sandbox execution).
- **Persistent Cron CLI**: Complete the `cron` command package mapping OpenClaw's exact command capability (`list`, `add`, `remove`) and background heartbeat/headless execution system natively over Go logic.

## [v3.9.0] - 2026-03-25
### Added
- **Continuous Autonomous Mode**: Introduced a new background agent loop (`RunAutonomous`) capable of pursuing complex, multi-step goals endlessly without stopping after a single response.
- **Finish Task Tool**: Added `finish_task` to the standard tool catalog, granting the LLM explicit control over when an overarching objective is completed.
- **`assistclaw auto` CLI Command**: Launch continuous background workflows directly from the terminal.
- **`/auto` Chat Command**: Spawn continuous autonomous background tasks straight from Telegram, Discord, Slack, and WhatsApp.

## [v3.8.1] - 2026-03-13
### Fixed
- **Onboarding Formatting**: Fixed a YAML indentation error in the `plano` configuration generator.
- **Documentation**: Finalized changelog entries for v3.7.x and v3.8.0.

## [v3.8.0] - 2026-03-12
### Added
- **Inbuilt Voice Support**: Voice features (STT/TTS) are now enabled by default. AssistClaw will automatically set up the internal voice microservice on first run without requiring manual configuration.
- **Improved Portability**: Voice daemon now resolves script paths relative to the executable, allowing it to run from any directory.

## [v3.7.3] - 2026-03-12
### Fixed
- **WhatsApp Empty Message**: Resolved an issue where voice notes on WhatsApp resulted in blank responses due to gaps in OpenAI provider modality handling.
- **Graceful Audio Fallback**: OpenAI provider now handles audio parts as text placeholders if transcription is unavailable, preventing agent crashes.

## [v3.7.2] - 2026-03-12
### Fixed
- **WhatsApp Audio Transcription**: Corrected audio format detection (OGG/MP4) to ensure voice notes are transcribed successfully by Whisper.
- **Empty Message Fallback**: Added "[Audio Message]" placeholder for any voice notes where transcription might fail, preventing agent errors.
- **Discord Voice Transcription**: Updated live audio processing to match the improved server-side STT signature.

## [v3.7.1] - 2026-03-12
### Fixed
- **WhatsApp Response Fragmentation**: Fixed an issue where the agent would send every token as a separate message. Responses are now correctly buffered and grouped into sentences.
- **Group Chat JID Handling**: Corrected message routing in WhatsApp groups to ensure replies are sent to the conversation thread, not just the sender's private DM.

## [v3.7.0] - 2026-03-12
### Added
- **Native Voice Support**: Integrated local STT (Whisper) and TTS (VoxCPM) via a managed Python microservice.
- **WhatsApp Voice Conversations**: Automated transcription of voice notes and AI-synthesized voice replies for continuous hands-free interaction.
- **Live Discord Voice interaction**: Real-time listening and speaking in Discord voice channels. Use `!join` and `!leave` to control the agent.
- **Advanced Automation**: Added Webhook support for external triggers and Gmail Pub/Sub integration for real-time email-based workflows.
- **Embedded Voice Management**: Automated Python environment setup and service lifecycle management within the AssistClaw binary.

## [v3.6.18] - 2026-03-07
### Added
- **Persistent Cron & Background Tasks**: Scheduled jobs and background processes now survive agent restarts via JSON persistence (`cron_jobs.json`, `processes.json`).
- **Static Cron Support**: Permanent scheduled tasks can now be defined directly in `assistclaw.yaml`.
- **Background Daemon Mastery**: Full integration of the Cron daemon into the agent's core startup sequence.

## [v3.6.17] - 2026-03-07
### Added
- **Hardware Sensing & Registration**: Automatic detection of cameras, microphones, and USB input devices on macOS.
- **`list_hardware` Tool**: Enables the agent to perform on-demand hardware re-scans.
- **Sensory Awareness**: Hardware details are now injected into the agent's system prompt for immediate relative awareness.

## [v3.6.16] - 2026-03-07
### Added
- **WhatsApp Parity (OpenClaw)**: Support for WhatsApp emoji reactions (⏳, ✅, ❌) for status updates and shared group context via JID-to-SessionID mapping.
- **Multimodal Vision Integration**: Support for `image_understand` tool (OCR, screenshot analysis) across all vision-capable providers.
- **Session Intelligence**: Added `/sessions` and `/forget` commands for granular chat history management.
- **Proactive Media Support**: Added `SendMediaTool` for autonomous file and image dispatch.
### Added
- **Chat Commands**: Optimized agent overhead by introducing slash commands for administrative tasks. Supported: `/reset`, `/status`, `/skills`, `/help`.

## [v3.6.14] - 2026-03-07
### Added
- **Self-Healing Startup**: AssistClaw now automatically checks and repairs dependencies for all enabled skills during the agent's boot sequence. This ensures existing skills become active without manual intervention.

## [v3.6.13] - 2026-03-07
### Added
- **Automated Skill Dependency Management**: The `assistclaw skills add` and `skills install` commands now automatically detect missing system dependencies (binaries) and attempt to install them (supporting `apt`, `brew`, `npm`, `pip`, `go`).
- **CLI Overhead Control**: Added a `--no-deps` flag to skills commands to bypass automatic installation if desired.

## [v3.6.12] - 2026-03-06
### Fixed
- **Streaming Responsiveness**: Optimized `StreamingBuffer` to prevent "timer starvation" where tokens were delayed indefinitely during continuous streaming.
- **WhatsApp Latency**: Shortened flush timeout to 500ms and restored real-time tool activation markers (e.g., `[🛠️ Activating bash...]`) for immediate user feedback.

## [v3.6.11] - 2026-03-06
### Added
- **Self-Healing Skills**: Introduced `repair_skill` tool, allowing the agent to automatically install missing system dependencies (via `apt`, `brew`, `npm`) when requested.
- **Maintenance Transparency**: Added visible `[Maintenance: Compacting session memory...]` indicator during silent memory flush turns.

### Changed
- **Context Optimization**: `skill_graph_index` now filters out skills with missing dependencies to reduce token bloat and prevent the agent from attempting to use broken tools.

## [v3.6.10] - 2026-03-06
### Fixed
- **WhatsApp Message Size**: Implemented 4000-character chunking in the WhatsApp channel to prevent silent message dropping by the protocol for large payloads (e.g., full skill indexes).

## [v3.6.9] - 2026-03-06
### Fixed
- **CI/CD Pipeline**: Fixed a regression in GitHub Actions where `ldflags` were omitted during binary compilation, resulting in "dev" version strings instead of git tags.

## [v3.6.8] - 2026-03-06
### Fixed
- **Version Management**: Refactored `cmd/assistclaw/main.go` to remove hardcoded version strings in favor of build-time LDFlag injection.

## [v3.6.4] - 2026-03-03
### Fixed
- Fixed an issue where OpenAI-compatible API endpoints that inherently require a `/v1` suffix (like `api.x.ai/v1`) were double-appending the `/v1` path (resulting in `/v1/v1/chat/completions`), leading to 404 stream timeouts. The router is now smarter about appending versions.

## [v3.6.3] - 2026-03-03
### Fixed
- Fixed an issue where selecting `xai` as a provider during onboarding did not prompt the user for an API key.
- Fixed 404 errors when using `xai` models by correcting the API endpoint base path sent to the compatibility router.

## [v3.6.2] - 2026-03-03
### Fixed
- **Hotfix:** Resolved a startup panic on Linux/macOS caused by an unsupported Perl-style regex lookahead `(?!` in the generic PII detection module (SSN matching).

## [v3.6.1] - 2026-03-03
### Added
- Added native support for xAI (Grok) models (`grok-4-latest`, `grok-beta`, `grok-vision-beta`, `grok-2`).

### Fixed
- Fixed an `assistclaw onboard` issue where old provider configuration variables (like API keys or bedock regions) leaked and overwrote the config file when switching providers in the interactive CLI.
- Fixed an issue causing a 404 error when selecting Groq due to an incorrect BaseURL API path endpoint.
- Channel selectors in `onboard` will now accurately display `[Configured]` if they are already integrated in `assistclaw.yaml`.

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
