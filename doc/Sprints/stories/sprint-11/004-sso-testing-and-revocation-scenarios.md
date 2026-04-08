# STORY-064 — Enterprise: SSO testing, token expiry, and user revocation

| Field | Value |
|-------|--------|
| **Sprint** | sprint-11 |
| **Type** | Test / Security |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Test **OIDC** edge cases: token refresh, expired sessions, revoked user (disabled in IdP), clock skew, group membership changes reflected on next login (or periodic refresh).

## User story

**As a** security engineer**  
**I want** predictable session behavior**  
**So that** offboarded users lose access quickly.

## Acceptance criteria

1. **Test cases** documented and automated where possible.
2. **Session max age** configurable.
3. **Immediate revoke** optional via short session TTL + refresh validation.
4. **Metrics**: auth failures, forbidden rate (no sensitive labels).
5. **Runbook** for “user still has access” incidents.

## Definition of Done

- [ ] Sign-off from security owner.

## Dependencies

- STORY-061–063.

## Risks

- Long-lived websockets; define re-auth strategy.
