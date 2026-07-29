# AssistClaw Revamp — Master Plan

> Single source of truth for the architecture revamp. Absorbs `JARVIS_ROADMAP.md`
> (its 5 phases + 11 production bugs become workstreams WS6–WS9 here).
> Written to be executable by any model, including low-capability ones: every
> workstream has concrete acceptance criteria (AC).

## Context

AssistClaw is Hridesh's personal always-on "Jarvis" in Go (~45k LOC, ~27 internal
packages). North star: **availability > features, offline > cloud, proactive > reactive**.

**Central pillar — unified cross-surface memory (like Claude's memory).** The agent must
hold *all* of the user's memories and keep them **continuous across every surface**: AI chat
(channels), co-work (collaborative sessions), and code (coding assistance). One memory of the
user, keyed by identity — never siloed per channel or per session. A Jarvis that forgets you
between surfaces isn't one. This reshapes WS8 from a subsystem into a first-class workstream.

Revamp targets:
1. Best-in-class CPU/RAM efficiency; cheap in cloud; performant for any task.
2. Runs on almost all OSes, and efficiently on edge devices (Raspberry Pi class),
   eventually inside/alongside custom wearables.
3. Rebuild the strongest capabilities of the researched agents (jcode, Hermes Agent,
   ZeroClaw) plus solid agentic tool-use.
4. Codebase restructured on SOLID principles.
5. Future: drive local edge models (Gemma-class) so responses feel frontier-quality.

Research groundwork: `research/agents/*.md` (jcode, hermes-agent, zeroclaw, codegraph,
assistclaw-vision) and a knowledge graph in `graphify-out/` (query with `graphify query "..."`).

## Research → design decisions

| Source | Idea adopted | Why |
|---|---|---|
| ZeroClaw | Five-trait contracts package (Provider, Channel, Tool, Memory, Peripheral) | SOLID backbone; built-ins AND plugins implement the same contracts |
| ZeroClaw | Compile-time composition via build tags; minimal edge preset vs full desktop | tiny edge binary + full desktop binary from one repo |
| ZeroClaw | Tolerant tool-call parser (JSON/XML/fenced/shorthand + recovery) | THE lever for Gemma-class models without native tool APIs |
| ZeroClaw | SOP/routine engine separate from chat loop | proactive > reactive without bloating the loop |
| ZeroClaw | Security-by-default (loopback gateway, pairing, risk-tiered autonomy) | zero-runtime-cost safety |
| jcode | Persistent server + thin clients (Unix socket) | always-on daemon; channels/TUI attach; ~10 MB marginal per session |
| jcode | Async sidecar memory: retrieval turn N → injected turn N+1 | small models get context "for free", loop never blocks |
| jcode | Context guardrails in tool layer (per-output 30%, projected 90%) | prevents context blowouts on small-ctx models |
| jcode | Prompt-cache discipline (deterministic tool order, cache-aware compaction) | biggest cloud cost lever |
| Hermes | Pluggable ContextEngine (should_compress/compress/select/prune-tool-results-first) | model-light context management, ideal on edge |
| Hermes | Byte-stable prompt prefix; cache re-decoration on failover | cache hits survive failover |
| Hermes | Self-distilling SKILL.md learning loop + FTS5 recall | agent grows with user, fully offline |
| Hermes | Small-model ergonomics: fewer tools, strict XML tool format, fixed sampling | frontier-feel from small models |
| codegraph | Typed nodes/edges, deterministic IDs, provenance, structure-not-content | AssistClaw's memory graph + future codebase index |

Anti-goals: Hermes's 64k hard context floor (rejects edge models — we degrade
gracefully instead), Python+Node runtime weight, jcode's cloud-dependent memory
consolidation, ZeroClaw's 638KB god-file loop.

## Current state (audit)

**Strengths to keep:** clean `provider.Provider` interface + `FailoverProvider`;
`channels/adapter` contract package (Adapter/Identity/InboundLifecycle/OutboundSender/Health);
3-tier memory (Working / Episodic FTS5 / Semantic sqlite-vec — vendored C ext, no
external service); in-flight `proactive` engine; `localintel` Gemma via gollama.cpp
behind build tags; zig static cross-builds + `make pi` + `doc/RASPBERRY_PI.md`; otel observability.

**Problems (revamp targets):**
1. God object `agent.Runner` (`internal/agent/runner.go`, 2,077 lines) — turn loop,
   planning, reflection, prompt builder, tool table, tool select/filter/exec, channel
   handling, streaming adapters, slash-command router, identity mgmt, memory flush.
2. God function `runAgent` (`cmd/assistclaw/main.go:1122–2085`, ~960 lines) — all wiring inline.
3. `Tool` interface defined in `agent` (runner.go:41), not a neutral contract pkg;
   impl packages know each other — no dependency-inversion seam.
4. No context-engine abstraction; system prompt rebuilt per request (cache-hostile).
5. Everything always-on: one binary links whatsmeow/chromedp/AWS/tsnet/otel/TUI regardless
   of deployment; only fts5/localgemma are tagged.
6. No tolerant tool-call parser (Gemma tool calls will fail).
7. Small-model ergonomics missing (no per-model toolset/format/sampling/context-floor).
8. Legacy TS layer (removed in WS0).
9. 11 cataloged production bugs (see JARVIS_ROADMAP §2).

## Decisions (approved 2026-07-29)

1. **Language: Go core + Rust satellites.** Go core stays. Rust only where it wins
   (`tui_rs` now; optional perf satellites later). No full-Rust rewrite.
2. **TS layer removed** (WS0). Future plugins = WASM via wazero, not Node.
3. **One master plan** (this doc) absorbs `JARVIS_ROADMAP.md`.

## Target architecture

### `internal/core` — the contracts package (std-only, zero third-party imports)

```
internal/core/
  provider.go      Provider, CompletionRequest/Response, StreamEvent, ProviderCaps, ModelInfo
  tool.go          Tool { Definition() ToolDef; Execute(ctx, json.RawMessage)(string,error) }, ToolRegistry iface, ToolDef
  channel.go       Channel contracts (unify channels/adapter) + StreamingReplyFunc + draft/approval defaults
  memory.go        WorkingStore, EpisodicStore, SemanticStore, MemoryEntry
  contextengine.go ContextEngine { ShouldCompress(pressure) bool; Compress(ctx,msgs); SelectContext(ctx,turn); PruneToolResults(msgs,keepLast) }
  peripheral.go    Peripheral { Name; Capabilities; Start(ctx, emit); Stop } — sensing/GPIO, the wearable seam
  events.go        StreamHandler, InboundEvent, Notification
  errors.go        one retryable/permanent error taxonomy (merge ProviderError + ChannelError)
```

Import rule (CI-enforced, WS1): `core` imports only std; `internal/{providers,channels,
tools,memory,...}/**` import `core` but never each other; only `internal/kernel` imports everything.

### `internal/kernel` — composition root

Replaces `runAgent`: `kernel.Build(cfg) (*App, error)` + small `buildProviders/buildMemory/
buildChannels/...` (≤60 lines each, fake-config testable). `App.Start(ctx)/Stop(ctx)`
supervises subsystems (ordered start, reverse-order drain ≤15s). `cmd/assistclaw` shrinks
to cobra definitions calling kernel.

### Agent decomposition (same package, focused files)

`turn.go` TurnEngine (≤300 lines, only the iterate loop; deps = core interfaces) ·
`prompt.go` PromptBuilder (byte-stable prefix) · `toolexec.go` ToolExecutor (guardrail +
result caps) · `commands.go` table-driven slash router · `identity.go` · `planner.go`
(behind `Planner`, off on edge) · `runner.go` thin façade preserving `Run/RunStream/HandleChannelMessage`.

### ContextEngine + cache discipline

`internal/contextengine`: pressure measured pre-request; defense order = prune old tool
results (keep last 6 turns, stub rest) → compact middle (reuse `WorkingMemory.Compact`) →
LLM summarize last resort, never mid-tool-chain; compress at 75% ctx; protect first 3 + last 6.
PromptBuilder emits stable prefix + volatile suffix (never mutate prefix); tools
deterministically sorted; `cache_control` set by provider adapter from `ProviderCaps`;
failover re-decorates per provider. ToolExecutor caps: single result ≤30% budget, projected ≤90%.

### Small-model layer

`internal/toolcallparse`: tolerant parser (JSON/XML `<tool_call>`/fenced/shorthand,
malformed-JSON recovery, name aliasing via Levenshtein ≤2, `<think>` stripping), fuzz-tested;
used when `ProviderCaps.NativeToolCalling == false`. `internal/modelprofile`: per-model class
{max tools, format, sampling presets (Gemma temp 0.6/top_p 0.95/top_k 20), graceful
context-floor degrade (shrink+prune, NOT reject), terse template}. Gemma default: ≤8 tools,
XML, aggressive pruning, sidecar injection.

### Memory upgrades — unified cross-surface user memory (central pillar)

Keep the 3 tiers (Working / Episodic FTS5 / Semantic sqlite-vec) but make them **one user-memory
store keyed by user identity, shared across all surfaces** — chat channels, co-work sessions, and
code assistance — never siloed per channel or per session. A fact learned in chat is available
while coding and vice versa. Concretely:
- **Identity-keyed store:** episodic + semantic rows carry a stable `user_id` (not just `session_id`);
  retrieval spans the user's whole history, scoped/ranked by recency + relevance, not by channel.
- **Sidecar retrieval** (`memory/sidecar.go`): async retrieve → inject the user's relevant memories
  into *every* surface's next turn via `ContextEngine.SelectContext`; never blocks the loop.
- **Learning loop** (`skills/distill.go`): 3 similar successes → nudge write SKILL.md, grade by usage,
  archive 90d — offline (FTS5 + templates), LLM only for the distill step.
- **Recall tool** `memory_sessions_search` over episodic FTS5, capped output, cross-surface.
- Surface adapters (channels, co-work, code/MCP) all read/write the same store — no per-surface memory.

### Compile-time presets (build tags)

| Tag | Channels | Extras | Target |
|---|---|---|---|
| `edge` | telegram, webhook | gateway(no tsnet), sqlite+fts5+vec, localintel optional | Pi Zero 2W / wearable host |
| default | + discord, slack, email | + tsnet, webui, voice, mcp | Pi 5 / home server |
| `full` | + whatsapp, browser, bedrock/aws | + TUI, sensing bridge | laptop/desktop |

Gate: whatsmeow, chromedp, AWS SDK, tsnet, otel exporters (edge = no-op), TUI. Pattern:
per-feature `register_<x>.go` (proven by `localintel/embedded_{on,off}.go`). Makefile:
`make edge|pi|full`; CI prints size+RSS per preset, gates >10% regression.

### Daemon + thin clients

Add a local control socket; `assistclaw repl|status|attach` become thin clients of the
running daemon (one model/memory instance serves TUI + channels + gateway).

## Workstreams (structure → efficiency → features → reach)

- **WS0 Hygiene + baseline** — branch; `doc/perf/BASELINE.md`; this doc; delete TS layer;
  `make build` green without pnpm. AC: build green; baseline committed. **[in progress]**
- **WS1 core + kernel** — `internal/core`, `internal/kernel`, import-lint. AC: main.go <400 lines;
  every pkg imports core not siblings; tests green.
- **WS2 Runner decomposition** — split per above; unit tests. AC: runner.go ≤300 lines; turn
  loop testable offline; parity on channel/email tests.
- **WS3 ContextEngine + cache** — implement + integrate + cache_control + tool caps. AC:
  byte-identical stable prefix golden test; 200k tool result can't blow ctx; cache-read >0 2nd call.
- **WS4 small-model layer** — toolcallparse (fuzz), modelprofile, wire local providers, Gemma
  profile. AC: fuzz green; Gemma does a 3-tool task offline; profile switch config-only.
- **WS5 presets + perf gates** — tag-gate heavy deps; `make edge|pi|full`; `scripts/perfcheck.sh`;
  CI regression gate. AC: edge binary measured; budgets enforced.
- **WS6 resilience** [roadmap Phase 2] — fix 11 bugs (one commit/category); drain; `/livez /readyz
  /metrics`; crash telemetry to own Telegram; offline mode. AC: roadmap §6.3.
- **WS7 proactivity** [roadmap Phase 1] — milestones 1.1–1.8 on core contracts. AC: roadmap §5.7.
- **WS8 unified cross-surface memory + learning** [central pillar] — identity-keyed memory store
  shared across chat/co-work/code; sidecar retrieval + next-turn injection on every surface;
  SKILL.md distillation; cross-surface session-recall tool. AC: a fact stored via chat is retrieved
  while coding (cross-surface test); injected memory appears next turn; SKILL.md draft loads; zero
  added turn latency (sidecar async, measured).
- **WS9 mobile + context + actions** [roadmap Phases 3–5] — PWA+push; wake-word (sensing →
  Peripheral); awareness → volatile suffix; Home Assistant skill; approval gates over any channel.
  AC: roadmap §7.4/§8.4/§9.2.
- **WS10 wearable/edge** (exploratory) — Peripheral impl for sensing; edge on Pi Zero 2W; BLE/serial
  bridge design doc; llama.cpp shared-inference option. AC: edge runs 24h on Pi within RSS budget.

## Performance budgets (enforced from WS5)

| Metric | edge | default | full |
|---|---|---|---|
| Binary size | ≤ 25 MB | ≤ 45 MB | ≤ 80 MB |
| Cold start → ready | ≤ 150 ms | ≤ 400 ms | ≤ 800 ms |
| Idle RSS (10 min) | ≤ 30 MB | ≤ 60 MB | ≤ 120 MB |
| Turn overhead (non-LLM) | ≤ 20 ms | ≤ 30 ms | ≤ 50 ms |
| Idle goroutines | ≤ 40 | ≤ 80 | ≤ 150 |

Current default binary = 51.9 MB (see BASELINE.md) — WS5 tag-gating brings it down.

## Verification

- Every WS: `make test` (race), `make check` (vet+fmt+lint), plus WS-specific AC tests.
- Parity harness (before WS2): golden transcripts of 5 flows (CLI chat, Telegram round-trip,
  email triage, cron job, Gemma offline answer); replay after each structural WS.
- Perf harness `scripts/perfcheck.sh` per preset in CI and on Pi via `make pi`.
- E2E: run daemon, send Telegram message, watch reply; run edge binary on Pi/Docker arm64.

## Execution discipline

- One workstream at a time; one milestone per commit; never mix refactor + behavior change.
- WS1/WS2 are pure refactors — parity harness is the gate, no behavior change permitted.
- File > ~500 lines or function > ~80 lines → split before continuing.
- All new interfaces live in `internal/core`.
- Milestone-level detail for WS6–WS9 lives in `JARVIS_ROADMAP.md` §5–§9 (the sub-plan of record).
