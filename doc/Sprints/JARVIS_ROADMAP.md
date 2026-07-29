# AssistClaw → Jarvis: Detailed Implementation Plan

> **Goal.** AssistClaw becomes Hridesh's personal Jarvis — reachable in any condition, any time, across devices, proactively useful, resilient to outages.
>
> **Scope of this document.** Architecture, phased milestones, interface designs, risks, test strategy, and acceptance criteria. No code is written yet — this is the design we agree on *before* implementation begins.

---

## Table of Contents

1. [Vision and Pillars](#1-vision-and-pillars)
2. [Current State Audit](#2-current-state-audit)
3. [Gap Map](#3-gap-map)
4. [Phase Sequencing and Rationale](#4-phase-sequencing-and-rationale)
5. [Phase 1 — Proactivity Engine](#5-phase-1--proactivity-engine)
6. [Phase 2 — Resilience Foundation](#6-phase-2--resilience-foundation)
7. [Phase 3 — Mobile Presence](#7-phase-3--mobile-presence)
8. [Phase 4 — Context Awareness](#8-phase-4--context-awareness)
9. [Phase 5 — Action Surface Expansion](#9-phase-5--action-surface-expansion)
10. [Cross-Cutting Concerns](#10-cross-cutting-concerns)
11. [Risks and Mitigations](#11-risks-and-mitigations)
12. [Definition of Done — Jarvis](#12-definition-of-done--jarvis)

---

## 1. Vision and Pillars

| Pillar | Definition | Why it matters for Jarvis |
|---|---|---|
| **Proactivity** | Claw initiates action based on signals, not just prompts | Jarvis interrupts Tony; he doesn't summon Jarvis for every thing |
| **Resilience** | Survives crashes, network loss, provider outages | "Any condition" requires graceful degradation, never silent failure |
| **Mobile presence** | Reachable from phone, hands-free, anywhere | Without mobile reach, Claw is a CLI — not a presence |
| **Context awareness** | Knows where/when/what Hridesh is doing | Drives both proactive triggers and response shaping |
| **Action surface** | Acts on the physical and digital world | Information without action is just a search engine |

Ordering principle: each pillar must be *individually deliverable* and unlock concrete user value, not just preparatory infrastructure.

---

## 2. Current State Audit

What already exists in the codebase (informs what we extend vs. build new):

| Subsystem | Status | Lives in |
|---|---|---|
| Agent loop (LLM + tool calls + memory) | ✅ Solid | `internal/agent` |
| 3-tier memory (Working / Episodic FTS5 / Semantic vec) | ✅ Solid | `internal/memory` |
| Skills graph (lazy-loaded) | ✅ Solid | `internal/skills`, `skills/` |
| Multi-channel ingress (WA, Telegram, Discord, Slack, WebUI) | ✅ Working | `internal/channels`, `internal/gateway` |
| MCP integration | ✅ Working | `internal/mcp` |
| Daemon mode + cross-platform service install | ✅ Working | `cmd/assistclaw/{daemon,service}*.go` |
| Time-based scheduled prompts (cron) | ✅ Working but isolated | `internal/cron` |
| Voice scaffolding | 🟡 Partial — `sensing/audio` + `internal/voice`, no wake-word loop | |
| Local LLM (gemma) | ✅ Working | `internal/localintel` |
| Security + audit log | ✅ Working | `internal/security` |
| Observability (zap, metrics, tracing, correlation IDs) | ✅ Working | `internal/observability` |
| **Event-driven proactivity** | ❌ Missing | — |
| **Provider failover** | ❌ Missing | — |
| **Outbound notification dispatcher** | ❌ Missing | — |
| **Mobile companion** | ❌ Missing | — |
| **Wake-word loop** | ❌ Missing | — |
| **Cross-device memory sync** | ❌ Missing | — |
| **Context signal ingestion** (location, calendar, device) | ❌ Missing | — |
| **Daemon supervisor + crash telemetry** | ❌ Missing | — |

### Known production-grade gaps (from prior `go-production` audit)

11 issues identified earlier and tracked in Phase 2:
- HTTP body leaks in `email/graph.go`, `voice/client.go`, `tools/web_search.go`
- `http.Client{}` without timeouts in `mcp/client.go`, `localintel/setup.go`, `skills/marketplace.go`
- `http.Server` without read/write/idle timeouts in `gateway/server.go`, `mcp/server.go`
- Panics in `webui/webui.go` + `channels/*` capability paths
- Hub double-close race in `gateway/hub.go`
- `srv.Close()` instead of graceful `Shutdown(ctx)` in mcp/gateway
- Unbounded per-message goroutine fan-out in channels
- `telegram.Ping` goroutine leak
- `context.Background()` injected mid-stack
- `log.Printf` instead of structured zap in gateway

---

## 3. Gap Map

```
Pillar           │ Current │ Phase that closes gap
─────────────────┼─────────┼──────────────────────
Proactivity      │  10%    │ Phase 1
Resilience       │  40%    │ Phase 2
Mobile presence  │   0%    │ Phase 3
Context awareness│  20%    │ Phase 4
Action surface   │  60%    │ Phase 5
```

(Percentages are rough self-assessment; "Action surface" is high because of the skills graph + MCP already in place.)

---

## 4. Phase Sequencing and Rationale

| Phase | Pillar | Duration | Depends on |
|---|---|---|---|
| 1 | Proactivity engine | ~2 weeks | — |
| 2 | Resilience foundation | ~1 week | — (can interleave) |
| 3 | Mobile presence | ~3 weeks | Phase 2 (must be reliable before exposing externally) |
| 4 | Context awareness | ~1 week | Phase 1 (triggers feed context); Phase 3 (mobile contributes signals) |
| 5 | Action surface expansion | ~1 week | Phase 1 (rules drive actions) |

**Cumulative: ~8 weeks**, with Phases 1 and 2 parallelizable to ~6 weeks if discipline permits.

### Why Phase 1 first (your choice)

Proactivity is the visible behavioral leap. Today Claw is a brilliant butler waiting to be called; after Phase 1 it taps you on the shoulder. Phase 2 (resilience) is the foundation but is invisible — the kind of work that's easy to perpetually deprioritize, so it sits in Phase 2 with a hard gate: nothing in Phase 3+ ships until the 11 audit issues are closed.

---

## 5. Phase 1 — Proactivity Engine

### 5.1 Goal

Generalize today's time-based `internal/cron` (prompt-on-schedule) into a full event-driven rule engine: **any signal → any action → any outbound channel**, with cooldowns, predicate filtering, persisted outbox, and hot-reloadable rules.

### 5.2 Architecture

```
┌──────────────┐  Event   ┌──────────┐  Match   ┌──────────┐
│   Trigger    │ ───────► │  Engine  │ ───────► │   Rule   │
│ (email,cal,  │          │ (dispatch│          │ (predicate│
│  cron, push) │          │  + cool- │          │  +prompt) │
└──────────────┘          │  down)   │          └────┬─────┘
                          └──────────┘               │
                                        ┌────────────┴──────────┐
                                        ▼                       ▼
                                 ┌────────────┐        ┌───────────────┐
                                 │   Action   │  resp  │   Notifier    │
                                 │ (run_agent,│ ─────► │ (Telegram,    │
                                 │  shell,    │        │  Discord,     │
                                 │  webhook)  │        │  push, console)│
                                 └────────────┘        └───────┬───────┘
                                                               │
                                                       ┌───────▼───────┐
                                                       │    Outbox     │
                                                       │ (SQLite, retry)│
                                                       └────────────────┘
```

### 5.3 Core abstractions (proposed; not yet code)

These are the Go interfaces the engine will expose. Final names land at implementation; this is the contract we're agreeing on.

```go
// Package: internal/proactive

type Event struct {
    Source  string         // trigger name
    Type    string         // domain event ("received", "starting_soon")
    Payload map[string]any // structured, template-substitutable
    Time    time.Time
}

type EmitFunc func(Event)

type Trigger interface {
    Name() string
    Start(ctx context.Context, emit EmitFunc) error // long-running
}

type Predicate func(Event) bool

type Rule struct {
    ID        string
    Trigger   string
    Match     Predicate
    Action    string
    Prompt    string        // text/template against Event
    NotifyTo  []string
    Cooldown  time.Duration
    Enabled   bool
}

type Action interface {
    Name() string
    Execute(ctx context.Context, ev Event, rule Rule) (string, error)
}

type Notification struct {
    RuleID string
    Body   string
    Meta   map[string]string // links, action buttons, severity
}

type Notifier interface {
    Name() string
    Send(ctx context.Context, n Notification) error
}

type Engine interface {
    RegisterTrigger(Trigger)
    RegisterAction(Action)
    SetRules([]Rule) error // atomic, validating
    Start(ctx context.Context)
    Stop()
}
```

Design decisions and rationale:

| Decision | Why |
|---|---|
| `AgentInvoker` lives in `proactive`, not `agent` | Point-of-consumption interface principle. Avoids import cycle. Engine testable without constructing a Runner. |
| Cooldown enforced in engine, not in actions | Single place to reason about notification storms; rules opt in declaratively. |
| Triggers own their goroutine and lifecycle | Lets each trigger pick its model: cron-tick, push-IDLE, polling, webhook. Engine just spawns + cancels. |
| `SetRules` is atomic + validating | Bad reference returns error without partial apply — safe for hot reload from YAML later. |
| Notifier registry is separate from action registry | Same action can fan out to many channels; rules opt in per-instance. |
| Outbox is a *Notifier wrapper*, not a separate concept | Any notifier can be wrapped to gain persistence + retry without engine changes. |

### 5.4 Milestones

| # | Milestone | Deliverable | Acceptance |
|---|---|---|---|
| **1.1** | Foundation scaffold | `internal/proactive/` with Engine, Rule, Trigger/Action/Notifier interfaces; ManualTrigger + RunAgentAction + WriterNotifier; 5 unit tests under `-race` | `go test -race` green; engine starts and stops cleanly; manual fire → agent invocation → notifier output |
| **1.2** | Real notifiers + agent adapter | `TelegramNotifier`, `DiscordNotifier` reusing `internal/channels` adapters; `agentInvokerAdapter` wraps `*agent.Runner`; CLI `assistclaw proactive notify <channel> <msg>` | Notification arrives in target chat within 5s; failure logged via zap |
| **1.3** | Time trigger (cron unification) | `CronTrigger` wraps `robfig/cron/v3`; existing `internal/cron/Daemon` rewritten as a thin shim over the proactive engine; old YAML format preserved | All existing cron jobs run through the new engine with no user-visible change |
| **1.4** | Email trigger (IMAP IDLE) | `EmailTrigger` using IMAP IDLE on configured account; emits `email.received` with `{from, subject, snippet, importance}`; importance computed by existing `internal/email/rules.go` | New email of priority ≥ "important" → Telegram ping within 60s with 2-line summary |
| **1.5** | Calendar trigger | `CalendarTrigger` polling Google Calendar every 60s; emits `event.starting_soon` at T-10min and T-2min; deduped by event ID + window | Test calendar event → 10-min warning lands in Telegram with attendee context + last related email thread |
| **1.6** | YAML rule loader + hot reload | `~/.assistclaw/rules.yaml`; loader validates references; `fsnotify` triggers `SetRules` reload | Edit YAML → log line confirms reload within 2s; broken YAML rejected with line number, old rules retained |
| **1.7** | Rule CRUD CLI | `assistclaw rule list / add / enable / disable / test <id> [--with-event '{...}']` | `rule test` injects a synthetic event without arming the trigger; round-trips through action + notifier |
| **1.8** | Outbox + retry | SQLite-backed `OutboxNotifier` wrapping any Notifier; exponential backoff with jitter; survives daemon restart | Kill daemon mid-send → on restart, pending notifications retry to completion; integrated with `dlq_cmd.go` |

### 5.5 Test strategy

| Layer | Approach |
|---|---|
| Unit | Fakes for `AgentInvoker`, `Notifier`; `ManualTrigger` for deterministic event injection; all tests `-race`-clean |
| Integration | IMAP test server (e.g. `github.com/emersion/go-imap/server` in-process); Telegram bot in dedicated test chat |
| End-to-end | `docker compose` with mailhog + a fake calendar HTTP server; assert notification arrives on local stdout notifier within deadline |
| Soak | Long-running daemon test: 1h, 100 synthetic events, assert no goroutine leak (`runtime.NumGoroutine` bounded), no FD leak |

### 5.6 Observability for Phase 1

- New metrics: `proactive_events_total{trigger}`, `proactive_rules_fired_total{rule,result}`, `proactive_action_duration_seconds{action}`, `proactive_notify_total{notifier,result}`, `proactive_cooldown_suppressed_total{rule}`.
- New audit log entries: every rule firing, every notifier send, every YAML reload.
- New `assistclaw doctor proactive` subcommand: lists rules, last-fired times, notifier reachability checks.

### 5.7 Phase 1 exit criteria

- [ ] All 8 milestones merged
- [ ] New email priority ≥ important → Telegram ping in ≤60s, end-to-end
- [ ] Calendar event T-10min → enriched Telegram ping
- [ ] Outbox survives kill -9 of daemon mid-send
- [ ] Zero new resource leaks (audit checks from Phase 2 applied)
- [ ] Test suite includes 1h soak test passing in CI

---

## 6. Phase 2 — Resilience Foundation

### 6.1 Goal

Make Claw's foundation production-grade enough to *trust as Jarvis*. Today, transient failures can take it down at the worst moment.

### 6.2 Milestones

| # | Milestone | Deliverable |
|---|---|---|
| 2.1 | Fix the 11 audited prod bugs | One PR per category: HTTP body leaks, client timeouts, server timeouts, panics → errors, hub double-close, graceful shutdown, goroutine bounds, `telegram.Ping` leak, context plumbing, structured logging |
| 2.2 | Provider failover | `internal/provider/failover.go`: wraps N providers; on transient error (timeout / 5xx / rate-limit) tries next; circuit-breaker per provider with cooldown; final fallback to local gemma |
| 2.3 | Daemon supervisor | systemd unit (Linux) + launchd plist (macOS) + Windows Service with `Restart=on-failure`, `RestartSec=5`, exponential backoff cap |
| 2.4 | Crash telemetry to self | Panic handler writes redacted stack + last 200 log lines to disk; on next start, `OutboxNotifier` ships report to your Telegram |
| 2.5 | Graceful shutdown drain | All long-lived servers (gateway, mcp, daemon) → `srv.Shutdown(ctx)` with 15s drain; in-flight requests complete, new ones rejected with 503 |
| 2.6 | Health endpoints | `/livez` (process up), `/readyz` (deps reachable: DB, primary provider, notifier registry), `/metrics` Prometheus |
| 2.7 | Offline mode | When all remote providers unreachable: route through gemma; queue tool calls that need internet for later replay; UI shows "offline mode" indicator |

### 6.3 Exit criteria

- [ ] `go-production` audit re-run finds zero of the original 11
- [ ] Killing OpenAI traffic via firewall → next request succeeds via Anthropic transparently
- [ ] Killing all internet → simple Q&A still works via gemma
- [ ] Kill -9 of daemon → systemd restarts within 5s; crash report arrives on Telegram

---

## 7. Phase 3 — Mobile Presence

### 7.1 Goal

Claw is reachable from your phone, hands-free, on any network.

### 7.2 Subcomponents

#### 7.2.1 Reachability layer

- **Tailscale-based** (recommended): both daemon and phone join your tailnet; phone hits `http://claw.tail-xxx.ts.net:port`. Zero new infra.
- **Alternative**: self-hosted relay (small VPS running a reverse-proxy gateway); phone → relay → home daemon over WebSocket. Required only if Tailscale is unavailable.

Decision deferred to Milestone 3.1.

#### 7.2.2 Mobile companion

Two-step strategy:
1. **PWA first** (Milestone 3.2): React/Svelte web app served by the gateway, installable to home screen, uses Web Push API + Web Speech API. Ships in days. Limitations: background audio is unreliable on iOS, push notifications on iOS require Safari ≥16.4 + HTTPS.
2. **Native iOS app** (Milestone 3.5, optional): SwiftUI, APNs, background audio session, Siri shortcuts, Widgets, CarPlay. Use `apple-swift` skill. Justified only if PWA limits prove blocking.

#### 7.2.3 Wake-word loop

- Uses existing `sensing/audio` (audio_stream.cpp).
- Wake-word detection: Porcupine (custom "Hey Claw" model, free for personal) or openWakeWord (fully OSS).
- Pipeline: mic → VAD → wake-word detector → buffer next 5s → Whisper.cpp (already integrated via `openai-whisper` skill or local) → agent → TTS via `sherpa-onnx-tts` skill.
- Runs on the laptop daemon initially; mobile gets it via PWA Web Speech API or native iOS in 3.5.

### 7.3 Milestones

| # | Milestone | Deliverable |
|---|---|---|
| 3.1 | Reachability spike | Tailscale set up; gateway reachable from phone browser off home Wi-Fi; documented in runbook |
| 3.2 | PWA companion v1 | Chat UI + push notifications + voice record button; auth via shared secret in URL fragment |
| 3.3 | Wake-word loop (desktop) | "Hey Claw" → STT → agent → TTS round-trip working on laptop |
| 3.4 | PWA voice integration | Web Speech API for STT; agent response auto-spoken via Web Speech Synthesis |
| 3.5 | Native iOS app (optional) | SwiftUI app with APNs + background audio + Siri Shortcut "Ask Claw" |

### 7.4 Exit criteria

- [ ] Phone on cellular, away from home → can chat with Claw via PWA
- [ ] Push notification from a Phase-1 rule arrives on phone within 5s
- [ ] Hands-free "Hey Claw, what's on my calendar?" works on laptop end-to-end

---

## 8. Phase 4 — Context Awareness

### 8.1 Goal

Runner system prompt is enriched per request with what Claw knows about your current state.

### 8.2 Context signals to ingest

| Signal | Source | Update cadence |
|---|---|---|
| Location | Phone (PWA Geolocation or native) → posts to gateway every 5min when active | Push |
| Active calendar event | Phase 1 calendar trigger writes "current event" to context store | On event boundaries |
| Network type | Phone reports cellular/wifi/offline | On change |
| Battery level | Phone reports | On change >10% |
| Screen state | Desktop daemon detects lock/unlock | On change |
| Recent memory highlights | Semantic memory query against last 24h | Per request |
| Time-of-day persona | Configurable: morning/work/evening/sleep | Per request |

### 8.3 Milestones

| # | Milestone | Deliverable |
|---|---|---|
| 4.1 | Context store | `internal/context/store.go`: typed key-value store, in-memory + SQLite snapshot; subscribers fire on change |
| 4.2 | Signal ingestion | Endpoints in gateway: `POST /context/signal`; PWA reports location/battery/network |
| 4.3 | Runner integration | Runner system prompt builder reads context store, injects relevant slice (token-budgeted) |
| 4.4 | Context-driven rule predicates | Rules can match on context: `when email.received AND location=home` |

### 8.4 Exit criteria

- [ ] Ask Claw "should I leave for my meeting?" → response factors in your current location + ETA
- [ ] Sleep-mode rule suppresses non-urgent notifications between 23:00–07:00 *only when phone reports stationary*

---

## 9. Phase 5 — Action Surface Expansion

### 9.1 Milestones

| # | Milestone | Deliverable |
|---|---|---|
| 5.1 | Home Assistant skill | New skill at `skills/home-assistant/` calling HA REST API; "turn off bedroom lights" works |
| 5.2 | Phone actions via PWA | PWA exposes endpoints daemon can call back: send SMS draft, place call (`tel:` link), open Maps with route |
| 5.3 | Approval gate for high-impact actions | Any action tagged `requires_approval` → Claw asks via notifier first; ten-second window for "yes/no" reply |

### 9.2 Exit criteria

- [ ] "Hey Claw, I'm leaving" → away mode: lights off, thermostat down, security armed
- [ ] Claw proposes destructive action → blocks on Telegram confirmation

---

## 10. Cross-Cutting Concerns

### 10.1 Security

- Notifier credentials in OS keychain via existing `1password` / keyring path; never in YAML.
- Rule YAML can execute `run_agent` and `shell` actions — `shell` requires an explicit allowlist of commands and is off by default.
- All proactive firings audited via existing `internal/security` audit log.
- Mobile auth (Phase 3): short-lived JWTs, paired-device list, revocable.

### 10.2 Privacy

- Crash telemetry (Phase 2.4) is to **your own Telegram**, never to a remote service.
- Location signals (Phase 4) are stored locally only; never leave the daemon.
- All embeddings stay in local sqlite-vec.

### 10.3 Performance budgets

| Path | Budget |
|---|---|
| Event ingestion → action start | < 100ms |
| Cooldown decision | < 1ms |
| YAML reload | < 50ms for 100 rules |
| Notifier dispatch (excluding network) | < 5ms |

### 10.4 Observability

Every phase adds its own metrics and `doctor` subcommand. Cumulative dashboard documented in `doc/observability/` updated each phase.

---

## 11. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Phase 1 notification storms (bug in cooldown) | Med | High (you stop trusting it) | Per-rule daily cap as belt-and-braces alongside cooldown; killswitch via `assistclaw rule disable --all` |
| IMAP IDLE flakiness on some providers | High | Med | Fall back to 60s polling per account; configurable per-rule |
| iOS push reliability via PWA | High | Med | Native iOS app in Phase 3.5 if it blocks |
| Tailscale dependency for reachability | Low | High if outage | Document fallback: relay-based path documented even if not implemented |
| Wake-word false positives waking the agent | Med | Low (annoying, not dangerous) | Wake-word confidence threshold tuned; double-trigger required for any *acting* command |
| Scope creep within a phase | High | High | Each milestone has a single deliverable; "Definition of Done" gates merge |
| Resilience phase keeps getting deprioritized | Med | High | Phase 3+ has a hard gate: 11 audit issues must be closed |

---

## 12. Definition of Done — Jarvis

Claw is "Jarvis" when **all** of these are true:

- [ ] Hridesh can ask anything from his phone, anywhere, including offline (degraded mode acceptable).
- [ ] Claw initiates ≥1 useful interaction per day without being asked (email triage, calendar prep, daily brief).
- [ ] Daemon uptime over 30 days ≥ 99.5% on the host machine.
- [ ] No provider outage causes user-visible failure.
- [ ] "Hey Claw" works hands-free on at least one always-with-him device.
- [ ] Claw knows where Hridesh is, what's on his calendar, and what he was last working on.
- [ ] At least one physical-world action surface (home, phone-side) is wired up.

When the checklist is green, the README tagline changes from "Autonomous Edge Intelligence System" to "Your Jarvis." That's the marker.

---

## Appendix A — File and Package Layout (proposed)

```
internal/
  proactive/           # NEW — Phase 1
    engine.go
    trigger.go         # interfaces + ManualTrigger + CronTrigger + EmailTrigger + CalendarTrigger
    action.go          # interfaces + RunAgentAction + ShellAction
    notifier.go        # interfaces + Telegram/Discord/Console/Push notifiers
    rule.go
    outbox.go          # SQLite-backed retrying notifier wrapper
    yaml_loader.go
  context/             # NEW — Phase 4
    store.go
    signals.go
  failover/            # NEW — Phase 2.2
    provider.go
  supervisor/          # NEW — Phase 2.3 (mostly platform service files)
mobile/                # NEW — Phase 3
  pwa/                 # React/Svelte source
  ios/                 # optional — Phase 3.5
```

## Appendix B — Open Questions Requiring Your Input

1. **Notifier priority for Phase 1.2**: Telegram + Discord first, or include WhatsApp (more complex via existing wacli)?
2. **Reachability for Phase 3.1**: Tailscale or self-hosted relay?
3. **PWA stack**: React (more ecosystem) or Svelte (smaller bundle on mobile)?
4. **Native iOS app**: build now in Phase 3 or defer until PWA limits prove blocking?
5. **Approval gate (5.3)**: which actions require it by default? (suggestion: anything tagged `shell`, anything touching money, anything sending outbound messages on your behalf)
