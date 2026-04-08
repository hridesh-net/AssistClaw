# STORY-032 — Google Chat: message mapping, spaces, and threading

| Field | Value |
|-------|--------|
| **Sprint** | sprint-06 |
| **Type** | Feature |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Map Google Chat events to internal messages: **DM vs space**, thread keys, user IDs, slash commands if applicable. Send replies to correct space/thread.

## User story

**As a** Chat user**  
**I want** coherent threads**  
**So that** the bot doesn’t spam the main space.

## Scope

### In scope

- Text messages; basic formatting per Chat API.
- Thread behavior documented for spaces that support threads.
- Mention handling / bot name triggers.

### Out of scope

- Card builder UI (stretch).

## Acceptance criteria

1. **DM** and **space** messages both work in test workspace.
2. **Thread** reply uses correct `threadKey` / API fields.
3. **Capability registry** entry for Google Chat complete.
4. **Tests**: unit tests for mapping functions; golden JSON fixtures.
5. **Logs** include space ID hashed or truncated if PII-sensitive.

## Definition of Done

- [ ] Parity matrix row filled (STORY-034).

## Dependencies

- STORY-031.

## Risks

- API differences between Chat editions; document supported SKU.
