# Structured Logging and Correlation IDs

This runbook defines the standard logging fields for request correlation across gateway, channel ingress, and runner/tool execution.

## Standard fields

- `request_id`: unique per inbound request/event; propagated through context.
- `session_id`: AssistClaw session key (for example `tg:123` or UUID web session).
- `channel`: origin channel (`telegram`, `discord`, `slack`, `gateway`, `webhook`).
- `trace_id`: optional distributed trace id from `X-Trace-Id` or `traceparent`.
- `tool`: tool name for tool execution logs.
- `path`, `method`, `remote_addr`: gateway ingress fields.

## Field naming and propagation

- HTTP/WS ingress sets or preserves `request_id` from `X-Request-Id`.
- If absent, server generates a UUID and returns it as `X-Request-Id`.
- `trace_id` is accepted from:
  - `X-Trace-Id`
  - W3C `traceparent` header
- Runner/channel code carries correlation fields in `context.Context`.

## Log levels

- `INFO`: inbound request/message, tool call lifecycle, webhook execution outcomes.
- `DEBUG`: high-frequency internals (model iteration details, noisy path traces).
- `WARN`: degraded behavior, transient failures, guardrail warnings.
- `ERROR`: failed tool execution, request failures, unrecoverable channel/gateway errors.

## PII and redaction

- Do not log message bodies or tokens in full by default.
- Keep user text/tool IO truncated and minimal in app logs.
- Security/audit redaction policy remains aligned with Sprint-04 controls (`security.pii_mask` and guardrail/audit policy).

## Sample queries

### Loki

- `{app="assistclaw"} | json | request_id="REQ_ID_HERE"`
- `{app="assistclaw"} | json | session_id="tg:123456"`

### Datadog Logs

- `service:assistclaw @request_id:REQ_ID_HERE`
- `service:assistclaw @session_id:tg:123456 @channel:telegram`

### Splunk

- `index=assistclaw request_id="REQ_ID_HERE"`
- `index=assistclaw session_id="tg:123456" channel="telegram"`
