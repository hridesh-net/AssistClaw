# STORY-022 — Secrets management strategy (env, file, future Vault/KMS)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-04 |
| **Type** | Documentation / Architecture |
| **Priority** | P1 |
| **Estimate** | S |

## Summary

Publish a **secrets strategy** document: recommended order (env vars, file permissions, OS keychain if any), rotation guidance, and roadmap to **Vault/KMS** (implemented in Sprint 13).

## User story

**As an** enterprise customer**  
**I want** clear secret handling**  
**So that** we can align with our security policy.

## Scope

### In scope

- Threat model summary for local state dir (`~/.assistclaw`).
- File permissions recommendations (Unix/Windows).
- Rotation playbooks for bot tokens and API keys.

### Out of scope

- Actual Vault integration (Sprint 13).

## Acceptance criteria

1. **Doc** in `doc/` with diagrams or tables as needed.
2. **Explicit** “what AssistClaw does not solve yet” (e.g. HSM).
3. **Alignment** with STORY-021 redaction.
4. **Review** by security champion.
5. **Link** from README security section.

## Definition of Done

- [ ] Referenced in enterprise sales one-pager (optional).

## Dependencies

- None.

## Risks

- Overpromising roadmap; label Vault/KMS as future.
