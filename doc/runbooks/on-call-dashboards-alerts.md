# On-call: dashboards, alerts, and triage

Internal runbook for **STORY-012** — how to use observability during incidents. Re-read when upgrading AssistClaw; default **gateway** listen address is `127.0.0.1:18790` unless `gateway.port` / `gateway.host` differ in `assistclaw.yaml`.

| Field | Value |
|-------|--------|
| **Runbook version** | 1.1 |
| **Last updated** | 2026-04-14 |
| **AssistClaw compatibility** | v3.10.x+ (CLI and `/metrics` as shipped) |

Throughout this doc, **`$STATE_DIR`** means your configured state root (default `~/.assistclaw`). Override with `--config` / `ASSISTCLAW_STATE_DIR` as you do for normal CLI use.

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

## Common CLI commands (on-call)

Run from a shell where `assistclaw` is on `PATH`, or use the full path to the binary.

```bash
# Process / service (systemd, launchd, or foreground install — see install.sh)
assistclaw status
assistclaw stop
assistclaw start
assistclaw restart

# Gateway aliases (same underlying behavior as status/stop/start where applicable)
assistclaw gateway status
assistclaw gateway stop
assistclaw gateway restart

# DLQ (uses config to resolve path; add -c /path/to/assistclaw.yaml if not default)
assistclaw dlq list --limit 50
```

## Metrics and health checks (`curl`)

Default bind: **`127.0.0.1:18790`**. If `gateway.token` is set in `assistclaw.yaml`, **all** routes including `/metrics` require a Bearer token (see [metrics-slo-indicators.md](metrics-slo-indicators.md)).

**No gateway token:**

```bash
curl -fsS "http://127.0.0.1:18790/metrics" | head -n 40
curl -fsS "http://127.0.0.1:18790/metrics" | grep -E 'gateway_health|dlq_depth|messages_(sent|failed)_total'
```

**With gateway token** (replace `"$TOKEN"` from your secrets store, not from chat logs):

```bash
export TOKEN='…'   # from gateway.token
curl -fsS -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:18790/metrics" | grep gateway_health
```

## Symptom → likely cause → checks → mitigation

### 1. High DLQ rate / depth

| Step | Action |
|------|--------|
| Likely cause | Channel token revoked, rate limit, network partition, or poison payload. |
| 1 | Open Grafana: DLQ / `dlq_depth` and failure panels (`doc/observability/grafana-sprint-02-channel-golden-signals.json`). |
| 2 | CLI: `assistclaw dlq list --limit 50` |
| 3 | Raw tail (optional): `tail -n 100 "$STATE_DIR/channels/dlq.ndjson"` or path from `channels.outbound.dlq_path`. |
| 4 | Logs: use [structured-logging-correlation-ids.md](structured-logging-correlation-ids.md) to tie failures to a single request/session. |
| 5 | Mitigate: fix upstream (rotate token, backoff), then re-send or replay per [dlq-inspection.md](dlq-inspection.md) replay guidance. |

### 2. Channel reconnect storm

| Step | Action |
|------|--------|
| Likely cause | Provider outage, local network flap, or invalid session. |
| 1 | Grafana: rate of `channel_reconnects_total` vs your baseline. |
| 2 | Metrics: `curl …/metrics \| grep channel_reconnects_total` |
| 3 | Logs: disconnect / reconnect lines; correlate time with provider status pages. |
| 4 | CLI: `assistclaw status` — confirm single expected process. |
| 5 | Mitigate: `assistclaw restart` after fixing config/network; escalate to **channel integration owner** if adapter bug suspected. |

### 3. High latency (P95)

| Step | Action |
|------|--------|
| Likely cause | Slow provider, queue backlog, or disk I/O on state volume. |
| 1 | Grafana: `message_latency_seconds` histogram / heatmap by `channel`. |
| 2 | Metrics snippet: `curl …/metrics \| grep message_latency_seconds` |
| 3 | Check DLQ not backing up: `assistclaw dlq list --limit 20` |
| 4 | Mitigate: shed load (disable non-critical cron/automation), scale out if you run multiple workers, or switch routing model per ops policy. |

### 4. Gateway crash / process gone

| Step | Action |
|------|--------|
| Likely cause | OOM, panic, or host reboot. |
| 1 | Scrape fails or `gateway_health` stuck at `0` in `/metrics`. |
| 2 | `assistclaw status` — not running? |
| 3 | `assistclaw start` (or your supervisor: `assistclaw service install` / systemd per [install.sh](../../install.sh)). |
| 4 | If still failing: foreground logs — `assistclaw gateway serve` in a tmux session **only in staging** (blocks terminal; dev/debug). |

### 5. Disk full on state directory

| Step | Action |
|------|--------|
| Likely cause | DLQ growth, huge logs, semantic/episodic DBs, or crash dumps filling `$STATE_DIR`. |
| 1 | **Free space:** `df -h "$STATE_DIR"` |
| 2 | **Largest dirs (macOS / Linux):** `du -sh "$STATE_DIR"/* 2>/dev/null \| sort -hr \| head -20` |
| 3 | Typical heavy paths: `channels/dlq.ndjson`, `logs/`, `memory/*.db`, `security/audit.ndjson`. |
| 4 | **DLQ:** archive then truncate per [dlq-inspection.md](dlq-inspection.md) retention; do not delete mid-incident without backup if you need evidence. |
| 5 | **Logs:** rotate or compress files under `$STATE_DIR/logs` per your log policy. |
| 6 | After space restored: `assistclaw restart` and confirm `curl …/metrics` shows `gateway_health` / scrape OK. |

**SQLite (optional):** if you use tools that support it, `sqlite3 "$STATE_DIR/memory/episodic.db" 'PRAGMA integrity_check;'` validates episodic DB after disk-pressure incidents (not required for every outage).

## Alert rules → intended action

Rules below mirror [metrics-slo-indicators.md](metrics-slo-indicators.md). Tune thresholds per environment.

| Alert | Default severity | Intended action |
|-------|------------------|------------------|
| `AssistClawHighFailureRate` | `page` | Page on-call; run **§1 High DLQ** and channel triage; open incident if SEV-1/2. |
| `AssistClawDLQGrowing` | `warning` | Ticket + investigate within business hours; escalate to **page** if paired with rising `messages_failed_total`. |
| *(Recommended)* High P95 latency | `ticket` or `page` | Add a recording rule or alert on `histogram_quantile(0.95, …)` per [internal SLOs](../observability/internal-slo-error-budgets.md); page if SLO burn policy triggers. |

## Ownership

| Area | Owner |
|------|--------|
| Channel adapters (Telegram, Slack, Discord, WhatsApp) | Channel integration owners |
| Gateway, `/metrics`, core agent, DLQ path defaults | Core AssistClaw maintainers |

## Quarterly drill

1. Schedule **60 minutes** on the team calendar (staging or read-only prod slice).  
2. Pick **one** scenario: DLQ growth **or** reconnect storm **or** disk full walkthrough.  
3. Record outcome in **First drill log** (below) or in a sprint issue.

### First drill log (template)

| Field | Value |
|-------|--------|
| Date | YYYY-MM-DD |
| Scenario exercised | e.g. DLQ inspection + metrics curl |
| Participants | |
| What went well | |
| Gaps (commands, access, dashboards) | |
| Follow-up issues | |

---

*Dependencies: STORY-007 (ops), STORY-008 (metrics), STORY-011 (SLOs).*
