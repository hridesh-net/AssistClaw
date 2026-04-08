# STORY-054 — macOS companion: beta distribution and crash reporting

| Field | Value |
|-------|--------|
| **Sprint** | sprint-09 |
| **Type** | Release / Ops |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Set up **beta channel**: signed builds, notarized (if applicable), update feed or manual releases, **opt-in crash reporting**, privacy policy for diagnostics.

## User story

**As a** release manager**  
**I want** controlled betas**  
**So that** we learn fast without burning users.

## Scope

### In scope

- CI build for macOS artifact; checksums.
- Crash reporting opt-in default **off** or clearly prompted.

### Out of scope

- Auto-update sparkle channel (optional).

## Acceptance criteria

1. **Install** doc for beta testers.
2. **Privacy** statement for crash data (what is sent).
3. **Version** string visible in UI.
4. **Rollback** path to previous build documented.
5. **Feedback** channel (Discord/GitHub) linked.

## Definition of Done

- [ ] Beta cohort ≥ N internal users (define N).

## Dependencies

- STORY-050–053.

## Risks

- Notarization delays; document gatekeeper bypass only for dev.
