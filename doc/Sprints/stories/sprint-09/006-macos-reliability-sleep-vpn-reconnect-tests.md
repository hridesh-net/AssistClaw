# STORY-055 — macOS companion: sleep, wake, VPN, reconnect tests

| Field | Value |
|-------|--------|
| **Sprint** | sprint-09 |
| **Type** | Test / QA |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Manual **test matrix** for macOS: sleep/wake, VPN connect/disconnect, network change (Wi-Fi ↔ Ethernet), gateway restart. Automate what’s feasible (unit tests for reconnect logic).

## User story

**As a** user**  
**I want** the app to recover from network issues**  
**So that** I don’t restart it constantly.

## Acceptance criteria

1. **Test matrix** document with pass/fail for each scenario.
2. **Reconnect** succeeds within 30s after network restored (target).
3. **No duplicate** connections (leak test with connection count metric if available).
4. **Crash-free** sessions metric ≥ 99.5% over beta period (target).
5. **Known issues** list published with workarounds.

## Definition of Done

- [ ] Sign-off from QA owner.

## Dependencies

- STORY-050.

## Risks

- Flaky manual tests; record screen captures for evidence.
