# STORY-073 — Security pipeline: quarterly pen-test and dependency audits

| Field | Value |
|-------|--------|
| **Sprint** | sprint-13 |
| **Type** | Security / Process |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Establish **continuous security**: SCA/dependency scanning on PR, container scanning if applicable, **quarterly** pen-test cadence with external or internal red team, tracking issues to resolution SLAs.

## User story

**As a** security lead**  
**I want** repeatable assurance**  
**So that** enterprise customers trust us.

## Acceptance criteria

1. **CI** jobs documented; failures block release on critical CVEs (policy defined).
2. **Quarterly** pen-test scheduled; template report stored securely.
3. **SLA** for S0/S1 findings (define days).
4. **Evidence** package for SOC 2 controls mapping (initial).
5. **Process** owner assigned.

## Definition of Done

- [ ] First pen-test completed or scheduled with vendor.

## Dependencies

- Core features stabilized enough to test meaningfully.

## Risks

- Tooling noise; tune severities.
