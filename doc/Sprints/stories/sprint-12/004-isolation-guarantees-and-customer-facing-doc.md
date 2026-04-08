# STORY-068 — Isolation guarantees: customer-facing documentation

| Field | Value |
|-------|--------|
| **Sprint** | sprint-12 |
| **Type** | Documentation / Compliance |
| **Priority** | P0 |
| **Estimate** | S |

## Summary

Publish **what we guarantee** vs **what customers must configure**: isolation model, tenant IDs, backup boundaries, subprocess boundaries, and known limitations.

## User story

**As a** enterprise buyer**  
**I want** clear guarantees**  
**So that** legal and security can approve.

## Acceptance criteria

1. **Document** in `doc/enterprise/` with version and date.
2. **Explicit** non-goals (e.g., OS-level isolation vs containers).
3. **Diagram** of tenant data flow.
4. **Alignment** with STORY-066 tests (reference by version).
5. **Review** by security + PM.

## Definition of Done

- [ ] Sales engineering deck links to this doc.

## Dependencies

- STORY-065–067.

## Risks

- Overclaim; tie statements to tests.
