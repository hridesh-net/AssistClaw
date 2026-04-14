# Load Test and Failure Injection

This runbook covers the sprint-02 load harness and failure injection patterns for reliability regression checks.

## Run locally

- Full scenario suite with summary:
  - `make load-test`
- Fast smoke profile:
  - `go test ./internal/channels/adapter -run TestLoadHarness_Smoke -count=1 -v`

## CI smoke

- Workflow: `.github/workflows/load-test-smoke.yml`
- Runs on:
  - pushes to `main`
  - nightly schedule

## Scenarios covered

- Steady throughput.
- Burst traffic.
- Sustained failure (circuit breaker open).
- Recovery after cooldown.

## Failure injection patterns

Simulate downstream API behavior in mock adapters:

- 429 / temporary API failure:
  - return `NewChannelError(ErrorKindRetryable, "...", err)`
- 500 / transient upstream failure:
  - return `NewChannelError(ErrorKindRetryable, "...", err)`
- permanent rejection:
  - return `NewChannelError(ErrorKindPermanent, "...", err)`

The harness validates retry/breaker behavior and confirms metrics counters move.

## Performance regression triage

1. Run smoke test first to confirm no panic/leak-level regression.
2. Run full load harness and compare p95/p99 with baseline doc:
   - `doc/observability/load-test-baseline.md`
3. If latency regresses:
   - inspect `adapter_retries_total`, `messages_failed_total`, `dlq_depth`
   - inspect structured logs by `request_id` and `session_id`
   - inspect tracing spans (`agent.model_call`, `adapter.send`) when tracing is enabled.
