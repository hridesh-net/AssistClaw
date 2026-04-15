# AssistClaw local palace routing baseline

This sheet tracks benchmarks for **AssistClaw’s optional sqlite-vec taxonomy routing** (`agent.palace`, heuristic router). It does **not** measure the upstream **[MemPalace](https://github.com/MemPalace/mempalace)** Python system (use MCP + `memory.mempalace` and profile end-to-end separately).

## Benchmark Command

- `go test ./internal/memory -bench BenchmarkPalaceRouting -benchmem`

## Baseline Table

| Scenario | ns/op | B/op | allocs/op | Notes |
|---|---:|---:|---:|---|
| BenchmarkPalaceRouting_FilterDocs | TBD | TBD | TBD | Route filter over 3000 docs |
| BenchmarkPalaceRouting_RouteQuery | TBD | TBD | TBD | Query taxonomy classification |

## Comparison Guidance

- Target improvement: lower retrieval noise with neutral or improved latency.
- Acceptable regression guardrail: <=10% latency increase if relevance clearly improves.
- Keep `agent.palace.fail_open: true` during first rollout of local routing.
