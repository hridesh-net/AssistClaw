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

1. **Default** path runs preflight; documented behavior.
2. **Exit code** non-zero on failure; no partial listen on invalid config.
3. **`--skip-preflight`** logs a single WARNING with security note.
4. **Tests** for both success and failure paths.
5. **Doc** updated in Quick Start.

## Definition of Done

- [ ] Metrics: `preflight_failures_total` (optional Sprint 2 alignment).

## Dependencies

- STORY-013–016.

## Risks

- Slower startup; keep fast subset under 2s locally.
