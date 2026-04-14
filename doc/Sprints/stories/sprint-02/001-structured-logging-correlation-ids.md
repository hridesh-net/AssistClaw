# STORY-007 — Structured logging and correlation IDs

| Field | Value |
|-------|--------|
| **Sprint** | sprint-02 |
| **Type** | Observability |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Standardize **structured logging** across gateway, channels, and agent runner with **correlation IDs**: `request_id`, `session_id`, `channel`, `trace_id` (optional), so logs can be joined in Loki/Datadog/Splunk.

## User story

**As an** SRE**  
**I want** to filter all logs for one user request**  
**So that** I can debug production issues quickly.

## Scope

### In scope

- Log schema (JSON or key=value) documented.
- Middleware or context propagation for `request_id` on inbound HTTP/WS and channel events.
- Consistent field names across packages.

### Out of scope

- Log shipping agents (customer infra); document recommended agents only.

## Acceptance criteria

1. **Every inbound message path** attaches a `request_id` (or equivalent) to context and includes it in logs for send/receive/tool execution.
2. **Documentation** lists all standard fields and levels (DEBUG vs INFO for noisy paths).
3. **No PII by default**: document redaction for phone numbers, tokens (align Sprint 4).
4. **Unit tests** verify correlation ID present on synthetic requests.
5. **Performance**: logging overhead benchmarked; no &gt;5% regression on hot path (or justified).

## Definition of Done

- [ ] On-call runbook references correlation ID usage (STORY-012).
- [x] Sample queries for one target backend (e.g. “filter by request_id”).

## Dependencies

- None.

## Risks

- Log volume explosion; add sampling for DEBUG if needed.

## Implementation status

- [x] Correlation context utility added in `internal/observability/correlation/`.
- [x] Gateway HTTP/WS inbound paths now preserve/generate `request_id` and propagate context.
- [x] Runner and tool execution logs include standard correlation fields (`request_id`, `session_id`, `channel`, `trace_id` when present).
- [x] Unit tests added for synthetic correlation propagation and request-id behavior.
- [x] Logging schema + sample backend queries documented in `doc/runbooks/structured-logging-correlation-ids.md`.
- [x] Added benchmark for correlation field extraction overhead (`BenchmarkFields`).
