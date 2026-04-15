# STORY-008 — Metrics: delivery, latency, queue depth, reconnects

| Field | Value |
|-------|--------|
| **Sprint** | sprint-02 |
| **Type** | Observability |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Expose **Prometheus-compatible metrics** (or agreed format) for: outbound/inbound message success and failure counts, latency histograms, DLQ depth, retry counts, channel reconnect events, and gateway health.

## User story

**As an** operator**  
**I want** dashboards and alerts**  
**So that** I meet SLOs and catch outages early.

## Scope

### In scope

- Counters/histograms with low-cardinality labels (avoid unbounded `user_id` labels).
- `/metrics` endpoint or documented scrape integration.
- Default Grafana dashboard JSON in `doc/` or `deploy/` (optional but valuable).

### Out of scope

- Full SaaS-hosted Grafana; provide exportable JSON only.

## Acceptance criteria

1. **Metrics** include at minimum: `messages_sent_total`, `messages_failed_total`, `message_latency_seconds` (histogram), `dlq_depth`, `channel_reconnects_total`, `adapter_retries_total`.
2. **Labeling policy** documented: allowed labels; forbidden high-cardinality patterns.
3. **Dashboard** (JSON or screenshot + import path) shows golden signals for one channel.
4. **Alerting examples**: PagerDuty/Opsgenie-style rules as YAML snippets (optional).
5. **Tests**: unit tests for metric registration (no duplicate panics) and that critical paths increment counters.

## Definition of Done

- [x] Linked from internal SLO doc (STORY-011) — see `doc/observability/internal-slo-error-budgets.md`.
- [ ] Reviewed for cardinality by observability owner.

## Dependencies

- STORY-002 (DLQ/retry) for meaningful metrics.

## Risks

- Metric explosion; enforce label allowlist in code review.

## Implementation status

- [x] Prometheus-compatible metrics endpoint exposed at `/metrics` in gateway.
- [x] Required metrics implemented: `messages_sent_total`, `messages_failed_total`, `message_latency_seconds`, `dlq_depth`, `channel_reconnects_total`, `adapter_retries_total`.
- [x] Label policy documented with low-cardinality allowlist and forbidden labels.
- [x] Dashboard JSON added at `doc/observability/grafana-sprint-02-channel-golden-signals.json`.
- [x] Alert examples documented in `doc/runbooks/metrics-slo-indicators.md`.
- [x] Unit tests added for singleton registration safety and critical counter/histogram increments.
