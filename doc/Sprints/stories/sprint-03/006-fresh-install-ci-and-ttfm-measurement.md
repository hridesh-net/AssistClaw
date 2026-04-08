# STORY-018 — Fresh install CI, golden snapshots, TTFM measurement

| Field | Value |
|-------|--------|
| **Sprint** | sprint-03 |
| **Type** | Test / Quality |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Add **scripted fresh install** tests (Linux + macOS) and **golden output** snapshots for `doctor`. Record **time-to-first-message (TTFM)** methodology and baseline for future improvement.

## User story

**As a** release manager**  
**I want** CI to catch install regressions**  
**So that** releases stay “it just works.”

## Scope

### In scope

- CI job: install script + `doctor` + minimal config (mock provider if needed).
- Snapshot tests for `doctor` text output (stable ordering).
- TTFM doc: steps timed from clone/install to first successful `agent --message` (or equivalent).

### Out of scope

- Windows path unless already in roadmap; document “not in CI” if skipped.

## Acceptance criteria

1. **CI job** runs on PR or nightly; failure blocks release branch if configured.
2. **Golden files** updated via intentional `UPDATE_SNAPSHOTS=1` workflow.
3. **TTFM** baseline documented with date and hardware profile.
4. **Flake policy**: retries with root-cause ticket if flaky.
5. **README** links to TTFM expectations.

## Definition of Done

- [ ] First successful CI run logged in CHANGELOG or internal release notes.

## Dependencies

- STORY-013–017.

## Risks

- CI time; use caches and minimal images.
