# STORY-057 — Mobile companion (alpha): chat list and send/receive

| Field | Value |
|-------|--------|
| **Sprint** | sprint-10 |
| **Type** | Feature / Client |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Implement **chat list** (sessions or channels) and **message thread** with send/receive for text; show typing/connection state.

## User story

**As a** mobile user**  
**I want** basic chat**  
**So that** I can interact with the assistant.

## Scope

### In scope

- Optimistic send with failure retry UI.
- Pagination/infinite scroll for history (limited window).

### Out of scope

- Full feature parity with web.

## Acceptance criteria

1. **Send** message reaches gateway and response appears in thread.
2. **Failure** shows retry; does not duplicate on success path (idempotency client-side).
3. **Accessibility**: large text support basics.
4. **Tests**: integration tests with mock gateway; UI tests for happy path.
5. **Performance**: scroll remains smooth with 1k messages (virtualized list).

## Definition of Done

- [ ] No P0 crashes on core path in alpha (STORY-060).

## Dependencies

- STORY-056.

## Risks

- WebSocket battery drain; document background limitations.
