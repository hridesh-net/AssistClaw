# STORY-013 — `assistclaw doctor`: configuration validation

| Field | Value |
|-------|--------|
| **Sprint** | sprint-03 |
| **Type** | Feature / UX |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Extend **`assistclaw doctor`** to validate `assistclaw.yaml` (and related files): schema/version, required keys per enabled feature, type checks, and deprecated key warnings with migration hints.

## User story

**As a** user**  
**I want** immediate feedback on bad config**  
**So that** I don’t fail at runtime with cryptic errors.

## Scope

### In scope

- Parse YAML; validate against a single source of truth (struct tags, JSON Schema, or manual validators).
- Exit codes: `0` OK, `1` warnings only, `2` errors (config invalid).
- Human-readable output with file path and line/column if available.

### Out of scope

- Remote provider checks (STORY-014).

## Acceptance criteria

1. **Invalid config** fails doctor with clear message and fix suggestion.
2. **Deprecated keys** emit warnings with replacement field names.
3. **Tests**: golden files for valid/invalid configs in `testdata/`.
4. **CI** runs doctor against sample configs.
5. **Doc** lists all checks and exit code semantics.

## Implementation status

- [x] `config.LoadForDoctor` + YAML walk for deprecated keys (`internal/config/doctor_load.go`).
- [x] `assistclaw doctor` exit codes: `0` OK, `1` warnings only, `2` errors (config invalid or check failed).
- [x] Fixtures: `internal/config/testdata/doctor/*.yaml` (valid, invalid port, deprecated `routing.primary`, missing `version`).
- [x] CI: `doctor --config .assistclaw.yaml.example --skip-network` in `.github/workflows/ci.yml`.
- [x] Doc: `doc/runbooks/doctor-config-validation.md` + index entry in `doc/runbooks/README.md`.
- [x] Onboarding copy points to `assistclaw doctor` and the runbook (STORY-016 placeholder satisfied in-repo).

## Definition of Done

- [x] Linked from onboarding flow (verify step + channel tips reference `doc/runbooks/doctor-config-validation.md`).

## Dependencies

- None.

## Risks

- False positives; allow `--ignore` flags only if absolutely needed (document anti-pattern).
