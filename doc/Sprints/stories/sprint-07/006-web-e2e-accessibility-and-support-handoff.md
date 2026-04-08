# STORY-042 — Web UI: E2E tests, accessibility, support handoff

| Field | Value |
|-------|--------|
| **Sprint** | sprint-07 |
| **Type** | Test / Quality |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Add **E2E** tests for web operator flows (Playwright/Cypress or equivalent). Run **accessibility** checks on core pages. **Support handoff**: short video or script for triaging “can’t receive messages” using STORY-037–039.

## User story

**As a** support lead**  
**I want** reliable UI tests and training**  
**So that** releases don’t break triage.

## Scope

### In scope

- CI job for E2E (headless).
- A11y: axe-core or similar on main views.
- Support doc: 1-page triage script.

### Out of scope

- Full visual regression per browser.

## Acceptance criteria

1. **E2E** covers: login/token, sessions list, channel health, errors panel.
2. **A11y** fails CI on critical violations (configurable).
3. **Support** team acknowledges training material.
4. **Flake rate** &lt; 2% over 2 weeks or quarantine policy.
5. **Release gate**: E2E green before shipping.

## Definition of Done

- [ ] STORY roadmap milestone “operator triage without SSH” marked done.

## Dependencies

- STORY-037–039.

## Risks

- Flaky E2E; use stable selectors and retries.
