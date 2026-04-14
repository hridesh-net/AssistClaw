# STORY-001 — Channel adapter interface (contract)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-01 |
| **Type** | Feature / Architecture |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Define a single, versioned **channel adapter** contract in Go that all messaging integrations implement. The contract must cover inbound/outbound messages, optional threading, media handles, and metadata needed for reliability features (idempotency, rate limits) in later stories.

## Background

Today channels may be implemented ad hoc. A stable interface reduces duplication, speeds new connectors (Teams, Google Chat, etc.), and enables shared testing and observability.

## User story

**As a** maintainer of AssistClaw channels  
**I want** a clear adapter interface and shared types  
**So that** new integrations are consistent, testable, and enterprise-ready.

## Scope

### In scope

- Go `interface` (or small set of interfaces) for: lifecycle (`Start`, `Stop`), outbound send, subscription to inbound events, health/ping.
- Shared types: `Message`, `ThreadRef`, `Attachment`, `ChannelCapabilities`, `DeliveryReceipt`, `InboundEvent` (names illustrative—align with codebase conventions).
- Versioning strategy: document how breaking changes are introduced (e.g. `AdapterV2`).
- Error taxonomy: `Retryable`, `Permanent`, `RateLimited` (or mapped errors) so upper layers can behave correctly.

### Out of scope

- Full migration of all channels (covered in STORY-004).
- Production metrics (Sprint 2).

## Technical notes

- Prefer small interfaces; avoid forcing unused methods on minimal channels.
- Consider context propagation and cancellation on `Stop`.
- Document thread/guild semantics per channel in capability registry (STORY-003).

## Acceptance criteria

1. **Interface published** in `internal/` (or agreed package) with godoc describing each method and when it is called.
2. **Types** cover at least: text body, optional media references, sender/recipient identifiers, channel-specific opaque IDs, and timestamps.
3. **Error mapping** documented: adapter returns errors that the runner can classify without string matching.
4. **Unit tests** exist for compile-time satisfaction of the interface by a **stub/mock** adapter used in tests.
5. **ADR or short design doc** (can live in `doc/`) explains versioning and migration path for existing channels.

## Definition of Done

- [ ] Code reviewed and merged. _(manual PR/reviewer process)_
- [x] No user-visible behavior change until STORY-004 lands (interface-only + mock).
- [x] Contributor-facing note links from STORY-006 checklist.

## Dependencies

- None (blocks STORY-002–006).

## Risks

- Overfitting the interface to one channel; mitigate with second channel review in Sprint 5–6 planning.

## Implementation status

- [x] Adapter v1 interfaces published in `internal/channels/adapter/adapter.go` with godoc.
- [x] Shared types and error taxonomy implemented in `internal/channels/adapter/types.go` and `internal/channels/adapter/errors.go`.
- [x] Stub/contract tests implemented in `internal/channels/adapter/stub.go` and `internal/channels/adapter/adapter_test.go`.
- [x] Versioning and migration notes documented in `doc/architecture/channel-adapter-v1.md`.
