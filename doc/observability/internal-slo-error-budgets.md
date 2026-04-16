# AssistClaw — Internal SLOs and error budgets

| Field | Value |
|-------|--------|
| **Version** | 1.1 |
| **Last updated** | 2026-04-16 |
| **Owner** | AssistClaw engineering lead |
| **Audience** | Internal engineering / ops (not customer SLA) |

## Purpose

Define measurable reliability targets for AssistClaw in production-like deployments, tie them to existing metrics (STORY-008), and state **error budget** policy so we can pause feature work when reliability regresses.

## Measurement sources

- **Metrics:** Prometheus scrape of gateway `GET /metrics` — see [Metrics and SLO indicators](../runbooks/metrics-slo-indicators.md) for names, labels, and scrape auth.
- **Dashboards:** Grafana import `doc/observability/grafana-sprint-02-channel-golden-signals.json` (golden signals for one channel).
- **Logs:** Structured logs with correlation IDs — [Structured logging runbook](../runbooks/structured-logging-correlation-ids.md).

## Rolling window and review

- **SLO evaluation window:** 30 calendar days, rolling (each day recompute over trailing 30d).
- **Review cadence:** Monthly engineering review of SLO compliance and burn rate; adjust targets only with written rationale (avoid silent drift).

## SLO definitions

### SLO 1 — Outbound message delivery success

- **Target:** ≥ **99.0%** of outbound send attempts succeed over 30d (per deployment / scrape target).
- **Good events:** `messages_sent_total` increments (success path).
- **Valid requests:** `messages_sent_total + messages_failed_total` (attempts).
- **SLI:** `sum(increase(messages_sent_total[30d])) / sum(increase((messages_sent_total + messages_failed_total)[30d]))`.
- **Dashboard (STORY-008):** Grafana dashboard `AssistClaw Channel Golden Signals (Sprint-02)` — panel **“Messages Sent / Failed Rate”** (`doc/observability/grafana-sprint-02-channel-golden-signals.json`).
- **Gap:** If per-channel SLO is required, duplicate panels filtered by `channel` label; single-job scrape must still use low-cardinality labels only.

### SLO 2 — Outbound send latency (P95)

- **Target:** **P95** of `message_latency_seconds` bucket **≤ 10s** over 30d for outbound sends (excluding cold starts / first message after idle — document exclusions in incident notes if needed).
- **SLI:** Prometheus histogram `histogram_quantile(0.95, sum(rate(message_latency_seconds_bucket[5m])) by (le, channel))` aggregated over the window (implementation-specific recording rules recommended).
- **Dashboard (STORY-008):** Same dashboard — panel **“P95 Message Latency”**.
- **P99 (not an SLO yet):** Track `histogram_quantile(0.99, …)` in Grafana for regression visibility; no committed target until we have a baseline from production-like traffic. **Gap:** add a dedicated P99 panel or recording rule when burn patterns justify a fourth SLO.
- **Gap:** If gateway and worker are split processes, ensure scrape covers the component that observes latency; otherwise SLO is “gateway-observed latency” only.

### SLO 3 — Gateway availability

- **Target:** **99.5%** monthly uptime of the HTTP gateway process from the perspective of the metrics endpoint (or health check equivalent).
- **SLI:** Fraction of 1-minute intervals in which `gateway_health == 1` (or successful `GET /metrics` with 200) over the month.
- **Dashboard (STORY-008):** Same dashboard — panel **“Gateway Health”** (supplement with external blackbox or uptime checks if your deployment exposes the gateway behind TLS or a reverse proxy).
- **Gap:** `gateway_health` reflects in-process state; use synthetic probes for TLS / external routing if those layers are in scope for your deployment.

## Alerting thresholds (leading indicators)

These are **not** SLOs themselves; they warn before budget exhaustion. Align with [alert examples](../runbooks/metrics-slo-indicators.md):

- Failure ratio elevated for 10m (see `AssistClawHighFailureRate`).
- DLQ depth sustained (see `AssistClawDLQGrowing`).
- Reconnect or retry storms: `rate(channel_reconnects_total[15m])` or `rate(adapter_retries_total[15m])` above a baseline (set per environment).

## Error budget policy (one page)

**Budget:** For each SLO, error budget = **1 − SLO target** over the same window (e.g. 1% bad delivery for a 99% SLO over 30d).

**Burn signals:**

- **> 50% of monthly budget consumed in any 7 consecutive days** → **Freeze** non-critical feature merges; reliability fixes and SEV-1/2 only until 7d burn returns below 25% projected monthly burn.
- **≥ 100% of monthly budget consumed** (SLO missed for the window) → **Stop** release train; run a **blameless postmortem** within 5 business days; no production promotions until mitigation owners sign off.
- **Budget healthy** (> 50% budget remaining at mid-month) → normal feature velocity.

**Escalation:** On-call follows [On-call: dashboards, alerts, triage](../runbooks/on-call-dashboards-alerts.md). Engineering lead approves any SLO target change or policy exception.

## Sign-off

Complete after review (names and dates are required for STORY-011 “Definition of Done” on the sprint story).

| Role | Name | Date |
|------|------|------|
| Engineering lead | *pending — fill on review* | |

---

*Linked from: [On-call runbook](../runbooks/on-call-dashboards-alerts.md). Metrics detail: [metrics-slo-indicators](../runbooks/metrics-slo-indicators.md).*
