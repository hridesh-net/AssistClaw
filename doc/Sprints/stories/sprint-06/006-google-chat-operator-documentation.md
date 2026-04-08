# STORY-036 — Google Chat: operator documentation and troubleshooting

| Field | Value |
|-------|--------|
| **Sprint** | sprint-06 |
| **Type** | Documentation |
| **Priority** | P1 |
| **Estimate** | S |

## Summary

Write **operator docs**: install, uninstall, rotate credentials, interpret logs, common errors (403, wrong space, OAuth consent).

## User story

**As an** on-call engineer**  
**I want** Chat-specific troubleshooting**  
**So that** MTTR stays low.

## Scope

### In scope

- Runbook section in STORY-012 extended for Chat.
- FAQ for Workspace admins.

### Out of scope

- Google’s own SLA; link only.

## Acceptance criteria

1. **Troubleshooting** tree: symptom → cause → fix.
2. **Log snippets** examples (synthetic).
3. **Credential rotation** steps tested once (evidence).
4. **Escalation**: when to blame Google vs product.
5. **Version** compatibility statement.

## Definition of Done

- [ ] Support team sign-off.

## Dependencies

- STORY-034.

## Risks

- Google API changes; date the doc.
