# Sprint-01 Story-003: Channel capability registry

## Story reference

- Source: `doc/Sprints/stories/sprint-01/003-channel-capability-registry.md`
- Goal: establish a machine-readable capability registry so runtime behavior can safely degrade when a channel does not support optional features.

## What was implemented

- Added centralized registry API in adapter layer:
  - `CapabilitiesFor(channelType)`
  - `RegisterCapabilities(channelType, caps)`
- Extended capability schema to include:
  - `mentions`
  - `voice`
- Registered built-in channel capability entries for:
  - Telegram
  - Discord
  - Slack
  - WhatsApp
- Updated channel capability-return implementations to align with registry shape.
- Wired runtime outbound degradation:
  - if channel does not support threading, outbound path drops `ThreadRef`/`ReplyToID`
  - emits clear log line instead of failing unexpectedly
- Added contributor-facing doc and capability matrix table.

## Key files

- `internal/channels/adapter/capability_registry.go`
- `internal/channels/adapter/capability_registry_test.go`
- `internal/channels/adapter/types.go`
- `internal/channels/adapter/reliability.go`
- `internal/channels/telegram/telegram.go`
- `internal/channels/discord/discord.go`
- `internal/channels/slack/slack.go`
- `internal/channels/adapter/stub.go`
- `doc/channels/capability-registry.md`
- `CONTRIBUTING.md`

## Acceptance criteria mapping

1. Registry returns consistent capabilities for built-in channels: **done**
2. Runner/send path checks capabilities before optional features and degrades gracefully: **done**
3. Unit tests for registered channels: **done**
4. Doc table with capabilities linked for contributors: **done**
5. Extensibility mechanism documented (registration hook): **done**

## Tests and validation

- Added capability registry tests for:
  - built-in coverage (`telegram`, `discord`, `slack`, `whatsapp`)
  - extension registration via `RegisterCapabilities`
- Full project test suite passes after integration.

## Follow-ups

- Keep registry + docs in sync as channels evolve (consider generated docs in future).
- Expand matrix depth in sprint-06 parity work.
