# STORY-062 — Enterprise: RBAC roles and resource scopes

| Field | Value |
|-------|--------|
| **Sprint** | sprint-11 |
| **Type** | Enterprise / Security |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Implement **RBAC**: roles such as `admin`, `operator`, `viewer`; **resource scopes** for channels, sessions, policies; enforce on HTTP APIs and UI.

## User story

**As a** security admin**  
**I want** least privilege**  
**So that** operators cannot change secrets.

## Scope

### In scope

- Policy matrix: role × action (read sessions, reconnect channel, edit config).
- Optional mapping from IdP groups to roles (STORY-061).
- Deny-by-default for new endpoints.

### Out of scope

- Fine-grained ABAC (Sprint 12 expands).

## Acceptance criteria

1. **Enforcement** on all operator endpoints (audit missing routes).
2. **Tests**: table-driven authorization tests; ensure 403 for wrong role.
3. **UI** hides controls user cannot use (not only server-side).
4. **Bootstrap** first admin creation documented (break-glass).
5. **Doc** RBAC model with examples.

## Definition of Done

- [ ] Enterprise pilot requirement “only admins manage channels” satisfied.

## Dependencies

- STORY-061.

## Risks

- Role creep; keep minimal roles v1.
