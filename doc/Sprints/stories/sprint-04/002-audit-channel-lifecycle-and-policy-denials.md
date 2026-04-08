# STORY-020 — Audit log: channel connect/disconnect and policy denials

| Field | Value |
|-------|--------|
| **Sprint** | sprint-04 |
| **Type** | Security / Compliance |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Extend the **audit log** to record: channel connect/disconnect, pairing approve/reject, and **policy denials** (guardrail blocked tool, blocked inbound, allowlist reject). Events must integrate with existing HMAC chain verification.

## User story

**As a** security auditor**  
**I want** tamper-evident logs of access and denials**  
**So that** we can investigate incidents and meet enterprise requirements.

## Scope

### In scope

- New event types with stable JSON schema.
- `assistclaw security verify` still works; extended tests.
- No sensitive payloads in full (truncate/redact).

### Out of scope

- SIEM export (Sprint 13).

## Acceptance criteria

1. **Events** emitted for listed actions; documented in `doc/security` or equivalent.
2. **Verification** tests include new event types in chain.
3. **Performance**: audit write does not block hot path (&gt;X ms budget defined in PR).
4. **Redaction** rules applied consistently (tokens, phone numbers).
5. **Migration** note for log format if versioned.

## Definition of Done

- [ ] Sample audit excerpt in docs (synthetic data).

## Dependencies

- Existing audit subsystem.

## Risks

- Log size; rotation policy referenced.
