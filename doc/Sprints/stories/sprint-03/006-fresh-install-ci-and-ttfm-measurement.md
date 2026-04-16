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

1. [x] **CI job** runs on PR (and can be extended with `schedule`); `fresh-install` job in `.github/workflows/ci.yml` on Linux + macOS.
2. [x] **Golden files** updated via `UPDATE_SNAPSHOTS=1 go test …` or `.github/workflows/update-doctor-snapshot.yml` (`workflow_dispatch`).
3. [x] **TTFM** baseline table and hardware profile in `doc/runbooks/ttfm-measurement.md` (as of 2026-04-16).
4. [x] **Flake policy** documented in same runbook (retry once, then ticket).
5. [x] **README** links to TTFM runbook.

## Definition of Done

- [x] First successful CI run noted under **Unreleased** in `CHANGELOG.md` (fresh-install + snapshot + TTFM runbook).

## Implementation notes

- `scripts/ci-fresh-install.sh`, `install.sh` (`SKIP_NODE`, `ASSISTCLAW_INSTALL_GO_TAGS`), sorted doctor text, `cmd/assistclaw/doctor_text_snapshot_test.go` + `testdata/doctor_text_valid_minimal.snapshot.txt`.
- Windows: not in fresh-install matrix; documented in TTFM runbook.

## Dependencies

- STORY-013–017.

## Risks

- CI time; use caches and minimal images.
