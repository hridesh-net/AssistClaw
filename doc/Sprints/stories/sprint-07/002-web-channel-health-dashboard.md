# STORY-038 — Web UI: channel health dashboard

| Field | Value |
|-------|--------|
| **Sprint** | sprint-07 |
| **Type** | Feature / UI |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Dashboard showing **per-channel health**: connected/disconnected, last successful message, error rate, reconnect count, DLQ depth (from Sprint 2 metrics). Uses same metrics as Prometheus where possible.

## User story

**As an** operator**  
**I want** a single pane of glass for channels**  
**So that** I spot outages before users do.

## Scope

### In scope

- UI consumes JSON from gateway or metrics proxy (avoid exposing raw Prometheus publicly if unsafe).
- Status colors: green/yellow/red with thresholds from SLO doc.

### Out of scope

- Editing channel secrets in UI (security risk this sprint).

## Acceptance criteria

1. **Each enabled channel** appears with status.
2. **Thresholds** configurable or documented defaults.
3. **Drill-down** link to logs query (copy-paste or deep link).
4. **Tests**: E2E with mock metrics API.
5. **Performance**: page works with 10+ channels.

## Definition of Done

- [ ] On-call runbook references dashboard (STORY-012).

## Dependencies

- STORY-008 metrics.

## Risks

- Misleading green when metrics missing; show “unknown” state.
