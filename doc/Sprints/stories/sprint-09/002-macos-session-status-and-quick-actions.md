# STORY-051 — macOS companion: session status and quick actions

| Field | Value |
|-------|--------|
| **Sprint** | sprint-09 |
| **Type** | Feature / Client |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Show **active sessions**, model in use, and quick actions: open web UI, copy session ID, pause notifications (local only).

## User story

**As a** power user**  
**I want** session awareness from the menu bar**  
**So that** I can context-switch quickly.

## Scope

### In scope

- Read-only session list from gateway API.
- Deep link to web session detail if available (Sprint 7).

### Out of scope

- Full chat transcript in macOS (heavy scope).

## Acceptance criteria

1. **Session list** matches gateway data within refresh interval (≤ 5s configurable).
2. **Errors** surfaced with human-readable text.
3. **Accessibility**: VoiceOver labels on critical controls.
4. **Tests**: UI tests for list load and empty state.
5. **Privacy**: no message content stored on disk without user action.

## Definition of Done

- [ ] Demo in sprint review.

## Dependencies

- STORY-050, STORY-037 APIs.

## Risks

- API churn; version gateway API.
