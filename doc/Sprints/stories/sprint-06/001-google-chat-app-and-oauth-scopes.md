# STORY-031 — Google Chat: app registration, OAuth/scopes, HTTP endpoint

| Field | Value |
|-------|--------|
| **Sprint** | sprint-06 |
| **Type** | Feature / Integration |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Implement **Google Chat** bot integration: Google Cloud project, Chat API enablement, service account or OAuth credentials as required, HTTP endpoint for Chat events, verify Google signatures.

## User story

**As a** Google Workspace customer**  
**I want** AssistClaw in Chat spaces**  
**So that** we can use one assistant across our stack.

## Scope

### In scope

- Secure webhook endpoint path (e.g. `/channels/googlechat/inbound`).
- Token verification and event parsing per Google Chat API.
- Config schema `channels.googlechat` with doctor checks.

### Out of scope

- Gmail integration (separate epic).

## Acceptance criteria

1. **Documentation** for Cloud Console: enable API, configure Chat app, install to workspace.
2. **Signature verification** implemented; rejects invalid requests with 401.
3. **Secrets** handling per STORY-021/022.
4. **Unit tests** with recorded JSON payloads (sanitized).
5. **Health**: doctor verifies credentials and endpoint reachability from Google’s perspective (document tunnel/ngrok for dev).

## Definition of Done

- [ ] Internal workspace E2E (STORY-033).

## Dependencies

- Sprint 1 adapter patterns.

## Risks

- Workspace admin approval delays; start test project early.
