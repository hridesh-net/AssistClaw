# Sprint-01 Story-002: Outbound reliability (retry, breaker, DLQ)

## Story reference

- Source: `doc/Sprints/stories/sprint-01/002-outbound-reliability-retries-dlq.md`
- Goal: add shared outbound reliability for adapter sends.

## What was implemented

- Added shared reliability wrapper in adapter layer:
  - exponential backoff with jitter
  - retry policy using adapter error kinds
  - circuit breaker with open/half-open/closed transitions
  - DLQ write on terminal failures
- Added configuration block under `channels.outbound`:
  - `max_attempts`
  - `base_delay_ms`
  - `max_delay_ms`
  - `jitter_percent`
  - `breaker_threshold`
  - `breaker_cooldown_s`
  - `dlq_path`
- Added defaults and validation in config loader.
- Wired reliability into outbound message tool path through adapter wrappers in main runtime.
- Added operator command:
  - `assistclaw dlq list --limit <n>`
- Added runbook for incident usage and manual replay guidance.

## Key files

- `internal/channels/adapter/reliability.go`
- `internal/channels/adapter/reliability_test.go`
- `internal/config/config.go`
- `cmd/assistclaw/main.go`
- `cmd/assistclaw/dlq_cmd.go`
- `.assistclaw.yaml.example`
- `doc/runbooks/dlq-inspection.md`

## Acceptance criteria mapping

1. Retry policy configurable in YAML: **done**
2. Idempotent-send interaction documented and enforced via idempotency key carriage: **done (MVP)**
3. Circuit breaker prevents hammering and logs transitions: **done**
4. DLQ persists failures with replay context: **done**
5. Unit tests for retry/breaker/DLQ: **done**
6. Integration-style behavior checks with flaky sender patterns: **done (via deterministic adapter tests)**

## Tests and validation

- Added tests covering:
  - retry then success
  - permanent failure -> DLQ write
  - breaker transitions and half-open probe behavior
- Full project test suite passes after integration.

## Follow-ups

- Add metrics counters (retry count, breaker opens, DLQ size) in sprint-02 metrics work.
- Add replay tooling (`assistclaw dlq replay`) as a later enhancement.
- Add retention policy automation (rotation/truncation) beyond manual ops runbook.
