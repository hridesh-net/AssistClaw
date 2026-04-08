# Future Updates & Releases — Sprint Execution Plan

This document turns the competitive roadmap (channel breadth, client surface, perceived maturity, enterprise grade) into a **sprint-by-sprint** execution plan. Sprints are assumed **two weeks** unless your team uses a different cadence—renumber dates accordingly.

**Reference matrix:** weekly scoreboard on channel breadth, reliability, clients, onboarding, docs, enterprise security, ops, and ecosystem (see internal planning docs).

---

## Release trains

| Train | Theme | Target window |
|-------|--------|----------------|
| **R1** | Foundations + reliability | Sprints 1–4 |
| **R2** | Channels + onboarding + observability | Sprints 5–8 |
| **R3** | Client surface + enterprise core | Sprints 9–12 |
| **R4** | Scale, compliance, GA hardening | Sprints 13+ |

Version tags (example): `v4.0.0` (R1 GA), `v4.1.0` (R2), `v5.0.0` (R3 enterprise), `v5.1.0` (R4).

---

## Sprint 1 — Adapter foundations & reliability primitives

**Goal:** One internal “channel adapter” contract so new integrations are fast and testable.

**Implement**

- Define adapter interface: send/receive, media, threading, idempotency keys, rate-limit hooks.
- Shared retry/backoff, circuit breaker, and dead-letter queue for failed outbound messages.
- Channel capability registry (what each channel supports).

**Test**

- Unit tests for retry/idempotency; property tests for ordering where applicable.
- Mock-based contract tests for the adapter boundary.

**Done when**

- At least one existing channel (e.g. Telegram) migrated behind the adapter without user-visible regressions.
- Documented adapter checklist in `doc/` or contributor guide.

---

## Sprint 2 — Observability baseline & SLO definitions

**Goal:** Measure before you optimize; define internal SLOs and dashboards.

**Implement**

- Structured logging (request/channel/session correlation IDs).
- Metrics: send success/failure, latency histograms, queue depth, reconnect counts.
- Optional: OpenTelemetry traces for gateway ↔ channels.

**Test**

- Load test harness (synthetic traffic) with failure injection.
- Dashboard smoke tests or snapshot of key panels.

**Done when**

- Published internal SLO doc: e.g. outbound delivery success target, P95 reply latency.
- On-call runbook: how to read dashboards and common alerts.

---

## Sprint 3 — Onboarding v2 (discovery & diagnostics)

**Goal:** Shorter time-to-first-message (TTFM); fewer “it works on my machine” failures.

**Implement**

- `assistclaw doctor` v1: config validation, provider reachability, channel token checks.
- Non-interactive mode for CI and support (`--json` output optional).
- Pre-flight checks before starting daemon/gateway.

**Test**

- Fresh install tests (scripted) on Linux + macOS (Windows if in scope).
- Golden output snapshots for `doctor` in CI.

**Done when**

- Median TTFM improvement measurable vs baseline (target: document % improvement).
- 90%+ of scripted installs pass doctor without manual fixes (stretch).

---

## Sprint 4 — Security hardening for multi-channel & enterprise prep

**Goal:** Enterprise buyers care about defaults, audit, and least privilege.

**Implement**

- Per-channel allowlists / pairing flows aligned with untrusted DM model.
- Audit log coverage for channel connect/disconnect and policy denials (extend existing audit if present).
- Secrets: document env vs file vs future vault; redact secrets in logs.

**Test**

- Security regression suite: injection attempts via channel payloads (where applicable).
- Audit chain verification still passes after new event types.

**Done when**

- Security checklist signed off for “pilot enterprise” (internal).
- No secrets in default debug output.

---

## Sprint 5 — New channel: Microsoft Teams (or #1 priority from backlog)

**Goal:** Prove adapter + reliability on a net-new enterprise integration.

**Implement**

- Teams bot: messaging, threading, attachments (phase 1 scope explicit).
- Config schema + onboarding steps + docs.

**Test**

- Sandbox tenant E2E; webhook/credential rotation test.
- Failure modes: token expiry, throttling.

**Done when**

- Meets channel SLO for pilot (e.g. delivery success & P95 latency targets from Sprint 2).
- User-facing doc: setup in < 30 minutes for someone with admin access.

---

## Sprint 6 — New channel: Google Chat (or #2 priority)

**Goal:** Second high-demand connector; reuse adapter patterns.

**Implement**

- Google Chat app install path, scopes, and message mapping.
- Unified “group/space” policy model (documented).

**Test**

- E2E in test workspace; permission denial tests.

**Done when**

- Same SLO bar as Sprint 5 for scoped features.
- Cross-channel parity matrix updated (threading, files, reactions).

---

## Sprint 7 — Web / gateway UX for operators

**Goal:** “Perceived maturity” via a usable control surface (even before native apps).

**Implement**

- Web UI improvements: session list, channel health, recent errors, model usage (as applicable).
- Read-only operator views first; then safe actions (restart job, reconnect channel) behind RBAC later.

**Test**

- E2E against local gateway; accessibility pass on core flows.

**Done when**

- Support team can triage “user can’t receive messages” without SSH (where deployment allows).

---

## Sprint 8 — Docs & examples “golden paths”

**Goal:** Beat OpenClaw on clarity for common setups.

**Implement**

- Five golden paths: small team, enterprise pilot, air-gapped, edge (Pi), cloud hybrid.
- CI: link check + optional snippet execution for copy-paste blocks.
- Troubleshooting index tied to `doctor` error codes.

**Test**

- Docs PR must pass CI gates; quarterly manual review of top 10 pages.

**Done when**

- Zero broken links on release branch; snippet pass rate tracked weekly.

---

## Sprint 9 — macOS companion (beta)

**Goal:** Client surface depth: menu bar / quick actions / notifications.

**Implement**

- macOS app: connect to gateway, show session/status, notifications for inbound messages (scope MVP).
- Secure device pairing if applicable to your architecture.

**Test**

- TestFlight/beta distribution process; crash reporting.
- Sleep/wake, VPN, and reconnect tests.

**Done when**

- Crash-free sessions ≥ target (e.g. 99.5%) in beta cohort.
- Published install + known limitations.

---

## Sprint 10 — Mobile companion (alpha)

**Goal:** Parity for read/reply and attachments on iOS/Android (pick one stack).

**Implement**

- Alpha: login/pair, chat list, send/receive, push notifications (best-effort).
- Attachment limits and policy messaging.

**Test**

- Device lab or cloud device farm for top OS versions.
- Background delivery tests.

**Done when**

- Internal dogfood + small external alpha; no P0 crashes on core path.

---

## Sprint 11 — Enterprise auth & RBAC v1

**Goal:** OIDC/SSO and role-based access for admin vs operator vs user.

**Implement**

- OIDC SSO for web/admin surfaces.
- RBAC: roles, resource scopes (channels, tenants, sessions).
- Audit: who changed what (config, policies, role assignments).

**Test**

- SSO mock tests; token expiry/refresh; revoked user behavior.

**Done when**

- Enterprise pilot can enforce “only admins manage channels.”
- SOC 2 **readiness** checklist started (access control + logging evidence).

---

## Sprint 12 — Policy engine & tenant isolation (v1)

**Goal:** Data/tool egress policy and multi-tenant boundaries.

**Implement**

- Policy engine v1: tool allow/deny, channel restrictions, optional OPA/Cedar later.
- Tenant isolation tests for data stores and config paths.

**Test**

- Negative tests: cross-tenant access must fail closed.
- Performance tests under policy evaluation load.

**Done when**

- Isolation guarantees documented; no critical findings in internal review.

---

## Sprint 13+ — HA, DR, compliance packaging

**Goal:** Enterprise-grade operations and certification readiness.

**Implement**

- HA deployment guide: stateless gateways, durable queues, backup/restore.
- SIEM export (e.g. Splunk HTTP Event Collector, Datadog, generic JSON Lines).
- Key management integrations (Vault / cloud KMS) as prioritized by customers.

**Test**

- DR drill: restore from backup; verify message continuity expectations.
- Quarterly pen-test and dependency audit pipeline.

**Done when**

- Target uptime and MTTR commitments documented for enterprise SKUs.
- Path to SOC 2 Type I evidence complete; Type II program scheduled.

---

## Cross-cutting testing matrix (every sprint)

| Area | Minimum bar |
|------|-------------|
| Unit / integration | All new code paths covered; flaky tests quarantined |
| E2E | Golden path per changed channel or client |
| Security | No new high/critical issues; secrets scanning clean |
| Performance | No regression beyond agreed budget on gateway hot paths |
| Docs | User-visible changes documented in same PR or release notes |

---

## Milestone checklist (release gates)

Before tagging a **minor** release (e.g. `v4.x`):

- [ ] CHANGELOG updated with breaking changes and migrations.
- [ ] SLO dashboards green for 7 days on main dogfood environment.
- [ ] Onboarding + doctor pass on fresh VMs for supported OS list.
- [ ] Security + audit spot check complete.

Before tagging **enterprise** (`v5.x`):

- [ ] SSO/RBAC/policy docs and tests complete.
- [ ] Tenant isolation validation signed off.
- [ ] Support runbooks + incident response for P1/P2.
- [ ] Customer-facing SLA draft (even if “best effort” initially).

---

## How to use this file

1. Import each sprint as an **epic** in your tracker; break into stories with the **Done when** clauses as acceptance criteria.
2. Re-order Sprints 5–6 channel choices based on **customer demand**.
3. Attach **release train** targets so marketing and support know what “R2” means.

**Sprint story tickets (detailed):** see [Sprints/stories/README.md](../../Sprints/stories/README.md) (STORY-001–074).

---

*Last updated: 2026-04-08*
