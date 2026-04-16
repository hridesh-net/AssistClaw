# STORY-017 — Pre-flight checks before `assistclaw start` / gateway

| Field | Value |
|-------|--------|
| **Sprint** | sprint-03 |
| **Type** | Feature |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Run **`doctor` checks** (or a fast subset) automatically before starting the daemon/gateway, with `--skip-preflight` escape hatch for experts.

## User story

**As a** user**  
**I want** the service to refuse to start on broken config**  
**So that** I don’t think it’s running when it will fail.

## Scope

### In scope

- Hook: `start`, `gateway` (as applicable) call preflight.
- Fast mode: local validation only; optional `--preflight-full` for network.
- Clear error messages when preflight fails.

### Out of scope

- Automatic repair (doctor can suggest commands only).

## Acceptance criteria

1. [x] **Default** path runs preflight; documented behavior (README + command help).
2. [x] **Exit code** non-zero on failure; no partial listen on invalid config (preflight runs before `Detach` / `runAgent` / `gateway serve` load).
3. [x] **`--skip-preflight`** logs a single WARNING with security note (stderr).
4. [x] **Tests** for both success and failure paths (`cmd/assistclaw/preflight_test.go`).
5. [x] **Doc** updated in Quick Start (README Run section).

## Definition of Done

- [x] Metrics: `preflight_failures_total` (`internal/observability/metrics`; incremented on preflight failure).

## Dependencies

- STORY-013–016.

## Risks

- Slower startup; keep fast subset under 2s locally.
