# ZeroClaw (github.com/zeroclaw-labs/zeroclaw)

## What it is

ZeroClaw is "fast, small, and fully autonomous AI personal assistant infrastructure — any OS, any platform, deploy anywhere, swap anything." It is a Rust rewrite of the OpenClaw concept (a personal always-on AI agent reachable over chat channels), explicitly marketed as the zero-overhead alternative to OpenClaw (TypeScript/Node, >1GB RAM) and a sibling to PicoClaw (Go), Nanobot, and AgentZero. Philosophy: "You own the agent. You own the data. You own the machine it runs on," and "the model sees what you configure" (no hidden system prompts).

Rust edition 2024, dual MIT/Apache-2.0. ~32.4k stars, created February 2026 (~5.5 months old, explosive growth), extremely active, v0.8.3, mid "microkernel transition." RFC-based contributions, mdBook docs, ADRs. ZeroClaw is AssistClaw's closest comparable.

## Architecture

- Process model: single self-contained binary on a size-trimmed tokio runtime; runs as a user-level daemon (systemd user service + loginctl linger on Pi); SQLite (bundled rusqlite) for sessions/state. An attribution-propagating spawn! macro wraps tokio::spawn for structured tracing.
- Workspace: microkernel — src/ files are tiny re-export stubs; real code in 20 crates (zeroclaw-api, zeroclaw-runtime, channels, providers, tools, memory, gateway, plugins, hardware, config, tool-call-parser, sop-graph, eval, robot-kit...).
- Five core traits, all in one small API crate (zeroclaw-api): ModelProvider (chat/chat_with_history/chat_with_tools/stream_chat returning a stream of ToolCall/Usage/Final events, plus a ProviderCapabilities struct advertising native tool calling, vision, prompt caching, extended thinking), Channel (send + listen over an mpsc, with rich default methods: health_check, self-message guards, draft/update/finalize progressive streaming into chat apps, approval prompts over any channel, room creation), Tool, Memory (backend-neutral MemoryEntry with category core/daily/conversation/custom, score, importance), and Peripheral (hardware abstraction). Plugins implement the same traits as built-ins.
- Agent loop (zeroclaw-runtime): loop with dedicated modules for memory_inject, history_pruner/history_trim (context management), loop_detector (anti-repetition), safety_net, tool_execution, tool_receipts (HMAC-SHA256 cryptographic receipts per action), cost tracking with a pricing catalog, thinking, personality templates, classifier, context analyzer, dispatcher.
- Request lifecycle: channel adapter (decode, dedup, pair-check, IAM policy drop) → turn engine resolves memory-injection policy by initiator (user vs cron) → provider call streams tokens → mid-stream tool call pauses the loop → security gate (autonomy level + allow/deny lists) → block/approve/invoke → result appended with optional HMAC receipt → reply streamed back (chunked for Discord/Slack, flush-on-complete for email/SMS). Inbound messages carry a reserved internal event field so orchestrator routing can never be driven by user-controlled fields like subject or content.
- Gateway: axum + hyper (feature-gated); HTTP/WebSocket plus a web dashboard. Security defaults: binds 127.0.0.1 only, refuses 0.0.0.0 without a tunnel or explicit allow; pairing via 6-digit one-time code → bearer token; all webhook requests require Authorization.
- Plugins: WASM Component Model (wasm32-wasip2) on wasmtime; a plugin is a manifest.toml (capabilities and permissions) plus a .wasm component; capability-based sandboxing with zero ambient authority, fuel metering, memory/instance ceilings, typed WIT ABI, signature policies. Three swappable wasmtime strategies as cargo features, including the pulley interpreter specifically for ARM32.
- Orchestration beyond the loop: an SOP (Standard Operating Procedures) engine — event-triggered via cron/MQTT/webhooks/peripherals, with approval gates and resumable execution, as a separate layer from the chat agent loop. Also subagent spawning, markdown skill bundles, ACP (JSON-RPC over stdio) for IDE integration, heartbeat, doctor, tunnel.
- Config: single TOML at ~/.zeroclaw/config.toml with a universal provider/model schema; encrypted secret store (ChaCha20-Poly1305).

## Efficiency claims and mechanisms

- Marketing claims: <5 MB RAM, <10 ms startup, ~3.4-8.8 MB binary (inconsistent). Own docs are more honest: ~26 MiB typical full release build, ~6.6 MB kernel-only minimal preset, "no meaningful memory floor" on Raspberry Pi (Pi Zero 2 W upward). No third-party benchmark validates the extreme numbers; the RAM figure also excludes local model inference, which dwarfs the runtime.
- How achieved: no runtime/VM; single static binary; cargo release profile opt-level=z, fat LTO, codegen-units=1, strip, panic=abort; microkernel plus aggressive feature-gating (every channel, gateway, hardware, observability, plugins individually opt-in cargo features; a no-default-features foundation build); dependency discipline (tokio minimal features, reqwest+rustls, bundled rusqlite, LRU-bounded sender state); terse localized tool descriptions; install presets so users compile only what they deploy.
- Edge support: ARM aarch64 + armv7 32-bit, x86, RISC-V; Linux/macOS/Windows/FreeBSD/NixOS/Docker/Android. Dedicated Raspberry Pi quickstart; prebuilt per-arch binaries because fat-LTO linking can OOM low-RAM boards; systemd user service; Podman recommended over Docker (rootless, no daemon). Hardware: GPIO/I2C/SPI/USB on Raspberry Pi, STM32 Nucleo, Arduino, ESP32 via the Peripheral trait; firmware directory and robot-kit crate.

## Channels

30+ channels (Telegram, Discord, Slack, Matrix, WhatsApp, Signal, iMessage, IRC, email + Gmail push, Bluesky, Nostr, Reddit, Twitter, Mattermost, Notion, webhooks, voice call, voice wake, TTS, transcription...). Kept lightweight because every channel is its own cargo feature (default bundle is only ACP + webhook + email + Telegram + Discord + filesystem); one thin Channel trait rather than per-platform frameworks; a shared orchestrator handles debounce, dedup, stall watchdog, link enrichment, and a SQLite-backed session store; listeners are tokio tasks in the one process; reliability concerns live in trait defaults and the orchestrator, not per-adapter.

## Local model support

~20+ providers; Ollama is first-class (native /api/chat, schema-based structured output, no API key); llama.cpp / LM Studio / any OpenAI-compatible endpoint via a custom slot. Cloud: Anthropic, OpenAI, Gemini, Bedrock, Azure, OpenRouter, Groq, Mistral, and others.

Small-model quality techniques:

- The zeroclaw-tool-call-parser crate is the key small-model trick: it parses tool calls from raw text for models lacking native tool-call APIs — JSON tool_calls, XML variants (tool_call, invoke, DeepSeek/Qwen formats), markdown code fences, shorthand, vendor formats — with malformed-JSON recovery, tool-name aliasing, and think-tag stripping.
- ProviderCapabilities flags let the loop degrade gracefully when native tool calling is absent.
- Built-in hybrid memory search (vector embeddings + keyword/FTS in SQLite, no external service) keeps context small and relevant; history pruning/trimming manages small context windows aggressively.
- loop_detector and safety_net catch small-model failure modes (repetition, runaway loops).
- Routing recommendation: multiple agents per task with channels routing traffic to the right agent, rather than in-loop fallback chains.

## Key design ideas worth stealing

1. Five-trait kernel in one tiny API crate — everything, including plugins, implements the same contracts. Go equivalent: one internal api package of interfaces; adapters never import each other.
2. Compile-time composition: every channel/tool/subsystem behind a build tag/feature; ship presets (minimal ~6.6MB vs full ~26MB). Go equivalent: build tags plus separate cmd targets.
3. Tolerant tool-call parser as a standalone library — the highest-leverage piece for driving Gemma-class local models. Directly portable to Go.
4. Channel trait with fat defaults: draft/update/finalize streaming, approval prompts over any channel, self-message guards, health checks — adapters stay thin, reliability lives in defaults/orchestrator.
5. Security-by-default that costs nothing at runtime: loopback-only gateway plus pairing-code → bearer token, workspace-scoped FS with symlink-escape detection, risk-tiered autonomy (block high, approve medium), HMAC tool receipts, per-initiator memory-injection policy, orchestrator events never routable from user-controlled fields.
6. WASM component plugins with capability manifests and fuel metering, behaviorally equivalent to built-ins; interpreter fallback for ARM32. Go equivalent: wazero (pure-Go, CGO-free).
7. SOP/routines engine as a separate resumable orchestration layer (cron/MQTT/webhook/peripheral-triggered, approval gates) instead of overloading the chat loop — the proactive-over-reactive piece.
8. Honest ops ergonomics for edge: prebuilt per-arch binaries because linking OOMs a Pi; systemd user service + linger; SQLite-only default with pluggable backends later (trait now, backends when needed).

## Weaknesses and tradeoffs

- Marketing numbers inconsistent and unverified by third parties; RAM figure excludes local inference (the "runs on $10 hardware" claim assumes cloud inference).
- Very young with huge churn: mid microkernel refactor, 662 open issues; the agent loop is a 638KB single file — internal code quality lags the clean trait story.
- Security story incomplete per reviewers: pairing/gating covers who can message the bot, not prompt injection or exfiltration by the agent itself.
- Missing OpenClaw features: multi-agent communication, heartbeat parity, ecosystem integrations; smaller community.
- Feature-flag matrix explosion is a heavy CI burden; users must know cargo features to get the small binary.
- No built-in provider fallback chains (pushed to multi-agent routing); memory backend switch has no data migration.

## Graph facts

- zeroclaw|is_a|AI agent runtime
- zeroclaw|written_in|Rust
- zeroclaw|maintained_by|zeroclaw-labs
- zeroclaw|positioned_as|lightweight alternative to OpenClaw
- openclaw|written_in|TypeScript/Node.js
- openclaw|criticized_for|over 1GB RAM usage
- zeroclaw|claims|under 5MB RAM and 10ms startup
- zeroclaw|typical_build_size|26 MiB full, 6.6 MB minimal preset
- zeroclaw|uses|tokio, reqwest/rustls, rusqlite, axum
- zeroclaw|organized_as|20-crate microkernel workspace
- zeroclaw_api|defines|ModelProvider, Channel, Tool, Memory, Peripheral traits
- zeroclaw_runtime|contains|agent loop, safety_net, memory_inject, cost tracking
- zeroclaw_tool_call_parser|enables|tool calling for models without native tool APIs
- zeroclaw_tool_call_parser|parses|JSON, XML, markdown-fence, shorthand formats
- zeroclaw|supports|30+ channels via cargo features
- channel_trait|provides|send/listen, draft streaming, approval prompts, self-loop guards
- zeroclaw|supports_local_models_via|Ollama native API and OpenAI-compatible endpoints
- zeroclaw_memory|defaults_to|SQLite hybrid vector+keyword search
- zeroclaw_plugins|run_on|wasmtime WASI Component Model
- zeroclaw_plugins|sandboxed_by|capability manifests and fuel metering
- pulley_interpreter|enables|plugins on ARM32
- zeroclaw_gateway|binds|127.0.0.1 with pairing-code auth
- zeroclaw_security|enforces|risk-tiered autonomy
- zeroclaw|emits|HMAC-SHA256 tool receipts
- zeroclaw_sop_engine|triggered_by|cron, MQTT, webhooks, peripherals
- zeroclaw|runs_on|Raspberry Pi Zero 2 W through Pi 5
- zeroclaw_hardware|abstracts|GPIO/I2C/SPI/USB via Peripheral trait
- zeroclaw|release_profile|opt-level z, fat LTO, strip, panic abort
- zeroclaw|configured_by|single TOML config
- zeroclaw|criticized_for|incomplete prompt-injection story, unverified perf claims
- assistclaw|closest_comparable|zeroclaw
