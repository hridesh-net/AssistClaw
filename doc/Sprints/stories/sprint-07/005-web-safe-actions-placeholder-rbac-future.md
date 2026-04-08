# STORY-041 — Web UI: safe actions (read-only first; RBAC hooks)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-07 |
| **Type** | Feature / Security |
| **Priority** | P2 |
| **Estimate** | M |

## Summary

Prepare **safe operational actions** for future RBAC: **reconnect channel**, **restart job**, **clear DLQ** (dangerous). For this sprint, ship **read-only** UI and **feature flags** for actions behind admin token + confirmation modal.

## User story

**As an** operator**  
**I want** safe recovery actions**  
**So that** I can restore service without CLI.

## Scope

### In scope

- Design API contracts for actions with idempotency keys.
- UI behind `gateway.ui.actions.enabled` or similar flag default **off**.
- Audit log entry for each action (STORY-020 pattern).

### Out of scope

- Full SSO RBAC (Sprint 11); use shared token + IP allowlist if needed.

## Acceptance criteria

1. **Flag off** → no action buttons visible.
2. **Flag on** → actions require confirmation + audit event.
3. **CSRF** / **token** protection for POST endpoints.
4. **Tests**: API tests for each action; permission denied without admin token.
5. **Doc** lists risks and rollback.

## Definition of Done

- [ ] Security review before enabling in beta.

## Dependencies

- STORY-020 audit.

## Risks

- Dangerous actions; default off.
