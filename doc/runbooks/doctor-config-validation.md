# `assistclaw doctor` — configuration validation and checks

This document describes what `assistclaw doctor` validates (config, LLM providers, channels, local intel) and how exit codes work.

For **`--json`** output shape, optional `details` per check, `jq` examples, and **`schema_version`** compatibility policy, see [doctor-json-schema.md](./doctor-json-schema.md).

In **text** mode (default), lines are printed sorted by check **`id`** so output order is stable for support scripts and golden snapshots.

## Exit codes

| Code | Meaning |
|------|--------|
| **0** | No errors and no warnings (all checks `ok` or `skipped`). |
| **1** | At least one **warning**, no **errors** (e.g. deprecated YAML key, unset `version`, local_intel path hint). |
| **2** | **Error**: invalid YAML, failed semantic validation, an LLM **provider** health check failed, or a channel check reported `error`. |

Parse failures (invalid YAML) and unreadable config path also yield exit **2**.

## Checks (config file present)

When `assistclaw.yaml` exists at the default path or `--config` path:

1. **YAML parse** — must be valid YAML after environment expansion (`${VAR}` in values).
2. **Semantic validation** — same rules as `config.Load`: gateway port range, outbound retry settings, `memory.semantic_backend`, MemPalace consistency, mining bounds, tracing sample ratio, at least one LLM provider when `routing.default` is empty, etc.
3. **Deprecated keys** — known legacy keys in the YAML tree emit **warnings** with a replacement hint (see `deprecatedYAMLKey` in `internal/config/doctor_load.go`). Example: `routing.primary` → use `routing.default`.
4. **Schema hint** — if top-level `version` is omitted, doctor emits a **warning** recommending `version: 1`.

Line and column numbers are included in messages when the YAML parser provides them (e.g. deprecated key location).

## Checks (no config file)

If the config file is missing, doctor uses environment-only defaults (`LoadFromEnv`) and does **not** run the YAML-specific checks above. Provider and channel checks still apply unless `--skip-network` is set.

## LLM provider checks

Unless `--skip-network` is set, doctor registers providers the same way as the agent (`registerProviders` in `cmd/assistclaw/main.go`) and runs [`Registry.CheckAll`](../../internal/provider/registry.go) (each provider’s `HealthCheck`, typically `ListModels` or equivalent) with a **45s** overall timeout for the whole batch.

- **Check IDs** are `provider.<name>` (e.g. `provider.openai`, `provider.ollama`).
- **Secrets** are not printed: error messages are passed through `sanitizeDoctorMessage` to redact common token patterns (`sk-…`, `Bearer …`).
- **No billing** checks — only reachability and auth-style signals.
- **Corporate proxy**: set standard environment variables (`HTTPS_PROXY`, `NO_PROXY`, etc.) for the Go HTTP client; doctor does not read a separate proxy block from YAML.

## Channel checks

After provider checks, doctor runs messaging-channel checks (unless `--skip-network`). Use **`--channel-timeout`** (default **15s** per channel) to cap each Telegram/Discord/Slack API call. The overall doctor run uses a **120s** upper bound for the whole command.

### Channel vs what is verified

| Channel | Check ID | What doctor verifies | Not verified here |
|---------|----------|------------------------|-------------------|
| **Telegram** | `channel.telegram` | Token shape (`<id>:<secret>`); one **getMe** (bot identity). | Sending messages, group membership |
| **Discord** | `channel.discord` | Token shape (three dot-separated segments); one **GET /users/@me**. | Gateway intents at runtime beyond token; voice |
| **Slack** | `channel.slack` | `xoxb-` / `xapp-` prefixes; one **auth.test**. | Event subscriptions, channel IDs |
| **WhatsApp** | `channel.whatsapp` | Local `whatsapp.db` session file exists (if `channels.whatsapp` is set). | Pairing, QR, or cloud API tokens (see limitations in OK message) |
| **Incoming webhooks** | `webhooks.gateway` | **Skipped** documentation when `webhooks.enabled` + mappings: gateway must be running for public ingress; doctor does not probe URLs. | End-to-end HTTP reachability from the internet |

Errors from chat APIs are passed through **`sanitizeDoctorMessage`** and channel-specific **hints** (401/403/429, timeouts). Tokens are never echoed in output.

**Performance:** With all channels configured, expect at most **one** outbound API call per provider above plus LLM providers; keep total under **~60s** in normal conditions by tightening `--channel-timeout` if needed.

## Troubleshooting: API works in another tool but `doctor` fails

1. **Different config file** — `doctor` uses `--config` / default `~/.assistclaw/assistclaw.yaml`. Confirm with `assistclaw doctor --config /path/to/assistclaw.yaml`.
2. **Timeouts** — provider checks share a 45s budget; slow networks or huge model lists can fail; retry or use `--skip-network` to isolate config vs network.
3. **TLS / proxy** — errors mentioning certificate or proxy may need `HTTPS_PROXY` or system trust store updates; air-gapped hosts should use `--skip-network` and rely on config validation only.
4. **Rate limits** — rare on list-models; if you see HTTP 429, wait and retry (doctor does not retry).
5. **Compare** — `assistclaw providers health` uses the same registry and `CheckAll`; if both fail, the issue is credentials or network, not doctor-specific.

## Tests and CI

- Golden-style fixtures live under `internal/config/testdata/doctor/` (valid, invalid, deprecated, version warning).
- CI runs `assistclaw doctor --config .assistclaw.yaml.example --skip-network` so the **example config parses** without hitting live LLM or chat APIs (no secrets in CI).
- Unit tests cover `sanitizeDoctorMessage` / provider error hints in `cmd/assistclaw/doctor_sanitize_test.go`.
- Token format + channel error hints (`formatChannelPingError`) are covered in `cmd/assistclaw/doctor_channel_test.go` (no live network).

## See also

- [Metrics and SLO indicators](metrics-slo-indicators.md)
- [On-call: dashboards, alerts, triage](on-call-dashboards-alerts.md)
