# Sprint-01 Story-001: Channel adapter interface

## Story reference

- Source: `doc/Sprints/stories/sprint-01/001-channel-adapter-interface.md`
- Goal: establish a versioned channel adapter contract with shared message/error types.

## What was implemented

- Introduced adapter v1 interface set in `internal/channels/adapter`:
  - `Identity`
  - `InboundLifecycle`
  - `OutboundSender`
  - `Health`
  - composed `Adapter` interface
- Added shared transport/domain types:
  - `InboundEvent`, `OutboundMessage`, `DeliveryReceipt`
  - `SenderRef`, `RecipientRef`, `ThreadRef`, `Attachment`
  - `ChannelCapabilities`
- Added structured error taxonomy and helpers:
  - `ChannelError`
  - `ErrorKindRetryable`, `ErrorKindPermanent`, `ErrorKindRateLimited`
  - `KindOf`, `IsRetryable`, `IsPermanent`, `IsRateLimited`
- Added `adapter.Stub` implementation for tests and scaffolding.
- Migrated channel implementations to satisfy adapter contract while keeping legacy bridge:
  - Telegram
  - Discord
  - Slack
  - (WhatsApp retains legacy behavior and can be incrementally expanded)

## Key files

- `internal/channels/adapter/adapter.go`
- `internal/channels/adapter/types.go`
- `internal/channels/adapter/errors.go`
- `internal/channels/adapter/stub.go`
- `internal/channels/adapter/adapter_test.go`
- `internal/channels/inbound.go` (bridge helpers/metadata)
- channel packages under `internal/channels/*`

## Acceptance criteria mapping

1. Interface published with godoc: **done**
2. Shared types for normalized inbound/outbound + metadata: **done**
3. Error mapping without string matching: **done**
4. Unit tests with compile-time contract satisfaction via stub: **done**
5. Architecture/design doc for migration path: **done** (see adapter architecture notes in docs)

## Tests and validation

- Adapter tests validate:
  - compile-time interface satisfaction
  - error kind wrapping/classification
  - normalized payload shape fields

## Follow-ups

- Complete full migration for every legacy path in later stories.
- Expand parity matrix and capability registry usage (story 003+).
