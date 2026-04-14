# STORY-010 — Load test harness and failure injection

| Field | Value |
|-------|--------|
| **Sprint** | sprint-02 |
| **Type** | Test / Reliability |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Provide a **repeatable load test** (script or Go test) that simulates inbound messages and expected replies, plus **failure injection** hooks (adapter errors, slow responses) to validate retries, circuit breaker, and metrics.

## User story

**As a** developer**  
**I want** automated load and chaos tests**  
**So that** regressions in throughput and reliability are caught in CI or nightly jobs.

## Scope

### In scope

- Harness that can run locally and in CI (with reduced load).
- Scenarios: steady throughput, burst, sustained failure (breaker opens), recovery.
- Artifacts: summary output (latency percentiles, error rate).

### Out of scope

- Full k6-in-production; keep to staging profile.

## Acceptance criteria

1. **Runnable** via `make load-test` or documented command.
2. **CI** runs a **smoke** subset (short duration) on main or nightly.
3. **Failure injection** documented: how to simulate 429/500 from mock adapter.
4. **Baseline** numbers recorded in `doc/` (even if “initial baseline”) for future comparison.
5. **Exit criteria**: no panic, no goroutine leak (best-effort check), metrics counters move as expected.

## Definition of Done

- [x] Linked from on-call runbook for “performance regression” triage.

## Dependencies

- STORY-002, STORY-008.

## Risks

- Flaky CI timing; use generous thresholds for smoke mode.

## Implementation status

- [x] Load/failure-injection harness implemented in `internal/channels/adapter/load_failure_injection_test.go`.
- [x] Runnable via `make load-test`.
- [x] CI smoke workflow added at `.github/workflows/load-test-smoke.yml`.
- [x] Failure injection patterns documented for 429/500/permanent adapter errors.
- [x] Initial baseline file added in `doc/observability/load-test-baseline.md`.
- [x] Exit criteria checks included (no panic path, best-effort goroutine leak guard, metrics counter movement assertions).
