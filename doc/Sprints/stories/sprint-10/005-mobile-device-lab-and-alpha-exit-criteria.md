# STORY-060 — Mobile companion (alpha): device lab testing and exit criteria

| Field | Value |
|-------|--------|
| **Sprint** | sprint-10 |
| **Type** | Test / Release |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Run **device matrix** tests (top iOS/Android versions), fix P0/P1 issues, define **alpha exit** criteria: crash rate, core path success, feedback themes.

## User story

**As a** release owner**  
**I want** clear alpha quality gates**  
**So that** we don’t pretend it’s production-ready.

## Acceptance criteria

1. **Matrix** doc: devices/OS versions tested.
2. **Crash rate** below agreed threshold (e.g. &lt; 1% sessions).
3. **Core path** test script passes on all matrix devices.
4. **Feedback** survey with ≥ 10 responses or internal equivalent.
5. **Go/no-go** decision recorded for beta (next phase).

## Definition of Done

- [ ] Known limitations doc shipped with alpha.

## Dependencies

- STORY-056–059.

## Risks

- Device lab cost; use cloud farm if needed.
