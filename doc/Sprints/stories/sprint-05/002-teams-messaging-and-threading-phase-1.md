# STORY-026 — Microsoft Teams: messaging and threading (phase 1)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-05 |
| **Type** | Feature |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Implement **1:1 and channel** message receive and reply for Teams **phase 1** scope: text, basic markdown as supported by Teams, **threaded replies** in channel where Activity IDs support it.

## User story

**As a** Teams user**  
**I want** replies in the right thread**  
**So that** conversations stay readable.

## Scope

### In scope

- Map Teams Activity to internal `Message` model; reply via Bot Framework connector.
- Typing indicators if supported without excessive API calls.
- Rate-limit handling using STORY-002.

### Out of scope

- Adaptive Cards deep interactions (phase 2 epic).

## Acceptance criteria

1. **Receive** user messages in personal chat and in a team channel (config allows test channel).
2. **Reply** lands in same thread/conversation reference.
3. **Errors** surface to logs with correlation IDs; user sees friendly failure when send fails.
4. **Capability registry** updated for Teams (Sprint 1).
5. **Tests**: unit tests for activity mapping; integration test in sandbox.

## Definition of Done

- [ ] Demo recording or screenshot in docs (sanitized).

## Dependencies

- STORY-025.

## Risks

- Conversation reference bugs; add golden fixtures for activities.
