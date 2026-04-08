# STORY-002 — Outbound reliability: retry, backoff, circuit breaker, DLQ

| Field | Value |
|-------|--------|
| **Sprint** | sprint-01 |
| **Type** | Feature / Reliability |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Implement shared **outbound message reliability** for channel adapters: exponential backoff with jitter, a **circuit breaker** when a downstream API is unhealthy, and a **dead-letter queue (DLQ)** path for messages that cannot be delivered after retries.

## Background

Enterprise messaging requires predictable delivery behavior and operability when a provider is down. Duplicates and silent drops are unacceptable; this layer centralizes policy.

## User story

**As an** operator**  
**I want** failed outbound sends to retry safely and surface failures clearly  
**So that** users trust the assistant and we can debug incidents.

## Scope

### In scope

- Configurable retry policy (max attempts, base delay, max delay, jitter).
- Circuit breaker: open after N failures; half-open probe; configurable thresholds.
- DLQ: persist failed payloads with reason, channel, idempotency key, timestamp; admin listing or log export (minimal MVP: durable store + file or DB table).
- Integration point: **only** used by adapter outbound path (not scattered per channel).

### Out of scope

- Automatic replay UI (can be a follow-up); MVP may be CLI `assistclaw dlq list` or SQL query documented.
- Cross-region failover (Sprint 13).

## Acceptance criteria

1. **Retry policy** is configurable via `assistclaw.yaml` (or env) with sane defaults documented.
2. **Idempotent sends**: same logical message is not duplicated on retry unless the adapter explicitly allows (document interaction with STORY-001 idempotency keys).
3. **Circuit breaker** prevents hammering a failing API; logs state transitions at INFO/WARN.
4. **DLQ** persists failures with enough context to replay manually; retention policy documented.
5. **Unit tests** cover retry timing (with fake clock if used), breaker transitions, and DLQ write.
6. **Integration test** with mock adapter that fails N times then succeeds (verifies retry) and fails permanently (verifies DLQ).

## Definition of Done

- [ ] Metrics hooks for retry count and DLQ size (counters) — can be no-op if Sprint 2 implements full metrics.
- [ ] Runbook snippet: “how to inspect DLQ after incident.”

## Dependencies

- STORY-001 (adapter interface).

## Risks

- Storage growth from DLQ; require retention + max size guardrails.
