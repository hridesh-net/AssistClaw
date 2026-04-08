# STORY-072 — Secrets: HashiCorp Vault and cloud KMS integrations

| Field | Value |
|-------|--------|
| **Sprint** | sprint-13 |
| **Type** | Enterprise / Security |
| **Priority** | P1 |
| **Estimate** | L |

## Summary

Implement optional **Vault** integration (KV v2 or dynamic secrets) and **cloud KMS** envelope encryption for state at rest where applicable. Align with STORY-022 roadmap.

## User story

**As an** enterprise customer**  
**I want** enterprise secret management**  
**So that** keys are not on disk plaintext.

## Acceptance criteria

1. **Config** schema for Vault/KMS with clear precedence over env vars.
2. **Failure modes**: Vault down behavior (fail closed vs cached—document).
3. **Tests**: integration tests with Vault dev server in CI (docker) or mocks.
4. **Docs**: deployment recipes for AWS/GCP/Azure.
5. **Threat model** update for new trust boundaries.

## Definition of Done

- [ ] Security review before GA.

## Dependencies

- STORY-021 redaction (still required).

## Risks

- Operational complexity; mark as enterprise-only feature flag.
