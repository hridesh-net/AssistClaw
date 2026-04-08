# STORY-056 — Mobile companion (alpha): authentication and device pairing

| Field | Value |
|-------|--------|
| **Sprint** | sprint-10 |
| **Type** | Feature / Client |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

**iOS/Android alpha** app: login via gateway token or OIDC (if ready), **device pairing** flow consistent with macOS (STORY-053), secure token storage (Keychain/Keystore).

## User story

**As a** mobile user**  
**I want** a safe connection to my gateway**  
**So that** I can chat on the go.

## Scope

### In scope

- One codebase (React Native/Flutter/native—pick and document).
- Biometric unlock optional for stored token.

### Out of scope

- App store production release (alpha only).

## Acceptance criteria

1. **Pairing** documented end-to-end with screenshots (sanitized).
2. **Token** never logged; screenshots blocked in production builds where possible.
3. **Revocation** tested (server-side).
4. **Tests**: unit tests for crypto/storage wrappers.
5. **Platform** requirements documented (iOS/Android min versions).

## Definition of Done

- [ ] Internal dogfood group ≥ 5 users.

## Dependencies

- Gateway pairing API.

## Risks

- App store policy for local gateway; clarify use case in doc.
