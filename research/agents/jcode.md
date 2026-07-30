# jcode (github.com/1jehuang/jcode)

## What it is

jcode is a Rust-native terminal AI coding-agent harness (TUI plus persistent server framework) by 1jehuang, self-described as "the most RAM efficient harness." It is a Claude Code / Codex-class agent CLI with extreme resource efficiency: ~27.8 MB RAM per session (vs 140-386 MB for competitors), ~10.4 MB marginal RAM per additional agent, 14 ms time-to-first-frame, plus semantic memory, multi-agent "swarm" coordination, and a self-modifying "self-dev" mode.

Language/runtime: Rust on the Tokio multi-threaded async runtime; no Node/Electron. TUI on ratatui + crossterm with a custom smooth-scrolling layer, plus cosmic-text/vello/wgpu for rich rendering. Maturity: ~13.3k stars, created January 2026, very active, MIT license, ~75+ crates in the workspace. Platforms: Linux x86_64/aarch64, macOS, Windows, Termux, iOS client.

## Architecture

- Process model: client/server. A persistent server owns sessions; thin clients (TUI, CLI run, connect) attach over Unix sockets. Sessions share the server process, so each extra agent adds only ~10 MB. Modes: interactive TUI, non-interactive run, named resumable sessions, persistent background agents, overnight/ambient mode.
- Agent loop: Agent with run_turn; streaming via mpsc channels; SoftInterruptQueue for graceful interrupts; input delivery interleaves with agent operations specifically to preserve KV/prompt-cache efficiency; cache-expiry notifications for Claude's 5-minute prompt cache.
- Tools: a Tool trait (async execute + to_definition returning name/description/JSON schema). Central registry is Arc<RwLock<HashMap<String, Arc<dyn Tool>>>>; stateless base tools cached in OnceLock, session-specific tools built per session. Tool definitions are allow-list filtered and deterministically sorted for prompt-cache consistency. Two-layer permission gating (session allow/deny policies plus pre_tool hook). Dispatch handles alias resolution, telemetry, post_tool hooks, and hallucinated-tool-name recovery via Levenshtein/prefix suggestions. Tool-output overflow guard: a single output is capped at 30% of the context budget, total projected context at 90%, with advisory truncation notices. 30+ tools including agentgrep (structural grep), browser, OS-level computer control, websearch, gmail, memory, todo, batch, swarm messaging, and selfdev.
- Context management: CompactionManager with manual compaction plus auto-compact on context-limit errors; hard compaction (synchronous summarize), a semantic mode with embedding snapshots, and provider-native compaction. After compaction it resets the cache tracker and provider session IDs for cache coherency.
- Memory: multi-layer semantic memory graph. Local all-MiniLM-L6-v2 embeddings via tract-onnx (pure-Rust ONNX, no external API), petgraph DiGraph with Memory/Tag/Cluster nodes; three organization layers (explicit tags, HDBSCAN auto-clusters, semantic edges: RelatesTo/Supersedes/Contradicts/DerivedFrom). Retrieval is cosine-similarity seed hits (0.5 threshold) plus BFS cascade to depth 2 with score decay. Injection is async and non-blocking: a memory agent runs on an mpsc channel, and results from turn N are injected at turn N+1 — no explicit tool call needed. Consolidation does write-time duplicate/contradiction detection; a small cloud sidecar model verifies retrieval relevance. Storage is JSON under ~/.jcode/memory/ with embeddings stored separately; project scope keyed by directory hash plus a global scope. Skills also load lazily by conversation semantic similarity, not at startup.
- Providers: a Provider trait behind Arc<dyn Provider> with available_models, model_routes, set_model_with_auth_refresh, set_reasoning_effort. A MultiProvider composite coordinates backends. Native OAuth for Claude/OpenAI/Gemini/Copilot/Azure; any OpenAI-compatible endpoint; local models via Ollama / LM Studio / vLLM endpoints. Secrets live in a protected app dir, not config files.
- Multi-agent swarm: agents in the same repo auto-coordinate through the server (file-conflict notifications), spawn subordinate swarms, direct-message or broadcast.
- Other: cross-harness session migration (Claude Code → jcode), self-dev mode (agent edits own source, rebuilds, hot-reloads without dropping sessions), MCP config compatible with Claude Code, voice input, restart snapshots.

## Performance and efficiency choices

- Measured: 27.8 MB RAM for one session (local embedding off); 260.8 MB for 10 concurrent sessions vs 833-3,237 MB for competitors; 10-15 concurrent agents on an 8 GB laptop.
- Techniques: single shared server process (marginal session cost, not per-process cost); native AOT Rust binary; Tokio async everywhere; non-blocking memory/embedding sidecar; optional jemalloc; rustls instead of OpenSSL; tract-onnx CPU inference (small pure-Rust ONNX runtime, no libtorch bloat).
- Build strategy: base release profile at opt-level=1 with incremental builds for fast self-dev cycles, surgical per-package opt-level=3 on hot paths (rendering, text shaping, inference, decoding), and a separate thin-LTO release profile for distribution.
- Prompt-cache economics treated as first-class: deterministic tool-definition ordering, cache-expiry alerts, cache-safe input interleaving — directly reduces API cost.

## Key design ideas worth stealing

1. Persistent shared server plus thin clients over Unix sockets: marginal cost per session collapses to ~10 MB — ideal for an always-on Pi/wearable daemon with multiple channel frontends attaching.
2. Async sidecar memory with turn N → N+1 injection: local embeddings plus cosine/graph cascade retrieval, injected automatically without blocking the agent loop or requiring tool calls. This mechanism makes a small local model "feel" smarter because relevant context arrives free.
3. Prompt-cache-first discipline: deterministic tool schema ordering, cache-aware compaction resets, input interleaving that preserves KV cache — the cheapest cloud-cost lever available.
4. Context budget guardrails in the tool layer: per-output 30% cap, 90% projected total cap, hallucinated-tool-name recovery with suggestions — cheap robustness wins, especially for small local models.
5. Semantic lazy-loading of skills/tools by conversation similarity rather than loading everything at startup.

## Weaknesses and tradeoffs for edge deployment

- The headline 27.8 MB is with local embedding off; the ONNX embedding + HDBSCAN + graph stack adds meaningful RAM/CPU when enabled.
- Memory consolidation depends on a cloud sidecar model for relevance verification — not offline-first; self-dev mode requires frontier models.
- Heavy GUI/rendering dependencies (vello, wgpu, cosmic-text) and a Firefox-backed browser tool are irrelevant on a Pi or wearable; it is a developer coding harness, not a headless assistant — no channels (Slack/Telegram/email), no cron/proactive layer.
- Local-model support is endpoint plumbing only (Ollama/vLLM URL); no small-model-specific prompting, quantization management, or model lifecycle. jcode assumes frontier cloud models for agent quality.
- Rust-specific wins (no GC, tract-onnx) don't transfer one-to-one to Go, but the architecture (shared server, marginal-cost sessions, async memory) does.

## Graph facts

- jcode|written_in|Rust
- jcode|created_by|1jehuang
- jcode|is_a|AI coding agent harness
- jcode|uses|Tokio
- jcode|uses|ratatui
- jcode|uses|tract-onnx
- jcode|uses|petgraph
- jcode|architecture|client-server over Unix sockets
- jcode|ram_per_session|27.8 MB
- jcode|ram_marginal_per_agent|10.4 MB
- jcode|embedding_model|all-MiniLM-L6-v2
- jcode|memory_design|semantic graph with tags, clusters, relation edges
- jcode|memory_injection|async sidecar, turn N results injected at turn N+1
- jcode|context_management|CompactionManager
- jcode|tool_system|Tool trait + RwLock registry
- jcode|tool_guardrail|30% per-output / 90% total context caps
- jcode|provider_abstraction|Provider trait + MultiProvider composite
- jcode|supports_providers|Claude, OpenAI, Gemini, OpenRouter, Ollama, vLLM
- jcode|feature|swarm multi-agent coordination
- jcode|feature|self-dev hot reload
- jcode|feature|semantic lazy skill loading
- jcode|optimizes_for|prompt-cache economics
- jcode|weakness_for_edge|cloud sidecar for memory consolidation
- jcode|weakness_for_edge|GPU and browser dependencies
