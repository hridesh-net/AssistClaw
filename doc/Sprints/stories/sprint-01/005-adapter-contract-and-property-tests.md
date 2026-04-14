# STORY-005 — Adapter contract tests and property tests

| Field | Value |
|-------|--------|
| **Sprint** | sprint-01 |
| **Type** | Test / Quality |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Add **contract tests** that validate any adapter implementation against the expected behavior of STORY-001, plus **property-based tests** where ordering guarantees (e.g. per-thread ordering) are declared.

## User story

**As a** maintainer**  
**I want** automated guarantees that adapters behave consistently  
**So that** new channels (Teams, Chat) don’t ship subtle bugs.

## Scope

### In scope

- Test harness that drives a mock or in-memory adapter through required scenarios: start/stop, send success, send retryable failure, permanent failure.
- Property tests (optional but recommended): e.g. idempotency key uniqueness, monotonic timestamps for a single thread if applicable.
- CI job runs these tests on every PR.

### Out of scope

- Full E2E with real Telegram (Sprint 5+ style); use mocks here.

## Acceptance criteria

1. **Contract test suite** runs in CI &lt; 2 minutes for the package.
2. **Clear failure messages** when a new adapter violates the contract.
3. **Coverage** for error classification paths (retryable vs permanent).
4. **Documentation** in `doc/` or CONTRIBUTING: “how to run contract tests locally.”
5. If property tests are included: **documented properties** and minimum seed/fuzz count.

## Definition of Done

- [ ] Required check for merge to main. _(Workflow added in `.github/workflows/adapter-contract-tests.yml`; branch protection still needs to mark it as required.)_
- [x] Example of adding a new adapter to the suite (snippet added to `CONTRIBUTING.md`).

## Implementation status

- [x] Contract harness added in `internal/channels/adapter/contract_property_test.go`.
- [x] Contract scenarios covered: start/stop, send success, retryable failure classification, permanent failure classification.
- [x] Property tests added with deterministic seed and minimum fuzz count (`seed=20260414`, `count=300`).
- [x] CI job added to run contract/property tests on PRs in under 2 minutes (`timeout-minutes: 2`).
- [x] Local run documentation added in `CONTRIBUTING.md`.

## Dependencies

- STORY-001 (interface stable enough to test).

## Risks

- Flaky property tests; use deterministic RNG seeds in CI.
