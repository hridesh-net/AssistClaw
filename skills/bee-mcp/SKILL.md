---
name: bee-mcp
description: "Connect AssistClaw to a Bee wearable via BeeMCP (MCP server) without using bee CLI/proxy. Use when: BeeMCP is already installed or user wants Bee tools exposed through MCP lazy-loading."
homepage: https://github.com/OkGoDoIt/beemcp
metadata:
  {
    "assistclaw":
      {
        "emoji": "🐝",
        "requires": { "bins": ["uvx"], "env": ["BEE_API_KEY"] },
        "primaryEnv": "BEE_API_KEY"
      }
  }
---

# BeeMCP Skill

Use this when you want Bee wearable context in AssistClaw through MCP only,
without the Bee CLI (`@beeai/cli`) and without `bee proxy`.

## When to Use

✅ **USE this skill when:**

- You already have a Bee developer API key.
- You want Bee tools exposed as MCP tool namespaces (`bee:*`).
- You prefer one integration path for multiple MCP clients.

❌ **DON'T use when:**

- You need `bee sync` markdown exports (use `skills/bee`).
- You rely on Bee CLI commands like `bee today` / `bee now`.

## Setup

### 1. Export Bee API key

```bash
export BEE_API_KEY="your_bee_api_key"
```

### 2. Register BeeMCP with AssistClaw

```bash
assistclaw mcp add --name bee \
  --command "uvx beemcp" \
  --env BEE_API_KEY=$BEE_API_KEY
```

### 3. Verify the server and tools

```bash
assistclaw mcp status
assistclaw mcp list-tools | grep '^bee:'
```

## Typical Workflow

Ask AssistClaw naturally after BeeMCP is connected:

- "What did I talk about in the last hour?"
- "Search my Bee memory for project roadmap notes."
- "Create a todo from this summary."

The agent will lazy-load BeeMCP tool schemas and call them as needed.

## Optional: Add BeeMCP in config

```yaml
mcp:
  clients:
    - name: bee
      command: "uvx beemcp"
      env:
        - "BEE_API_KEY=${BEE_API_KEY}"
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| No `bee:*` tools listed | Confirm `BEE_API_KEY` is exported in the daemon environment |
| MCP client fails to spawn | Verify `uvx` exists: `uvx --version` |
| Tool calls return auth errors | Rotate key in Bee dashboard and restart AssistClaw |

## Notes

- BeeMCP is a community integration; APIs may change between releases.
- Keep `BEE_API_KEY` in environment/secret manager, never hardcode in files.
- If you need local markdown exports and proxy endpoints, use `skills/bee`.
