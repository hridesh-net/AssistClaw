# STORY-012 — On-call runbook: dashboards, alerts, triage

| Field | Value |
|-------|--------|
| **Sprint** | sprint-02 |
| **Type** | Operations |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Write an **on-call runbook**: how to use dashboards, interpret metrics, common alerts, escalation, and first-response steps for channel outages, DLQ growth, and gateway crashes.

## User story

**As an** on-call engineer**  
**I want** a concise runbook**  
**So that** I can restore service without reading the whole codebase.

## Scope

### In scope

- Symptom → likely cause → checks → mitigation.
- Links to: logs (correlation ID), metrics panels, DLQ inspection.
- Severity definitions aligned with incident process.

### Out of scope

- 24/7 vendor contract; internal ops only.

## Acceptance criteria

1. **Runbook** covers: high DLQ rate, channel reconnect storm, high latency, disk full on state dir.
2. **Each scenario** has step-by-step commands (`assistclaw` subcommands, curl, SQL if any).
3. **Alert rules** listed with intended action (page vs ticket).
4. **Ownership**: who owns channel integrations vs core gateway.
5. **Quarterly drill**: scheduled (calendar invite optional); document first drill outcome.

## Definition of Done

- [ ] Reviewed by someone not on the core team (fresh eyes).
- [ ] Linked from README or `doc/` index.

## Dependencies

- STORY-007, STORY-008, STORY-011.

## Implementation status

- [x] Runbook: `doc/runbooks/on-call-dashboards-alerts.md` (links SLO doc, metrics, DLQ, logging).

## Risks

- Stale commands; version the runbook with AssistClaw version.
