# ADR: Channel Adapter v1

## Status

Accepted — 2026-04-08

## Context

AssistClaw historically integrated chat providers through [`internal/channels.Channel`](../../internal/channels/channel.go): `Name`, `Start` with a legacy [`MessageHandler`](../../internal/channels/channel.go), and `Stop`. That model mixed inbound handling with ad hoc reply callbacks and did not standardize outbound sends, health checks, capabilities, or error classification for retries.

We need a **versioned contract** so new connectors (Microsoft Teams, Google Chat, etc.) and reliability features (retries, DLQ, metrics) share one shape without breaking existing users during migration.

## Decision

1. Introduce package [`internal/channels/adapter`](../../internal/channels/adapter/) defining **Adapter v1**:
   - Composed interfaces: `Identity` (name, `AdapterVersion`, `Capabilities`), `InboundLifecycle`, `OutboundSender`, `Health`.
   - Normalized types: [`InboundEvent`](../../internal/channels/adapter/types.go), [`OutboundMessage`](../../internal/channels/adapter/types.go), [`ThreadRef`](../../internal/channels/adapter/types.go), [`Attachment`](../../internal/channels/adapter/types.go), [`ChannelCapabilities`](../../internal/channels/adapter/types.go), [`DeliveryReceipt`](../../internal/channels/adapter/types.go).
   - Errors: [`ChannelError`](../../internal/channels/adapter/errors.go) + [`ErrorKind`](../../internal/channels/adapter/errors.go) (`Retryable`, `Permanent`, `RateLimited`); classify with [`KindOf`](../../internal/channels/adapter/errors.go) / `errors.Is` — **not** string matching.

2. **Legacy `channels.Channel` remains** until each integration is migrated. No change to runtime wiring in the first PR; a [`Stub`](../../internal/channels/adapter/stub.go) adapter proves the interface and supports tests.

3. **Versioning**
   - `Identity.AdapterVersion()` returns `adapter.Version1` (const = 1) for this contract.
   - Inbound lifecycle is named **`StartInbound`** (not `Start`) so the same concrete type can also implement legacy [`channels.Channel`], which uses `Start(ctx, MessageHandler)`.
   - Breaking changes (method signature or semantic change) require **Adapter v2**: new types in `adapter` (e.g. `AdapterV2` interface or new package) and a migration note; bump `AdapterVersion` or introduce parallel registration.

4. **Mapping**
   - Inbound: provider events → [`InboundEvent`](../../internal/channels/adapter/types.go) → (during migration) optional bridge to legacy [`channels.Message`](../../internal/channels/channel.go) for the existing runner.
   - Outbound: runner → [`OutboundMessage`](../../internal/channels/adapter/types.go) → provider APIs.

## Consequences

- **Positive**: Single place for capabilities, outbound idempotency keys, and classified errors for STORY-002+.
- **Negative**: Temporary duplication (legacy `Channel` + `adapter.Adapter`) until migrations land.
- **Follow-up**: STORY-003 capability registry; STORY-004 migrate Telegram (or first channel) behind adapter + bridge.

## References

- Sprint story: [STORY-001 — Channel adapter interface](../Sprints/stories/sprint-01/001-channel-adapter-interface.md)
- Contributor checklist: [STORY-006](../Sprints/stories/sprint-01/006-adapter-checklist-and-contributor-docs.md)
