# STORY-053 — macOS companion: secure device pairing

| Field | Value |
|-------|--------|
| **Sprint** | sprint-09 |
| **Type** | Security / Client |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

If architecture requires **device pairing**, implement short-lived pairing codes, mutual confirmation, and revocation from gateway. Align with enterprise expectations (no static passwords in plist).

## User story

**As a** security admin**  
**I want** explicit device pairing**  
**So that** random laptops can’t connect to the gateway.

## Scope

### In scope

- Pairing protocol design doc + implementation.
- Revocation list UI/API on gateway (minimal).

### Out of scope

- MDM deployment guide (stretch).

## Acceptance criteria

1. **Pairing** completes in under 2 minutes in happy path.
2. **Revoke** device blocks further connections immediately (≤ 30s).
3. **Tests**: protocol unit tests; misuse attempts fail.
4. **Doc** for firewall and LAN discovery if used.
5. **Audit** events for pair/unpair (STORY-020).

## Definition of Done

- [ ] Threat model review (short).

## Dependencies

- Gateway support for pairing endpoints.

## Risks

- mDNS/Bonjour complexity; document fallback.
