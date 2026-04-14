# Sprint-01 Story-006: Adapter checklist and contributor documentation

## Story reference

- Source: `doc/Sprints/stories/sprint-01/006-adapter-checklist-and-contributor-docs.md`
- Goal: provide a production-ready contributor checklist for implementing and validating new channel adapters.

## What was implemented

- Added `New Channel Adapter Checklist` in `CONTRIBUTING.md` with direct mappings to STORIES 001-005 artifacts and tests.
- Added cross-links from checklist to:
  - capability matrix (`doc/channels/capability-registry.md`)
  - DLQ runbook (`doc/runbooks/dlq-inspection.md`)
- Added PR template checklist and validation section:
  - `.github/PULL_REQUEST_TEMPLATE.md`
- Linked checklist and related docs from README documentation table.

## Key files

- `CONTRIBUTING.md`
- `README.md`
- `.github/PULL_REQUEST_TEMPLATE.md`
- `doc/channels/capability-registry.md`
- `doc/runbooks/dlq-inspection.md`

## Acceptance criteria mapping

1. Checklist exists and is linked from README or CONTRIBUTING: **done**
2. Each checklist item maps to a concrete artifact: **done**
3. Review by at least one maintainer: **manual follow-up** (PR process)
4. PR DoD references checklist in PR template: **done**

## Tests and validation

- Documentation links verified against repo paths.
- Related adapter/channel tests and CI workflows validated in sprint closeout checks.

## Follow-ups

- Mark `CI` and `Adapter Contract Tests` as required checks in branch protection.
- Keep checklist updated when adapter interface or reliability semantics evolve.
