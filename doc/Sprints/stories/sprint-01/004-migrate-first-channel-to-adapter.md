# STORY-004 — Migrate first channel behind adapter (e.g. Telegram)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-01 |
| **Type** | Feature / Refactor |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Migrate **one** production channel (recommended: **Telegram**) to implement the new adapter interface and use the shared outbound reliability layer (STORY-002). Prove no user-visible regressions for existing users.

## User story

**As a** Telegram user of AssistClaw**  
**I want** the same behavior as before**  
**So that** the refactor does not break my workflows.

## Scope

### In scope

- Telegram bot path uses adapter interface end-to-end.
- Wire retries/DLQ for outbound Telegram sends.
- Feature parity with pre-migration behavior for: text, attachments supported today, commands if any.

### Out of scope

- Migrating WhatsApp/Discord/Slack in this sprint (optional stretch if time-boxed).

## Acceptance criteria

1. **Functional parity**: manual test checklist executed (send, receive, reply, attachment if supported, error handling).
2. **Automated tests**: existing Telegram tests updated; new tests for adapter + retry path using mocks.
3. **No regressions**: existing integration tests pass; CI green.
4. **Config migration**: if any config keys change, provide migration note + backward compatibility or one-time warning.
5. **Rollback plan** documented in PR: how to revert if issues arise in release.

## Definition of Done

- [ ] Code reviewed with channel owner.
- [ ] CHANGELOG entry under “Refactor” or “Internal”.

## Dependencies

- STORY-001, STORY-002, STORY-003.

## Risks

- Subtle API timing differences; mitigate with staged rollout or feature flag if available.
