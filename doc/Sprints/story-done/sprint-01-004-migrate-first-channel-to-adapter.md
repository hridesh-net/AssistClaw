# Sprint-01 Story-004: Migrate first channel to adapter (Telegram)

## Story reference

- Source: `doc/Sprints/stories/sprint-01/004-migrate-first-channel-to-adapter.md`
- Goal: migrate Telegram as the first production channel behind adapter contract + shared reliability.

## What was implemented

- Telegram channel implements adapter interfaces and bridges to legacy handler path:
  - `StartInbound` emits normalized `adapter.InboundEvent`
  - `Start` uses adapter->legacy bridge for runner compatibility
- Telegram outbound send path already used adapter `Send`; migration completed by routing legacy reply sends through adapter outbound calls.
- Wired shared reliability wrapper (`adapter.ReliableSender`) into Telegram runtime so outbound reply sends use:
  - retry/backoff
  - circuit breaker
  - DLQ persistence
- Added Telegram-focused tests for:
  - session parsing
  - max-length split behavior

## Key files

- `internal/channels/telegram/telegram.go`
- `internal/channels/telegram/telegram_test.go`
- `cmd/assistclaw/main.go`
- `internal/channels/adapter/reliability.go`
- `doc/channels/telegram-setup.md`

## Acceptance criteria mapping

1. Functional parity (send/receive/reply + behavior expectations): **done**
2. Automated tests (Telegram + retry path by mocks in adapter): **done**
3. No regressions (full test suite passes): **done**
4. Config migration note/backward compatibility: **done** (no breaking key rename; additive options only)
5. Rollback plan documented: **done** (see below)

## Manual checklist executed

- Telegram DM receive -> assistant reply
- Group mention-gated reply behavior
- Reply-to original inbound message behavior
- Long message splitting within Telegram limits
- Error classification via adapter send path

## Rollback plan

If issues are detected in release:

1. Revert the Telegram migration commit(s) that wire legacy replies through reliability wrapper.
2. Keep adapter contract and reliability core intact; revert only Telegram runtime wiring.
3. Re-tag patch release with rollback notes and announce temporary fallback to direct Telegram send path.

## Follow-ups

- Add attachment parity tests once Telegram attachment ingestion is expanded.
- Add feature flag for staged rollout if future channel migrations need gradual adoption.
