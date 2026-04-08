# STORY-043 — Golden path documentation: small team

| Field | Value |
|-------|--------|
| **Sprint** | sprint-08 |
| **Type** | Documentation |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Author an end-to-end **Small team** guide: one server, one gateway, one channel (e.g. Slack or Telegram), single LLM provider, backups of `~/.assistclaw`, basic monitoring.

## User story

**As a** small team admin**  
**I want** a copy-paste path**  
**So that** we go live in one sitting.

## Acceptance criteria

1. **Doc** covers prerequisites, install, onboard, start, verify with `doctor`, first message.
2. **Time estimate** and checklist at top.
3. **Troubleshooting** links to STORY-048 index.
4. **Tested** on fresh VM by someone other than author (sign-off line).
5. **Version** pinned: “tested with AssistClaw vX.Y.Z.”

## Definition of Done

- [ ] Linked from README “Quick start” variants.

## Dependencies

- Sprints 1–3 stability.

## Risks

- Doc drift; CI link check (STORY-047).
