# STORY-059 — Mobile companion (alpha): attachments and policy limits

| Field | Value |
|-------|--------|
| **Sprint** | sprint-10 |
| **Type** | Feature / Security |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Support **image attachments** (phase 1) with size limits, content-type validation, and user-visible errors when policy denies upload.

## User story

**As a** user**  
**I want** to share screenshots**  
**So that** the assistant can see what I see.

## Scope

### In scope

- Client-side compression optional.
- Integration with gateway upload endpoint if required.

### Out of scope

- Arbitrary file types beyond agreed list.

## Acceptance criteria

1. **Max size** enforced client and server.
2. **Rejected** uploads show actionable error.
3. **Security**: no path traversal; virus scan policy referenced (customer).
4. **Tests**: unit tests for validators.
5. **Privacy**: explain where files are stored temporarily.

## Definition of Done

- [ ] Align with STORY-027 attachment philosophy.

## Dependencies

- Gateway media pipeline capabilities.

## Risks

- Large media costs; default limits conservative.
