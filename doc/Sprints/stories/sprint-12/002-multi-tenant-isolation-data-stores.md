# STORY-066 — Multi-tenant isolation: data stores and config paths

| Field | Value |
|-------|--------|
| **Sprint** | sprint-12 |
| **Type** | Enterprise / Architecture |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Implement **tenant isolation** for enterprise deployments: separate state prefixes or databases per tenant, no cross-tenant reads/writes, consistent tenant ID propagation in logs/metrics.

## User story

**As a** SaaS operator**  
**I want** hard isolation**  
**So that** one customer cannot access another’s data.

## Scope

### In scope

- Tenant context on every DB query and file path join.
- Enforcement middleware for APIs (STORY-062 extended).

### Out of scope

- Full multi-region active-active (Sprint 13).

## Acceptance criteria

1. **Negative tests** prove cross-tenant access impossible (automated).
2. **Code audit** checklist for `WHERE tenant_id` coverage.
3. **Migrations** for single-tenant → multi-tenant if needed.
4. **Performance**: index strategy documented.
5. **Doc** deployment patterns: shared gateway vs per-tenant gateway.

## Definition of Done

- [ ] External review or internal red-team attempt documented.

## Dependencies

- STORY-062.

## Risks

- Legacy code paths; long-running migration.
