# STORY-071 — SIEM export: Splunk HEC, Datadog, generic JSON Lines

| Field | Value |
|-------|--------|
| **Sprint** | sprint-13 |
| **Type** | Enterprise / Security |
| **Priority** | P0 |
| **Estimate** | M |

## Summary

Ship **SIEM integration**: forward audit logs and security events to Splunk HTTP Event Collector, Datadog Logs API, or a generic JSON Lines file tail-friendly output.

## User story

**As a** SOC analyst**  
**I want** centralized logs**  
**So that** we can alert and investigate.

## Acceptance criteria

1. **At least two** exporters implemented or one exporter + generic format documented as extensible.
2. **Authentication** for outbound export secure (tokens via env, rotated).
3. **Backpressure** handling: drop vs block (document).
4. **Tests**: unit tests for batching and redaction.
5. **Doc**: sample dashboards/alerts.

## Definition of Done

- [ ] Pilot customer validates ingestion (staging).

## Dependencies

- STORY-020, STORY-063.

## Risks

- Log volume cost; sampling policy documented.
