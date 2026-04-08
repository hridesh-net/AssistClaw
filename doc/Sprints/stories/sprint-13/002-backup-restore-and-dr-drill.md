# STORY-070 — Backup, restore, and disaster recovery drill

| Field | Value |
|-------|--------|
| **Sprint** | sprint-13 |
| **Type** | Enterprise / Ops |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Define **backup scope**: SQLite/DB files, config, audit logs, DLQ, uploaded media. Automate restore procedure and run a **quarterly DR drill** with evidence.

## User story

**As an** operator**  
**I want** confidence in recovery**  
**So that** incidents don’t cause permanent data loss.

## Acceptance criteria

1. **Runbook** with RPO/RTO targets (internal first).
2. **Scripted** backup example (cron) with encryption guidance.
3. **Restore** test checklist executed and logged.
4. **DR drill** report template filled once.
5. **Gap list** if continuity of in-flight messages not guaranteed (honest).

## Definition of Done

- [ ] Stored artifact: drill date + participants.

## Dependencies

- STORY-002 DLQ semantics.

## Risks

- Encrypted backups key management (link KMS stories).
