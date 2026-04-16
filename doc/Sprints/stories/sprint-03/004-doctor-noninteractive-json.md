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
- Non-interactive: no prompts; `--no-input` / `--non-interactive` (reserved for future checks that might require stdin).

### Out of scope

- Remote upload of JSON to support (optional later).

## Acceptance criteria

1. [x] **`assistclaw doctor --json`** prints valid JSON to stdout; errors to stderr.
2. [x] **Schema** documented with examples in `doc/` (`doc/runbooks/doctor-json-schema.md`).
3. [x] **CI** parses JSON and asserts required fields (`.github/workflows/ci.yml`).
4. [x] **Backward compatibility** policy: additive fields OK; breaking changes bump `schema_version` (documented in runbook).
5. [x] **Tests** cover JSON output for a fixed config (`doctor_json_test.go` + `valid_minimal.yaml` fixture).

## Definition of Done

- [x] Example `jq` queries in doc for support team (`doc/runbooks/doctor-json-schema.md`).

## Implementation notes

- JSON types: `doctorOutput` / `doctorCheck` in `cmd/assistclaw/doctor_cmd.go`; optional `details` map; `config_path` and `exit_code` in JSON for machines.
- Runbook: `doc/runbooks/doctor-json-schema.md` (schema, compatibility, `jq`).
- CI: `.github/workflows/ci.yml` runs `doctor --json` and `jq` assertions.
- Tests: `cmd/assistclaw/doctor_json_test.go` (round-trip + subprocess with `valid_minimal.yaml`).

## Dependencies

- STORY-013–015.

## Risks

- Schema churn; version field mandatory from day one.
