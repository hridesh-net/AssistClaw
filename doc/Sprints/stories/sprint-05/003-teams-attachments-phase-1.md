# STORY-027 — Microsoft Teams: attachments (phase 1)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-05 |
| **Type** | Feature |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Support **incoming attachments** (images, files) for Teams phase 1: download via authenticated Graph or Bot API as required, size limits, temp file lifecycle, pass references to agent pipeline.

## User story

**As a** user**  
**I want** to send screenshots to the bot**  
**So that** the assistant can analyze them.

## Scope

### In scope

- Configurable max attachment size; reject gracefully.
- Virus scan hook point documented (customer responsibility) or optional ClamAV integration stub (optional).

### Out of scope

- Outbound rich cards with embedded images (unless trivial).

## Acceptance criteria

1. **Inbound** image attachment results in agent-visible content or file path per existing pipeline conventions.
2. **Large files** rejected with user-visible message in Teams.
3. **Temp files** cleaned up; test for leak.
4. **Security**: no arbitrary file write paths; content-disposition validated.
5. **Tests**: mock attachment payloads.

## Definition of Done

- [ ] Doc: limits and privacy note for attachments.

## Dependencies

- STORY-026.

## Risks

- Graph permission scope creep; document minimal permissions.
