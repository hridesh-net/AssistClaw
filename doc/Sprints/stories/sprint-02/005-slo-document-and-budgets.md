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

## Implementation status

- [x] Internal SLO + error budget document published.
- [x] Linked from metrics runbook and on-call runbook.

## Definition of Done

- [ ] Shared in team channel / wiki.
- [x] Linked from STORY-012 runbook (`doc/runbooks/on-call-dashboards-alerts.md`).

## Dependencies

- STORY-008 (metrics available).

## Risks

- Overpromising; start conservative and iterate.
