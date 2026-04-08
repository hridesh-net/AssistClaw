# STORY-067 — Policy evaluation performance and load tests

| Field | Value |
|-------|--------|
| **Sprint** | sprint-12 |
| **Type** | Test / Performance |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Benchmark policy evaluation under load: high message rate, many rules, concurrent tool calls. Ensure no lock contention or O(n²) scans.

## User story

**As a** SRE**  
**I want** predictable overhead**  
**So that** policies don’t slow the agent.

## Acceptance criteria

1. **Benchmarks** in CI (thresholds) or nightly job.
2. **Profiling** results attached; hotspots fixed or documented.
3. **Worst-case** rule count documented for enterprise SKU.
4. **Load test** integrates policy with STORY-010 harness.
5. **Regression** guard: fail CI if &gt; X% overhead vs baseline.

## Definition of Done

- [ ] Graphs in doc or internal wiki.

## Dependencies

- STORY-065.

## Risks

- Benchmark flakiness; use fixed seeds and hardware.
