# STORY-048 — Docs CI: link check, snippet validation, troubleshooting index

| Field | Value |
|-------|--------|
| **Sprint** | sprint-08 |
| **Type** | Tooling / CI |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Add CI jobs: **markdown link check** on `doc/`, optional **snippet execution** for fenced code blocks (where safe), and a **troubleshooting index** mapping `doctor` error codes to STORY-043–047 pages.

## User story

**As a** maintainer**  
**I want** CI to guard docs**  
**So that** releases don’t ship broken instructions.

## Acceptance criteria

1. **Link check** runs on PR; fails on broken internal links.
2. **Snippet** runner (if any) whitelisted paths only; no network calls by default.
3. **Troubleshooting index** lists error codes / substrings → doc anchors.
4. **Contributor** guide explains how to add new snippets safely.
5. **Weekly** report optional: snippet pass rate tracked in dashboard or issue.

## Definition of Done

- [ ] Zero broken links on release branch at time of sprint end.

## Dependencies

- STORY-013–016 doctor codes stable enough to index.

## Risks

- External link flakiness; exclude or allowlist.
