# STORY-003 — Channel capability registry

| Field | Value |
|-------|--------|
| **Sprint** | sprint-01 |
| **Type** | Feature |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Introduce a **capability registry** that describes what each channel supports: threads, reactions, edits, attachments, voice, DMs vs groups, max message length, etc. The runner and UI use this to avoid sending unsupported operations and to document behavior clearly.

## User story

**As a** developer**  
**I want** a single source of truth for channel capabilities  
**So that** the agent and tools do not assume unsupported features.

## Scope

### In scope

- Machine-readable registry (e.g. map keyed by channel type or struct per channel).
- Capabilities include at minimum: `threading`, `attachments`, `dm`, `group`, `mentions`, `maxMessageLength` (as applicable).
- Runtime API: `CapabilitiesFor(channelType)`.
- Documentation table generated or maintained in `doc/` for parity matrix (extended in Sprint 6).

### Out of scope

- Dynamic capability discovery from remote APIs (optional future).

## Acceptance criteria

1. **Registry** returns consistent capabilities for each built-in channel type.
2. **Runner or send path** checks capabilities before optional features (e.g. thread reply) and degrades gracefully with a clear log line.
3. **Unit tests** for each registered channel type.
4. **Doc** lists capabilities in a table (Markdown) linked from contributor guide.
5. **Extensibility**: documented how a new channel registers its capabilities (hook or registration function).

## Definition of Done

- [x] Linked from STORY-006 checklist.
- [ ] Reviewed by one channel owner. _(manual PR/reviewer process)_

## Dependencies

- STORY-001 (adapter interface) for alignment.

## Risks

- Drift between code and docs; add CI check or single source generation if feasible.

## Implementation status

- [x] Capability registry API implemented in `internal/channels/adapter/capability_registry.go`.
- [x] Built-in capability coverage tests in `internal/channels/adapter/capability_registry_test.go`.
- [x] Runtime graceful degradation for unsupported threading in `internal/channels/adapter/reliability.go`.
- [x] Contributor-facing matrix published in `doc/channels/capability-registry.md`.
- [x] Thread-safe registry access added for production safety.
