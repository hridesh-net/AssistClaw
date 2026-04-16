# STORY-014 — `assistclaw doctor`: provider reachability

| Field | Value |
|-------|--------|
| **Sprint** | sprint-03 |
| **Type** | Feature |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Add **network checks** to doctor: for each configured LLM provider, perform a lightweight authenticated call (e.g. list models or minimal API) with timeouts; report DNS failures, TLS issues, auth failures.

## User story

**As a** user**  
**I want** doctor to tell me if my API keys work**  
**So that** I can fix auth before starting the daemon.

## Scope

### In scope

- Per-provider check functions with strict timeouts (e.g. 5–10s).
- Clear error messages: 401 vs 403 vs timeout.
- Optional `--skip-network` for air-gapped CI.

### Out of scope

- Billing verification.

## Acceptance criteria

1. **Each enabled provider** is checked or explicitly skipped with reason.
2. **No secrets** printed in output (mask tokens).
3. **Tests** use httptest mocks or recorded fixtures; no live keys in CI.
4. **Doc** explains corporate proxy requirements if any.
5. **Exit code** aligns with STORY-013 policy.

## Implementation status

- [x] `assistclaw doctor` calls `registerProviders` + [`Registry.CheckAll`](../../../../internal/provider/registry.go) (`HealthCheck` per provider) with a **45s** timeout, unless `--skip-network` (same as channel skips).
- [x] Check IDs: `provider.<name>`; messages use `sanitizeDoctorMessage` (`sk-…`, `Bearer …`); hints for 401/403/timeout/DNS-style errors.
- [x] Tests: `cmd/assistclaw/doctor_sanitize_test.go` (no live keys).
- [x] Doc: [doctor-config-validation.md](../../../runbooks/doctor-config-validation.md) — LLM section, proxy env vars, troubleshooting “API works but doctor fails”, exit code row updated.
- [x] CI unchanged: `doctor --skip-network` avoids provider calls in CI.

## Definition of Done

- [x] Runbook entry: “API works but doctor fails” troubleshooting (in `doc/runbooks/doctor-config-validation.md`).

## Dependencies

- STORY-013.

## Risks

- Rate limits; use minimal calls and cache nothing sensitive.
