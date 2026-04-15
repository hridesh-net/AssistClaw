# Memory Mining Runbook

This runbook covers AssistClaw’s **built-in** markdown → sqlite-vec indexing (`assistclaw memory mine …`). That is separate from the **[MemPalace](https://github.com/MemPalace/mempalace)** project, which has its own CLI (`mempalace mine`, ChromaDB, MCP server). To use MemPalace inside AssistClaw, see [mempalace-rollout-rollback.md](mempalace-rollout-rollback.md).

## Commands

- Validate config and embedder readiness:
  - `assistclaw memory mine validate`
- Run incremental mining:
  - `assistclaw memory mine run`
- Run full backfill (explicit confirmation required):
  - `assistclaw memory mine backfill --yes`
- Check last run status:
  - `assistclaw memory mine status`

## Recommended Rollout

1. Keep `agent.palace.enabled: false`.
2. Run `assistclaw memory mine validate`.
3. Run `assistclaw memory mine run --dry-run`.
4. Run `assistclaw memory mine run`.
5. Confirm `assistclaw memory mine status` shows low/zero errors.

## Troubleshooting

- `no embedding provider available for mining`:
  - Configure at least one `embeddings.priority` provider and credentials.
- Frequent indexing errors:
  - Reduce `memory.mining.max_files_per_run` and rerun.
- Large files skipped:
  - Increase `memory.mining.max_file_size_kb` or split files.
