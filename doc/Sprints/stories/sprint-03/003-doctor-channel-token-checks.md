# STORY-015 — `assistclaw doctor`: channel token checks

| Field | Value |
|-------|--------|
| **Sprint** | sprint-03 |
| **Type** | Feature |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

For each **enabled channel** (Telegram, Discord, Slack, WhatsApp), implement **non-destructive** checks: token format, bot identity fetch where API allows, webhook endpoint reachability (if applicable).

## User story

**As a** user**  
**I want** channel misconfiguration caught before go-live**  
**So that** I don’t miss messages silently.

## Scope

### In scope

- Channel-specific modules with shared reporting format.
- Rate-limit friendly: one call per channel max in doctor.
- Clear guidance for “missing scope” errors.

### Out of scope

- Sending test messages to real users (optional flag `--send-test-message` later).

## Acceptance criteria

1. **Each channel** has a documented check (or explicit “not supported in doctor yet”).
2. **Failures** include actionable next steps (e.g. “invite bot to workspace”).
3. **Tests** mock external APIs; no network in default CI.
4. **Secrets** never logged.
5. **Performance**: full doctor run completes in &lt; 60s with all channels enabled (configurable timeout).

## Definition of Done

- [ ] Table in doc: channel vs what is verified.

## Dependencies

- STORY-013.

## Risks

- WhatsApp session complexity; document limitations clearly.
