# Load Test Baseline (Initial)

Story: `sprint-02/004-load-test-and-failure-injection`

Date: 2026-04-14

Environment:

- Local developer machine baseline (initial reference only)
- Harness command: `make load-test`

## Initial baseline numbers

These initial values are placeholders for first-run comparison. Replace with measured values from CI/local outputs on your next run.

| Scenario | Total | Success | Failed | p50 | p95 | p99 |
|---|---:|---:|---:|---:|---:|---:|
| Steady | 120 | TBD | TBD | TBD | TBD | TBD |
| Burst | 240 | TBD | TBD | TBD | TBD | TBD |

## Notes

- Smoke CI uses reduced profile (`TestLoadHarness_Smoke`) to avoid flaky timing.
- Track trend deltas over time; absolute thresholds can be tightened after 3-5 baseline runs.
