# STORY-065 — Policy engine v1: tool allow/deny and channel restrictions

| Field | Value |
|-------|--------|
| **Sprint** | sprint-12 |
| **Type** | Enterprise / Security |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Introduce a **policy engine** evaluated before tool execution and inbound routing: allow/deny lists for tools, channel-specific rules, default-deny for high-risk tools in group contexts.

## User story

**As an** enterprise admin**  
**I want** centralized policies**  
**So that** agents cannot exfiltrate data or run shell in the wrong place.

## Scope

### In scope

- Policy schema in YAML (`policy.yaml` or embedded in `assistclaw.yaml`).
- Decision logging (audit) with rule ID.
- Hot reload or documented restart requirement.

### Out of scope

- Full OPA sidecar (optional future); keep embedded evaluator v1.

## Acceptance criteria

1. **Deny** decisions block tool calls with user/agent-visible reason code.
2. **Unit tests** for rule precedence: explicit deny overrides allow.
3. **Performance**: median evaluation &lt; 1ms per call at N rules (define N).
4. **Docs**: examples for “coding in DM allowed, bash denied in groups.”
5. **Migration**: safe defaults for existing installs.

## Definition of Done

- [ ] Threat model updated for policy bypass risks.

## Dependencies

- STORY-019 group policies.

## Risks

- Misconfiguration locks users out; break-glass admin path documented.
