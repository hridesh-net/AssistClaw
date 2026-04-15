# On-call: dashboards, alerts, and triage

Internal runbook for **STORY-012** — how to use observability during incidents. Version with AssistClaw when commands or ports change.

## Quick links

| Resource | Location |
|----------|----------|
| **Internal SLOs & error budgets** | [internal-slo-error-budgets.md](../observability/internal-slo-error-budgets.md) |
| **Metrics names, scrape, alerts** | [metrics-slo-indicators.md](metrics-slo-indicators.md) |
| **Grafana dashboard (golden signals)** | `doc/observability/grafana-sprint-02-channel-golden-signals.json` |
| **DLQ inspection** | [dlq-inspection.md](dlq-inspection.md) |
| **Correlation IDs / logs** | [structured-logging-correlation-ids.md](structured-logging-correlation-ids.md) |
| **Tracing (optional)** | [opentelemetry-tracing.md](opentelemetry-tracing.md) |
| **Load / failure drill** | [load-test-and-failure-injection.md](load-test-and-failure-injection.md) |

## Severity (internal)

| Level | Meaning | Notify |
|-------|---------|--------|
| SEV-1 | No outbound messages or gateway down for many users | Page on-call |
| SEV-2 | Elevated failures or DLQ growth; partial degradation | Page or ticket per policy |
| SEV-3 | Single channel flaky; workaround exists | Ticket, next business day |

## Symptom → checks → mitigation

### High DLQ rate / depth

1. Open Grafana DLQ / `dlq_depth` panel (see dashboard JSON).
2. Follow [dlq-inspection.md](dlq-inspection.md) — inspect `channels/dlq.ndjson` under `state_dir`.
3. Check `messages_failed_total` by `channel`; correlate with logs (correlation ID runbook).
4. Mitigate: fix upstream (token, rate limit), replay or discard poison messages per policy.

### Channel reconnect storm

1. Panel: `channel_reconnects_total` rate vs baseline.
2. Logs: disconnect reasons; verify network and provider status.
3. Mitigate: backoff config in `assistclaw.yaml` channels; restart gateway if wedged; escalate to channel owner.

### High latency (P95)

1. Histogram `message_latency_seconds` in dashboard.
2. Split by `channel`; check provider latency and queue depth.
3. Mitigate: scale workers, reduce batch size, or temporarily shed load (disable non-critical automations).

### Gateway crash / process gone

1. `gateway_health == 0` or scrape failures.
2. Restart: `assistclaw start` / systemd / launchd per install.
3. If disk full on `state_dir`: free space, rotate logs; see SLO doc for availability budget.

## Alert rules → action

| Alert (examples in metrics runbook) | Intended action |
|-------------------------------------|-----------------|
| `AssistClawHighFailureRate` (page) | Page on-call; begin DLQ + channel triage. |
| `AssistClawDLQGrowing` (warning) | Ticket + investigate within business hours unless paired with failures. |

## Ownership

| Area | Owner |
|------|--------|
| Channel adapters (Telegram, Slack, Discord, WA) | Channel integration owners |
| Gateway, `/metrics`, core agent | Core AssistClaw maintainers |

## Quarterly drill

Schedule a 60-minute calendar block to walk through one scenario (DLQ growth or reconnect storm) using this doc and staging. Record outcome in sprint notes or issue.

---

*Dependencies: STORY-007 (ops), STORY-008 (metrics), STORY-011 (SLOs).*
