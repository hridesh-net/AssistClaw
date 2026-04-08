# STORY-023 — Security regression suite: prompt/tool injection via channels

| Field | Value |
|-------|--------|
| **Sprint** | sprint-04 |
| **Type** | Test / Security |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Add automated tests that send **malicious payloads** through inbound channel adapters (simulated): prompt injection strings, oversized messages, path traversal in filenames, command injection in metadata. Assert guardrail + policy behavior.

## User story

**As a** security engineer**  
**I want** regression tests for untrusted inbound**  
**So that** new channels don’t bypass protections.

## Scope

### In scope

- Test vectors library in `testdata/security/`.
- Integration with guardrail (if present) and allowlist.
- CI job running suite on each PR.

### Out of scope

- Full fuzzing of binary protocols (stretch).

## Acceptance criteria

1. **Minimum** 20 curated vectors across categories (documented).
2. **CI** fails on regression (tool executed when should be denied).
3. **Coverage** for each channel type’s parsing entry point where applicable.
4. **False positive** review process documented.
5. **Quarterly** vector refresh ticket template.

## Definition of Done

- [ ] Mapped to STORY-019 pairing/allowlist expectations.

## Dependencies

- STORY-019, STORY-020.

## Risks

- Brittle tests; prefer stable assertions on outcomes not exact strings.
