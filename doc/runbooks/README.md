# Runbooks

Operational guides for AssistClaw. Start here during incidents, then open the specific runbook.

| Runbook | Use when |
|---------|----------|
| [On-call: dashboards, alerts, triage](on-call-dashboards-alerts.md) | **First stop** — triage paths, CLI/curl commands, severities (STORY-012) |
| [Metrics and SLO indicators](metrics-slo-indicators.md) | Metric names, `/metrics` scrape, alert YAML examples |
| [Internal SLOs & error budgets](../observability/internal-slo-error-budgets.md) | Targets, budgets, monthly review |
| [DLQ inspection](dlq-inspection.md) | Outbound failures, DLQ file format, replay notes |
| [Structured logging & correlation IDs](structured-logging-correlation-ids.md) | Log fields, tracing a request across components |
| [OpenTelemetry tracing (optional)](opentelemetry-tracing.md) | Trace export and sampling |
| [Load test & failure injection](load-test-and-failure-injection.md) | Harness, regression triage |
| [MemPalace rollout / rollback](mempalace-rollout-rollback.md) | Optional MemPalace MCP |
| [Memory mining](memory-mining.md) | Taxonomy mining / backfill |
