# STORY-034 — Google Chat: E2E tests and permission denial scenarios

| Field | Value |
|-------|--------|
| **Sprint** | sprint-06 |
| **Type** | Test |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Execute **E2E** in a Google Workspace test project: install app, send messages, verify replies. Test **permission denial**: app removed, token revoked, user not in allowlist.

## User story

**As a** release owner**  
**I want** confidence in Chat reliability**  
**So that** we meet the same SLO bar as Teams.

## Scope

### In scope

- Manual + optional automated tests; evidence stored for release.
- Metrics compared to STORY-011 targets.

### Out of scope

- Multi-region latency optimization.

## Acceptance criteria

1. **E2E** evidence attached (logs with redaction).
2. **Denial tests**: user not allowlisted gets pairing or silent deny per policy.
3. **Revoked token** produces clear doctor failure and runtime error path.
4. **SLO** table filled for pilot.
5. **Regression** suite entry: “Chat must pass before tag.”

## Definition of Done

- [ ] Sprint demo includes live or recorded E2E.

## Dependencies

- STORY-031–033.

## Risks

- Workspace policy blocking tests; use dedicated test OU.
