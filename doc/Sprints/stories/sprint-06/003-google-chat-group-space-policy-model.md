# STORY-033 — Unified group/space policy model (Google Chat + cross-channel)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-06 |
| **Type** | Architecture / Security |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Document and implement a **unified policy model** for “group-like” contexts: Slack channels, Discord guilds, Teams channels, Google Chat spaces. Align allowlists, mention policies, and admin-only commands.

## User story

**As an** admin**  
**I want** one mental model for group policies**  
**So that** YAML stays consistent across channels.

## Scope

### In scope

- Config schema proposal + migration notes.
- Code: shared policy evaluator used by Chat implementation.
- Doc: examples for mutli-channel deployments.

### Out of scope

- Full RBAC UI (Sprint 11).

## Acceptance criteria

1. **Policy doc** describes concepts: `spaceAllowlist`, `requireMention`, `ownerCommands`.
2. **Google Chat** uses shared evaluator; tests cover allow/deny cases.
3. **No regression** for existing channels’ behavior (diff tests or manual checklist).
4. **Examples**: 3 YAML snippets for common setups.
5. **Review** with maintainer of Slack/Discord for consistency.

## Definition of Done

- [ ] Linked from STORY-019 pairing doc as “group extensions.”

## Dependencies

- STORY-019, STORY-032.

## Risks

- Breaking YAML; use versioned config or deprecation period.
