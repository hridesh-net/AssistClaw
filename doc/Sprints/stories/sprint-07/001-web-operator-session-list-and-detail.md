# STORY-037 — Web UI: session list and session detail (operator)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-07 |
| **Type** | Feature / UI |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Add or improve **gateway web UI** to list active sessions (agents/conversations) with metadata: last activity, channel, model, token usage summary. Session detail shows recent transcript excerpts (redacted) and tool call summary.

## User story

**As a** support engineer**  
**I want** to see sessions without SSH**  
**So that** I can debug user issues quickly.

## Scope

### In scope

- Read-only by default; auth model aligns with gateway token (document).
- Pagination and search by session ID.
- Privacy: PII masking toggle or role-based (stretch).

### Out of scope

- Full transcript export (compliance epic; link only).

## Acceptance criteria

1. **Sessions** list loads in &lt; 2s for 1k sessions (or documented limits).
2. **Detail** view shows last N messages with configurable N (admin setting).
3. **Authorization**: unauthenticated users cannot access (if gateway requires token, enforce in UI).
4. **Tests**: E2E against local gateway; accessibility for core table (labels, headers).
5. **Docs**: how to access UI URL and token.

## Definition of Done

- [ ] Support team validates triage flow (STORY-042).

## Dependencies

- Gateway exposes APIs or existing endpoints; may need new JSON APIs.

## Risks

- Large transcripts; enforce limits and warnings.
