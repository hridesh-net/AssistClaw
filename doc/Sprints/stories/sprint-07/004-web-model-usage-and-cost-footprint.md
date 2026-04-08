# STORY-040 — Web UI: model usage and cost (where available)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-07 |
| **Type** | Feature / UI |
| **Priority** | P2 |
| **Estimate** | M |

## Summary

Show **model usage** per provider/model: tokens in/out, requests, estimated cost if pricing metadata available. Aggregates by day/week.

## User story

**As a** finance-conscious admin**  
**I want** to see usage**  
**So that** I can control spend.

## Scope

### In scope

- Read-only charts; data from existing usage tracking if present.
- Clear disclaimer when cost is estimated.

### Out of scope

- Billing integration (Stripe) — future.

## Acceptance criteria

1. **Dashboard** displays totals for selected time window.
2. **If cost unknown**, UI says “estimated” or hides cost column.
3. **Export** CSV optional (stretch).
4. **Tests**: unit tests for aggregation math.
5. **Docs**: how token counts are computed.

## Definition of Done

- [ ] Linked from enterprise pilot checklist when cost matters.

## Dependencies

- Metrics or DB tables for usage.

## Risks

- Wrong cost estimates; conservative labeling.
