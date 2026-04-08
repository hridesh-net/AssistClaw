# STORY-016 — `assistclaw doctor`: non-interactive and JSON output

| Field | Value |
|-------|--------|
| **Sprint** | sprint-03 |
| **Type** | Feature |
| **Priority** | P1 |
| **Estimate** | S |

## Summary

Add **`--no-input`** / **non-interactive** mode and optional **`--json`** output for machine consumption (CI, support scripts, remote diagnostics).

## User story

**As a** platform engineer**  
**I want** JSON output from doctor**  
**So that** I can integrate checks into pipelines and monitoring.

## Scope

### In scope

- Stable JSON schema version field (`schema_version`).
- Each check as an object: `id`, `severity`, `message`, `details` (optional).
- Non-interactive: no prompts; fail if input required.

### Out of scope

- Remote upload of JSON to support (optional later).

## Acceptance criteria

1. **`assistclaw doctor --json`** prints valid JSON to stdout; errors to stderr.
2. **Schema** documented with examples in `doc/`.
3. **CI** parses JSON and asserts required fields.
4. **Backward compatibility** policy: additive fields OK; breaking changes bump `schema_version`.
5. **Tests** cover golden JSON output for a fixed config.

## Definition of Done

- [ ] Example `jq` queries in doc for support team.

## Dependencies

- STORY-013–015.

## Risks

- Schema churn; version field mandatory from day one.
