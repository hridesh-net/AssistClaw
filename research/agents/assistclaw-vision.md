# AssistClaw revamp — vision and constraints

## What AssistClaw is

AssistClaw is Hridesh's personal always-on AI assistant ("personal Jarvis"), written in Go. Today it spans: an agent runner with an autonomous mode, multi-channel adapters (Telegram, Discord, Slack) with a shared reliability layer, an email assistant pipeline (IMAP, rules, approvals, SQLite FTS5 store), a gateway hub with A2A support, MCP client and server, a cron scheduler, a skills system (loader, resolver, marketplace), memory with a file watcher, provider adapters (Anthropic, OpenAI), local intelligence via a Gemma engine, voice client, web UI, security guardrails, a TUI/REPL, and native sensing modules (camera capture and audio streaming in C++).

## North star

Always-on personal Jarvis. Priorities in order: availability over features, offline over cloud, proactive over reactive.

## Revamp goals

1. Most efficient and optimized in CPU and RAM utilization; efficient at handling any task; performant and cheap when deployed to the cloud.
2. Easy and efficient on almost all OS systems (macOS, Linux, Windows, ARM boards).
3. Must run efficiently on small edge devices like a Raspberry Pi, and eventually serve as a personal agent along with or inside custom wearable devices.
4. Future: run edge-capable local models (like Gemma) on small devices or laptops through this agent in a way that responses feel like a smart frontier LLM (Fable-class).

## Benchmarks and inspirations researched

- jcode (Rust coding harness): persistent shared server + thin clients, ~10 MB marginal RAM per agent, async sidecar memory injection, prompt-cache-first discipline.
- Hermes Agent (Nous Research, Python): pluggable context engine, byte-stable prompt caching, self-distilling SKILL.md learning loop, small-model ergonomics via reduced toolsets and strict XML tool-call formats.
- ZeroClaw (Rust personal-assistant infra): five-trait microkernel, compile-time feature composition, tolerant tool-call parser for non-native-tool models, WASM capability-sandboxed plugins, SOP proactive engine, Raspberry Pi-first ops. Closest comparable to AssistClaw.
- codegraph (TypeScript+Rust): symbol-level code knowledge graph with typed nodes/edges, provenance, two-phase extract-then-resolve — the model for this research graph and for future codebase memory.

## Constraint notes

- Go single static binary is AssistClaw's structural advantage over Hermes (Python+Node) and matches ZeroClaw's deployment story; Go build tags can emulate ZeroClaw's cargo-feature composition.
- The Gemma engine in internal/localintel is the seed of the future local-model story; techniques to adopt: tolerant tool-call parsing, reduced toolsets per deployment, aggressive history pruning, hybrid SQLite vector+FTS retrieval, memory injection so small models get context for free.
- Wearable target implies: headless daemon, tiny idle RAM, cheap wake path, hardware peripheral abstraction, and channels as thin attachable clients.

## Graph facts

- assistclaw|is_a|personal always-on AI assistant
- assistclaw|written_in|Go
- assistclaw|owned_by|Hridesh
- assistclaw|north_star|availability over features, offline over cloud, proactive over reactive
- assistclaw|goal|minimal CPU and RAM utilization
- assistclaw|goal|runs on Raspberry Pi and wearables
- assistclaw|goal|cross-OS support
- assistclaw|goal|cheap cloud deployment
- assistclaw|future_goal|local Gemma with frontier-feel responses
- assistclaw|has_component|email assistant pipeline
- assistclaw|has_component|channel adapters (Telegram, Discord, Slack)
- assistclaw|has_component|gateway hub with A2A
- assistclaw|has_component|MCP client and server
- assistclaw|has_component|cron scheduler
- assistclaw|has_component|skills system
- assistclaw|has_component|Gemma local intelligence engine
- assistclaw|has_component|security guardrails
- assistclaw|has_component|C++ sensing modules (camera, audio)
- assistclaw|uses|SQLite FTS5
- assistclaw|inspired_by|jcode
- assistclaw|inspired_by|hermes_agent
- assistclaw|inspired_by|zeroclaw
- assistclaw|memory_graph_modeled_on|codegraph
