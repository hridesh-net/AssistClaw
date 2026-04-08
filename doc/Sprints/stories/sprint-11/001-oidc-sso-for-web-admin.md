# STORY-061 — Enterprise: OIDC SSO for web admin surfaces

| Field | Value |
|-------|--------|
| **Sprint** | sprint-11 |
| **Type** | Enterprise / Security |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Add **OIDC** login for gateway web UI and operator APIs: support Okta/Azure AD/Google Workspace OIDC; map `sub` and groups claims; session management with secure cookies or bearer tokens.

## User story

**As an** enterprise admin**  
**I want** SSO**  
**So that** we use corporate identity instead of shared tokens.

## Scope

### In scope

- Config: issuer, client ID, client secret (or confidential client), redirect URLs, scopes.
- Logout and session expiry.
- Migration path from static gateway token (dual mode during transition).

### Out of scope

- SAML directly (optional via IdP bridge).

## Acceptance criteria

1. **SSO** login works with at least two major providers in test (e.g. Okta + Azure AD).
2. **Group** claim mapping documented (optional RBAC input for STORY-062).
3. **Security**: CSRF, cookie flags, state parameter for OAuth; no tokens in URL on success path.
4. **Tests**: integration tests with mock OIDC server.
5. **Doc**: admin setup per provider with screenshots checklist.

## Definition of Done

- [ ] Pilot customer can log in with their IdP (staging).

## Dependencies

- Web UI from Sprint 7.

## Risks

- Redirect URL mismatch; strong validation errors.
