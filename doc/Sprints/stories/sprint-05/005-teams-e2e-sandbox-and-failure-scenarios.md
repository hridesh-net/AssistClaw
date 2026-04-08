# STORY-029 — Microsoft Teams: sandbox E2E, credential rotation, throttling

| Field | Value |
|-------|--------|
| **Sprint** | sprint-05 |
| **Type** | Test / Reliability |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Run **end-to-end tests** in a sandbox Teams tenant: send/receive, attachment, thread reply. Test **credential rotation** (secret rollover) and **429/503** handling with retries.

## User story

**As a** QA engineer**  
**I want** repeatable Teams tests**  
**So that** releases don’t break enterprise messaging.

## Scope

### In scope

- Manual test script + optional automated nightly if service principal allows.
- Metrics validation: success rate, latency (Sprint 2).

### Out of scope

- Load test at Teams API limits (stretch).

## Acceptance criteria

1. **E2E checklist** signed for release with evidence (logs/screenshots).
2. **Secret rotation** procedure tested: update config, restart, no message loss for new traffic (define expectations).
3. **Throttling** test: simulated 429 from mock or recorded replay.
4. **SLO**: meets pilot targets from STORY-011 (document actuals vs target).
5. **Incident**: if E2E fails, blocker for release.

## Definition of Done

- [ ] Results attached to sprint review.

## Dependencies

- STORY-025–028.

## Risks

- Flaky external API; isolate network failures vs product bugs.
