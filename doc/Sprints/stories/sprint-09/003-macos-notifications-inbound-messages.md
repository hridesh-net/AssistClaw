# STORY-052 — macOS companion: notifications for inbound messages

| Field | Value |
|-------|--------|
| **Sprint** | sprint-09 |
| **Type** | Feature / Client |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

**User notifications** when inbound messages arrive (configurable per channel/session). Respect macOS notification permissions and Do Not Disturb.

## User story

**As a** user**  
**I want** desktop notifications**  
**So that** I don’t miss urgent agent replies.

## Scope

### In scope

- Notification content: channel, sender label (redacted), snippet optional.
- Click action: opens web UI or app panel.

### Out of scope

- Rich media in notification center.

## Acceptance criteria

1. **Permission** flow handles denied state gracefully.
2. **No secrets** in notification text.
3. **Rate limit** notifications to avoid spam (debounce).
4. **Tests**: unit tests for debounce logic; manual checklist for permissions.
5. **Privacy** doc: what appears on lock screen.

## Definition of Done

- [ ] User setting to disable notifications per channel.

## Dependencies

- STORY-050.

## Risks

- Notification fatigue; strong defaults off or summaries only.
