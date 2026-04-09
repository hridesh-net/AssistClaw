# Runbook: Inspect Outbound DLQ

When channel outbound sends repeatedly fail, AssistClaw writes records to the DLQ (dead-letter queue).

## Default location

- `~/.assistclaw/channels/dlq.ndjson`
- configurable via `channels.outbound.dlq_path` in `assistclaw.yaml`

## Quick inspection

Use the built-in command:

```bash
assistclaw dlq list --limit 50
```

Each record includes:

- timestamp (`failed_at`)
- channel name
- session id
- idempotency key
- attempts count
- failure reason

## Manual inspection

DLQ is NDJSON (one JSON object per line). Example:

```bash
rg "telegram" ~/.assistclaw/channels/dlq.ndjson
```

## Replay guidance (manual MVP)

1. Inspect failed `session_id` + `text`.
2. Resolve root cause (token revoked, network, rate limit, etc.).
3. Re-send manually using the appropriate channel/session.
4. Keep old DLQ lines as incident evidence; rotate file periodically.

## Retention

For now retention is operational (manual rotation). Suggested practice:

- rotate weekly
- archive incident windows before truncation
