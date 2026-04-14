# STORY-006 — Adapter checklist and contributor documentation

| Field | Value |
|-------|--------|
| **Sprint** | sprint-01 |
| **Type** | Documentation |
| **Priority** | P1 |
| **Estimate** | S |

## Summary

Publish a **Contributor checklist** for implementing a new channel adapter: interface implementation, capabilities registration, tests, security (allowlist), logging, and release notes.

## User story

**As a** external contributor**  
**I want** a clear checklist**  
**So that** my PR can be merged without rework.

## Scope

### In scope

- Markdown doc in `doc/` (or `CONTRIBUTING.md` section): prerequisites, file layout, testing steps, sample config.
- Checklist items: STORY-001–005, capability registry, security (forward reference to Sprint 4).
- Link to contract tests command.

### Out of scope

- Full video walkthrough (optional later).

## Acceptance criteria

1. **Checklist** exists and is linked from `README.md` or `CONTRIBUTING.md`.
2. **Each item** maps to a concrete artifact (file, test, doc section).
3. **Review** by at least one maintainer.
4. **“Definition of done”** for a channel PR explicitly references the checklist in PR template (if repo uses PR template).

## Definition of Done

- [x] New contributor can follow doc and run tests locally without asking in chat.
- [x] Cross-link to STORY-003 capability table.

## Dependencies

- STORY-001–005 complete or in final review.

## Risks

- Doc rot; assign owner to update when interface changes.

---

## Links (STORY-001 handoff)

- **Adapter v1 code:** [`internal/channels/adapter`](../../../../internal/channels/adapter/)
- **ADR / versioning:** [`doc/architecture/channel-adapter-v1.md`](../../../architecture/channel-adapter-v1.md)

## Implementation status

- [x] Checklist added to `CONTRIBUTING.md` (`New Channel Adapter Checklist`) with artifact mapping for STORIES 001-005.
- [x] Checklist linked from `README.md`.
- [x] Capability matrix and DLQ runbook cross-links added to contributor docs.
- [x] PR template added at `.github/PULL_REQUEST_TEMPLATE.md` with checklist references.
- [ ] Maintainer review sign-off remains a manual repository process (outside code changes).
