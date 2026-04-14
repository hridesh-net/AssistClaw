# OpenTelemetry Tracing (Optional)

AssistClaw supports optional OpenTelemetry tracing for gateway, channel, and runner paths. Tracing is off by default to keep edge-device overhead low.

## Enable tracing

In `assistclaw.yaml`:

```yaml
tracing:
  enabled: true
  otlp_endpoint: "localhost:4317"
  service_name: "assistclaw"
  sample_ratio: 0.01
```

Environment variable expansion works in config values, so you can use:

```yaml
tracing:
  enabled: true
  otlp_endpoint: "${ASSISTCLAW_OTLP_ENDPOINT}"
```

## Sampling

- `sample_ratio` is 0..1.
- Default when enabled: `0.01` (1%).
- Sampler strategy: parent-based ratio sampling.

## Spans emitted

- `gateway.receive_message`
- `agent.enqueue_message`
- `agent.model_call`
- `agent.send_reply`
- `adapter.send`

## Local Jaeger quickstart

Run Jaeger all-in-one with OTLP gRPC:

```bash
docker run --rm -p 16686:16686 -p 4317:4317 jaegertracing/all-in-one:latest
```

Then:

- set `tracing.enabled: true`
- set `tracing.otlp_endpoint: "localhost:4317"`
- start AssistClaw and generate traffic
- open [http://localhost:16686](http://localhost:16686), service `assistclaw`

## Failure behavior

- If exporter initialization fails, AssistClaw logs a warning and continues with tracing disabled.
- If exporter is unreachable later, errors are logged once (no crash).

## Performance note

- At 1% sampling, expected overhead is low for normal workloads.
- Keep tracing disabled on constrained devices unless debugging latency regressions.
