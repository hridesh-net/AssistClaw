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
- [x] Linked from README or `doc/` index — [README.md](../../../../README.md) and [runbooks index](../../../../doc/runbooks/README.md).

## Dependencies

- STORY-007, STORY-008, STORY-011.

## Implementation status

- [x] Runbook: [`doc/runbooks/on-call-dashboards-alerts.md`](../../../../doc/runbooks/on-call-dashboards-alerts.md) — scenarios (DLQ, reconnect storm, high latency, gateway down, **disk full**), step-by-step `assistclaw` + `curl` commands, alert→action table, ownership, quarterly drill template.
- [x] Runbooks index: [`doc/runbooks/README.md`](../../../../doc/runbooks/README.md).

## Acceptance criteria (verification)

1. [x] Runbook covers high DLQ, reconnect storm, high latency, disk full on `state_dir`.
2. [x] Each scenario includes concrete commands (`assistclaw`, `curl`, optional `du`/`df`/`tail`).
3. [x] Alert rules listed with page vs ticket (plus recommended latency alert row).
4. [x] Ownership table (channels vs core gateway).
5. [x] Quarterly drill: template to record first drill outcome (`on-call-dashboards-alerts.md` § Quarterly drill).

## Risks

- Stale commands; version the runbook with AssistClaw version.
