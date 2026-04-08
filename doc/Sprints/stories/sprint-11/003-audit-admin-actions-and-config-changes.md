# STORY-063 — Enterprise: audit log for admin actions and config changes

| Field | Value |
|-------|--------|
| **Sprint** | sprint-11 |
| **Type** | Compliance / Security |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Extend audit events for **who changed what**: RBAC role assignment, config updates, channel reconnect actions, policy toggles. Integrate with verify chain (STORY-020).

## User story

**As an** auditor**  
**I want** tamper-evident admin audit**  
**So that** we can prove compliance.

## Acceptance criteria

1. **Each mutating API** emits audit event with actor, action, resource, diff hash.
2. **No secrets** in diffs; redact values.
3. **Verification** tests updated for new event types.
4. **Export** JSON Lines for SIEM (minimal) or forward reference STORY-068.
5. **Retention** policy documented.

## Definition of Done

- [ ] SOC 2 readiness doc updated with access control evidence pointers.

## Dependencies

- STORY-020, STORY-062.

## Risks

- Large config objects; store hashed summary + path keys.
