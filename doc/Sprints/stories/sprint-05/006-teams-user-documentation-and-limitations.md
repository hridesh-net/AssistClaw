# STORY-030 — Microsoft Teams: user-facing documentation and limitations

| Field | Value |
|-------|--------|
| **Sprint** | sprint-05 |
| **Type** | Documentation |
| **Priority** | P1 |
| **Estimate** | S |

## Summary

Publish **user-facing** Teams documentation: what works in phase 1, commands, limitations (no private channel X if unsupported), privacy, data residency notes (customer responsibility).

## User story

**As a** customer admin**  
**I want** accurate limitations**  
**So that** I can set expectations with users.

## Scope

### In scope

- `doc/channels/teams.md` (path illustrative).
- FAQ: multi-tenant, guest access, compliance mode.
- Link from main README channel matrix.

### Out of scope

- Legal DPA wording (legal team).

## Acceptance criteria

1. **Limitations** explicitly listed vs competitors (honest).
2. **Screenshots** or sanitized examples.
3. **Review** by engineer who implemented Teams.
4. **Support** team acknowledges doc in handoff.
5. **Version** note: which AssistClaw version supports documented features.

## Definition of Done

- [ ] Linked from release announcement template.

## Dependencies

- STORY-025–029.

## Risks

- Overclaim; tie statements to tests in STORY-029.
