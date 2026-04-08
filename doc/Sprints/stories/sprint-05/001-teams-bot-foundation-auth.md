# STORY-025 — Microsoft Teams: bot foundation and authentication

| Field | Value |
|-------|--------|
| **Sprint** | sprint-05 |
| **Type** | Feature / Integration |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Stand up a **Microsoft Teams** integration using the **Bot Framework** / Azure Bot Service path: app registration, tenant ID, client secret or certificate auth, messaging endpoint, and secure webhook validation.

## User story

**As an** enterprise customer**  
**I want** AssistClaw on Teams**  
**So that** employees can use the assistant in our collaboration hub.

## Scope

### In scope

- Config schema under `channels.teams` (names illustrative).
- Token acquisition (Microsoft identity platform), refresh, and error handling.
- Health check for doctor (Sprint 3 patterns).

### Out of scope

- Full Graph calendar integration (future).

## Acceptance criteria

1. **Documentation** lists Azure portal steps: app registration, Bot Channels Registration, Teams channel enablement.
2. **Secrets** loaded via env or config with redaction (Sprint 4).
3. **Inbound** signature validation for Bot Framework activity POSTs.
4. **Unit tests** with mocked Microsoft token and webhook payloads.
5. **Failure modes** documented: wrong tenant, expired secret, invalid messaging endpoint.

## Definition of Done

- [ ] Sandbox Teams tenant E2E successful (STORY-028).
- [ ] Parity with adapter interface (Sprint 1).

## Dependencies

- STORY-001–002, STORY-015.

## Risks

- Azure policy variance; test in customer-like tenant early.
