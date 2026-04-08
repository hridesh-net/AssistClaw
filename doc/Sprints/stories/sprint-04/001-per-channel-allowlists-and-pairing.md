# STORY-019 — Per-channel allowlists and DM pairing flows

| Field | Value |
|-------|--------|
| **Sprint** | sprint-04 |
| **Type** | Security / Feature |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Implement **untrusted inbound** defaults aligned with enterprise expectations: unknown DMs receive a **pairing code** or are ignored until approved; per-channel **allowlists** (`allowFrom`, group policies) with consistent config schema across Telegram, Discord, Slack, WhatsApp.

## User story

**As an** admin**  
**I want** to control who can talk to the bot**  
**So that** we don’t process arbitrary internet traffic as trusted input.

## Scope

### In scope

- Config model: `dmPolicy` (pairing | open | deny), allowlist stores, approve/revoke commands or CLI (`assistclaw pairing approve` pattern).
- Persistence for approved peers (local DB or existing state store).
- User-facing messages for pairing (short, localized optional).

### Out of scope

- Full SSO mapping for DMs (Sprint 11).

## Acceptance criteria

1. **Default** for DMs is safe for each channel (document per-channel constraints).
2. **Admin** can approve pairing via documented command; state survives restart.
3. **Group chats**: mention-gating or allowlist documented and enforced.
4. **Tests**: table-driven tests for policy engine; simulated inbound from unknown ID.
5. **Migration** from old config keys if any; deprecation warnings.

## Definition of Done

- [ ] Security review checklist item signed (STORY-024).
- [ ] Doc: “hardening defaults” page.

## Dependencies

- Channel adapter work from Sprint 1 helps consistency.

## Risks

- UX friction; provide clear messages and admin docs.
