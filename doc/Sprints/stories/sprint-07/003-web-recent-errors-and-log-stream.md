# STORY-039 — Web UI: recent errors and correlated log view

| Field | Value |
|-------|--------|
| **Sprint** | sprint-07 |
| **Type** | Feature / UI |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Surface **recent errors** from structured logs (last N minutes): channel, error code, count, sample message. Allow jumping to correlation ID for full trace (logs panel or export).

## User story

**As an** operator**  
**I want** quick error visibility**  
**So that** I don’t grep disks via SSH.

## Scope

### In scope

- Server-side aggregation endpoint with rate limits.
- Redaction applied (STORY-021).

### Out of scope

- Full log search engine (Splunk); link only.

## Acceptance criteria

1. **Errors** list shows top errors with counts and last seen time.
2. **Clicking** an error copies correlation ID or opens detail view.
3. **Rate limit** prevents expensive queries; documented.
4. **Tests**: unit tests for aggregation; E2E smoke.
5. **Security**: no arbitrary file path log reading.

## Definition of Done

- [ ] Privacy review for error messages.

## Dependencies

- STORY-007.

## Risks

- Sensitive content in errors; scrub aggressively.
