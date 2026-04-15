# MemPalace (upstream) integration — rollout and rollback

[MemPalace](https://github.com/MemPalace/mempalace) is a **Python** memory system. AssistClaw integrates it the same way other MCP-capable clients do: **spawn the MemPalace MCP server** and register its tools on the agent.

This is **not** the same thing as `agent.palace` in `assistclaw.yaml`, which only tweaks AssistClaw’s built-in sqlite-vec retrieval. For the real MemPalace product, follow this runbook.

## Prerequisites

1. Install MemPalace: `pip install mempalace` (use a venv if you prefer).
2. Initialize a palace/world (see MemPalace README): `mempalace init <path>`.
3. Mine or import data as needed (`mempalace mine`, etc.).

## Wire MemPalace into AssistClaw

### Option A: explicit `mcp.clients` (Docker / systemd / Compose)

Under `mcp.clients`, add a stdio client that runs the upstream MCP entrypoint:

```yaml
mcp:
  clients:
    - name: mempalace
      transport: stdio
      command: python
      args: ["-m", "mempalace.mcp_server"]
```

Use the same `python` that has `mempalace` installed (full path or venv `python` if needed).

Restart AssistClaw (`assistclaw start` / your process supervisor). On startup the agent connects to each MCP client and registers tools as `mcp:mempalace:<tool_name>` (for example `mcp:mempalace:mempalace_search`).

### Option B: binary install — auto-start child (no `mcp.clients` entry)

For a single-machine install where MemPalace is `pip install`’d into a Python environment, AssistClaw can spawn the MCP server as a **stdio child process** on startup (same lifecycle as the agent):

```yaml
memory:
  mempalace:
    auto_start: true
    # python_executable: /path/to/venv/bin/python   # optional; default python3
```

Set `ASSISTCLAW_MEMPALACE_PYTHON` if you prefer not to hardcode the interpreter in YAML. If `auto_start: true` and no `mcp.clients` entry exists for `mcp_client_name` (default `mempalace`), AssistClaw injects an equivalent stdio client internally and logs `mcp: MemPalace auto-start (stdio child process)`.

### Option C: fully automated managed venv (minimal manual steps)

If you have a system `python3` with the stdlib `venv` module, AssistClaw can create `<state_dir>/mempalace/venv`, `pip install mempalace`, run `mempalace init` once for `<state_dir>/mempalace/world`, and start the MCP server from that venv (working directory set to the world):

```yaml
memory:
  mempalace:
    enabled: true
    auto_start: true
    managed_venv: true
    # bootstrap_python: python3.12   # optional; or ASSISTCLAW_MEMPALACE_BOOTSTRAP_PYTHON
```

First agent start (or `assistclaw mempalace setup`) may take several minutes while PyPI packages install. Use `assistclaw mempalace doctor` to verify paths and imports.

From the repo installer: `WITH_MEMPALACE=1 bash install.sh` (Unix) or `.\install.ps1 -WithMemPalace` (Windows) runs the same bootstrap using `--state-dir` so no `assistclaw.yaml` is required yet. `bash uninstall.sh` removes the managed `mempalace/` tree by default (`--keep-mempalace` to retain it).

## Optional: merge MemPalace into `memory_search`

To append MemPalace semantic results to the built-in `memory_search` tool (episodic + AssistClaw semantic), enable:

```yaml
memory:
  mempalace:
    enabled: true
    mcp_client_name: mempalace   # must match mcp.clients[].name
    inject_into_memory_search: true
    search_limit: 5              # 0 = use the same limit as memory_search
```

If `memory.mempalace.enabled` is true, config validation requires either a matching `mcp.clients` entry **or** `memory.mempalace.auto_start: true`. Option C satisfies this via the synthetic stdio client injected when `auto_start` is true.

## Rollout sequence

1. Add MemPalace as MCP only (`mcp.clients` or `auto_start`, optionally with `managed_venv`); leave `memory.mempalace.enabled: false`. Confirm the model can call `mcp:mempalace:mempalace_*` tools (see tool list / `find_tools`).
2. Enable `memory.mempalace.inject_into_memory_search` so `memory_search` also surfaces MemPalace hits.
3. Tune `search_limit` and monitor latency.

## Rollback

- **Stop using MemPalace tools:** remove the `mcp.clients` MemPalace entry, set `memory.mempalace.auto_start: false`, or set `memory.mempalace.enabled: false` and `inject_into_memory_search: false`, then restart AssistClaw.
- **AssistClaw-only retrieval:** set `agent.palace.enabled: false` if you had enabled local routing experiments.

## Verification

- Logs show `mcp: registered external server tools` with `server=mempalace` and a non-zero tool count.
- A query that should hit mined content returns rows from `mcp:mempalace:mempalace_search` or `[mempalace]` lines inside `memory_search` when merge is enabled.

## Related docs

- Upstream MCP setup (from MemPalace README): `python -m mempalace.mcp_server`
- Local AssistClaw mining (separate from MemPalace): [memory-mining.md](memory-mining.md)
