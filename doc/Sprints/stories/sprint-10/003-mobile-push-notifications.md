# STORY-058 — Mobile companion (alpha): push notifications (best-effort)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-10 |
| **Type** | Feature / Client |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

**Push notifications** for inbound messages when app backgrounded. Requires gateway or relay to send pushes—document architecture options (APNs/FCM) and privacy.

## User story

**As a** mobile user**  
**I want** notifications**  
**So that** I don’t keep the app open.

## Scope

### In scope

- Minimal viable: FCM/APNs wiring + device token registration to gateway.
- User toggle per session/channel.

### Out of scope

- Full unified notification service for multi-tenant SaaS.

## Acceptance criteria

1. **Device token** registration secure; token revocable.
2. **Payload** minimal; no message body if “privacy strict” mode on.
3. **Tests**: sandbox push tests or mocked.
4. **Doc** limitations: iOS background, Android Doze.
5. **Cost** note if third-party push relay used.

## Definition of Done

- [ ] Explicit “best-effort” disclaimer in UI.

## Dependencies

- Gateway endpoints for device tokens.

## Risks

- Complexity; consider deferring to beta if schedule slips.
