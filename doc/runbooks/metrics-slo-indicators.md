# Metrics and SLO Indicators

This runbook defines the default Prometheus-compatible metrics exposed by AssistClaw at `GET /metrics` on the gateway.

## Required SLO metrics

- `messages_sent_total{channel}`: outbound successful sends.
- `messages_failed_total{channel}`: outbound failed sends.
- `message_latency_seconds{channel}`: outbound send latency histogram.
- `dlq_depth{channel}`: dead-letter queue depth gauge.
- `channel_reconnects_total{channel}`: reconnect events.
- `adapter_retries_total{channel}`: retry attempts.
- `gateway_health`: 1 when gateway is up, 0 after shutdown.

## Additional operational metrics

- `messages_received_total{channel}`: inbound message events observed by runner ingress.

## Label policy (cardinality controls)

Allowed labels:

- `channel`

Forbidden labels:

- `user_id`
- `session_id`
- `request_id`
- arbitrary message text
- dynamic IDs from providers

Rationale: only low-cardinality labels are allowed on counters/histograms to avoid memory and TSDB cardinality explosions.

## Scrape integration

- Endpoint: `/metrics`
- Content-Type: `text/plain; version=0.0.4; charset=utf-8`
- If gateway Bearer token is configured, include `Authorization: Bearer <token>` in scrape config.

Example scrape config snippet:

```yaml
scrape_configs:
  - job_name: assistclaw
    metrics_path: /metrics
    static_configs:
      - targets: ["127.0.0.1:18789"]
    authorization:
      type: Bearer
      credentials: "<gateway-token>"
```

## Alert examples

```yaml
groups:
  - name: assistclaw-observability
    rules:
      - alert: AssistClawHighFailureRate
        expr: sum(rate(messages_failed_total[5m])) / clamp_min(sum(rate(messages_sent_total[5m])), 1) > 0.05
        for: 10m
        labels:
          severity: page
        annotations:
          summary: "AssistClaw failure rate above 5%"
          description: "Outbound failure ratio exceeded threshold for 10m."

      - alert: AssistClawDLQGrowing
        expr: sum(dlq_depth) > 25
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "AssistClaw DLQ depth is high"
          description: "Dead-letter queue depth exceeded 25 messages."
```
