# STORY-009 — OpenTelemetry tracing (optional)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-02 |
| **Type** | Observability |
| **Priority** | P2 |
| **Estimate** | M |

## Summary

Add **optional** OpenTelemetry traces for gateway ↔ channel ↔ agent spans to complement logs and metrics. Must be **off by default** or low-overhead to avoid impacting edge devices.

## User story

**As a** performance engineer**  
**I want** distributed traces**  
**So that** I can find slow segments in the pipeline.

## Scope

### In scope

- OTel SDK integration behind config flag `tracing.enabled`.
- Spans: receive message, enqueue, model call (if in scope), send reply, adapter send.
- Exporter: OTLP endpoint configurable.

### Out of scope

- Hosted Jaeger deployment; document docker-compose example only.

## Acceptance criteria

1. **Tracing** can be enabled without recompile; documented env vars.
2. **Sampling** configurable (e.g. parent-based, 1% default when on).
3. **No crash** if exporter unreachable; failures logged once.
4. **Doc**: how to run Jaeger locally and view traces.
5. **Tests**: unit test with in-memory exporter verifying span names exist.

## Definition of Done

- [x] Performance note: overhead when enabled at 1% sampling.

## Dependencies

- STORY-007 (correlation IDs) for trace/span linkage.

## Risks

- CPU overhead on Raspberry Pi; keep default off.

## Implementation status

- [x] Optional tracing config added under `tracing.*` (off by default).
- [x] OTel initialization wired at runtime in `cmd/assistclaw/main.go` without requiring recompile.
- [x] Sampling configurable via `tracing.sample_ratio` (1% default when enabled).
- [x] Exporter failures are non-fatal; warnings logged and runtime continues.
- [x] Spans added across gateway, runner, and adapter send paths.
- [x] Local Jaeger setup documented in `doc/runbooks/opentelemetry-tracing.md`.
- [x] In-memory span test verifies required span names.
