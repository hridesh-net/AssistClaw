# STORY-035 — Cross-channel parity matrix (threading, files, reactions)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-06 |
| **Type** | Documentation |
| **Priority** | P1 |
| **Estimate** | S |

## Summary

Maintain a **parity matrix** across channels: threading, attachments, reactions, edits, threads, voice, DMs, slash commands. Update for Teams + Google Chat.

## User story

**As a** customer evaluating AssistClaw**  
**I want** a clear comparison table**  
**So that** I know what to expect per platform.

## Scope

### In scope

- Single Markdown table in `doc/`; generated optional.
- Legend for “supported”, “partial”, “planned.”

### Out of scope

- Competitive comparison to OpenClaw in marketing tone (keep factual).

## Acceptance criteria

1. **Every shipped channel** has a row including Teams and Google Chat.
2. **Partial** features have footnotes and issue links.
3. **Owner** assigned for matrix maintenance in CONTRIBUTING.
4. **Review** each release before tag.
5. **Link** from README.

## Definition of Done

- [ ] Linked from STORY-030 and Chat-specific docs.

## Dependencies

- STORY-003, STORY-026, STORY-032.

## Risks

- Stale matrix; tie to contract tests where possible.
