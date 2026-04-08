# STORY-028 — Microsoft Teams: config schema, onboarding, and install UX

| Field | Value |
|-------|--------|
| **Sprint** | sprint-05 |
| **Type** | UX / Docs |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Add **Teams** to `assistclaw onboard` (prompts or detection), validate keys in doctor, and ship **reference YAML** snippets.

## User story

**As a** new operator**  
**I want** guided setup**  
**So that** I don’t misconfigure Azure and Teams.

## Scope

### In scope

- Onboarding steps: collect app ID, tenant, secret, messaging URL alignment.
- Example `assistclaw.yaml` fragment.
- Troubleshooting: Teams app not showing bot, consent issues.

### Out of scope

- Multi-tenant SaaS bot marketplace listing.

## Acceptance criteria

1. **Onboard** path completes with validation errors surfaced (STORY-013).
2. **Doctor** checks Teams connectivity (STORY-015 pattern).
3. **Doc** &lt; 30 minutes for admin with Azure Portal access (validated by internal dry run).
4. **CHANGELOG** entry and upgrade notes.
5. **Support macros**: copy-paste replies for top 3 errors.

## Definition of Done

- [ ] Recorded TTFM for Teams setup in internal spreadsheet.

## Dependencies

- STORY-025–027.

## Risks

- Azure UI changes; keep docs versioned with date.
