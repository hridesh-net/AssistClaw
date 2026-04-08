# STORY-021 — Secrets redaction in logs and debug output

| Field | Value |
|-------|--------|
| **Sprint** | sprint-04 |
| **Type** | Security |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Ensure **no API keys, tokens, or webhook secrets** appear in INFO logs or default debug. Implement central redaction helpers and scan common patterns (Bearer, bot tokens, etc.).

## User story

**As a** security-conscious user**  
**I want** safe defaults for logging**  
**So that** pasted logs don’t leak credentials.

## Scope

### In scope

- Redaction middleware for structured logs.
- Audit of `fmt.Printf` / debug dumps in channel packages.
- Tests with synthetic secrets that must not appear in output.

### Out of scope

- Full DLP for message content (enterprise later).

## Acceptance criteria

1. **Test suite** fails if known secret strings appear in captured logs for covered paths.
2. **Documentation** lists what is redacted and limitations (e.g. custom token formats).
3. **Debug level** still avoids raw secrets; if unavoidable, behind explicit env `ASSISTCLAW_UNSAFE_LOG=1` with warnings.
4. **Code review** checklist item: “secrets safe.”
5. **Spot check** on three channels: Telegram, Discord, Slack.

## Definition of Done

- [ ] Linked from STORY-022 secrets strategy doc.

## Dependencies

- STORY-007 logging.

## Risks

- Over-redaction breaking debugging; provide structured “token_id” references.
