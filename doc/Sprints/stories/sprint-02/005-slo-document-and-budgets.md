# STORY-011 — Internal SLO document and error budgets

| Field | Value |
|-------|--------|
| **Sprint** | sprint-02 |
| **Type** | Process / Docs |
| **Priority** | P0 |
| **Estimate** | S |

## Summary

Publish an **internal SLO document** defining targets for message delivery success, P95/P99 latency (inbound→reply), and availability of the gateway. Include **error budget** policy: what happens when budget is burned (freeze features, focus on reliability).

## User story

**As a** product/engineering lead**  
**I want** explicit SLOs**  
**So that** we prioritize reliability over feature churn when needed.

## Scope

### In scope

- SLO definitions with measurement sources (which metric names).
- Rolling windows (e.g. 30-day) and alerting thresholds.
- Error budget calculation and review cadence (monthly).

### Out of scope

- Customer-facing SLA (enterprise); internal first.

## Acceptance criteria

1. **Document** in `doc/` with version and owner — `doc/observability/internal-slo-error-budgets.md`.
2. **At least 3 SLOs**: delivery success %, P95 latency, gateway uptime.
3. **Each SLO** maps to dashboard panels (STORY-008) or explicit gap if metric missing.
4. **Error budget** policy written in one page.
5. **Sign-off** from engineering lead (table in doc; fill when reviewed).

**Verification (v1.1 doc):** Each SLO names the Grafana dashboard title and panel (“Messages Sent / Failed Rate”, “P95 Message Latency”, “Gateway Health”). P99 is documented as monitored-but-not-committed until baseline exists.

## Implementation status

- [x] Internal SLO + error budget document published (`doc/observability/internal-slo-error-budgets.md`, versioned header).
- [x] Linked from [metrics runbook](../../../runbooks/metrics-slo-indicators.md) and [on-call runbook](../../../runbooks/on-call-dashboards-alerts.md).

## Definition of Done

- [x] Shared in team channel / wiki (paste link to `doc/observability/internal-slo-error-budgets.md` on default branch, or internal wiki mirror).
- [x] Linked from STORY-012 runbook (`doc/runbooks/on-call-dashboards-alerts.md`).
- [x] Engineering lead sign-off row completed in the SLO document (name + date).

## Dependencies

- STORY-008 (metrics available).

## Risks

- Overpromising; start conservative and iterate.
