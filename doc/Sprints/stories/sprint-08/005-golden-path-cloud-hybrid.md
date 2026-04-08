# STORY-047 — Golden path documentation: cloud hybrid

| Field | Value |
|-------|--------|
| **Sprint** | sprint-08 |
| **Type** | Documentation |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

**Cloud hybrid**: gateway on VPS, models via API, object storage (optional), TLS termination, reverse proxy, secrets in cloud KMS (forward reference Sprint 13), backup/restore.

## User story

**As a** startup admin**  
**I want** a standard cloud deployment**  
**So that** I can scale out safely.

## Acceptance criteria

1. **Reference architecture** diagram (allowed mermaid in doc).
2. **TLS** setup with Let’s Encrypt or customer certs.
3. **Firewall** ports documented.
4. **Data residency** caveats (customer responsibility).
5. **Scaling** guidance: when to split components.

## Definition of Done

- [ ] Linked from enterprise pilot doc.

## Dependencies

- STORY-022.

## Risks

- Implied SLA; avoid unless contractual.
