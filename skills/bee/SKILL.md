---
name: bee
description: "Query and control a Bee wearable (bee.computer) — conversations, facts, todos, journals, daily briefs, and real-time event streams. Use when: user references their Bee device, asks what they just talked about, wants a daily brief, manages facts/todos captured by Bee, or searches their personal lifelog. NOT for: generic calendar/reminder apps, historical data older than the Bee account, or non-Bee wearables."
homepage: https://docs.bee.computer/
metadata:
  {
    "assistclaw":
      {
        "emoji": "🐝",
        "requires": { "bins": ["bee", "curl", "jq"] },
        "primaryEnv": "BEE_TOKEN"
      }
  }
---

# Bee Skill

Connects AssistClaw to your Bee wearable (https://bee.computer) — the always-on
personal AI that captures conversations, learns facts, and tracks todos.

There are **two transports** this skill can use, pick whichever is available:

1. **`bee` CLI** — easiest, returns markdown by default, JSON with `--json`.
2. **Local HTTP proxy** (`bee proxy`) — REST API under `/v1/*` on
   `http://127.0.0.1:8787`. Best for scripting, streaming, and tool chains.

A third option — the community **BeeMCP** server — can also be registered as
an external MCP client (see "MCP alternative" at the bottom).

## When to Use

✅ **USE this skill when:**

- "What was I just talking about?" / "Summarise the meeting I just had"
- "Give me my daily brief" / "What's on my plate today?"
- "Add a todo from Bee: ..." / "Mark that reminder done"
- "Who is <name> and how do I know them?"
- "Search my Bee memory for <topic>"
- "Tail my Bee stream for new utterances / todos"

❌ **DON'T use when:**

- The user wants generic reminders with no connection to Bee captures → use
  `apple-reminders` / `things-mac`.
- Historical events predating Bee account creation.
- Non-Bee wearables (Friend, Limitless, Rewind, etc.).

## Setup (one time)

### 1. Enable Developer Mode in the Bee iOS app

Settings → tap the app **Version** 5 times until "Developer Mode" unlocks.

### 2. Install the CLI

```bash
npm install -g @beeai/cli
bee version
```

### 3. Authenticate

```bash
bee login                 # opens browser
bee status                # verifies auth
bee ping                  # server reachability
```

Credentials are stored by the CLI; no env var is strictly required for CLI
calls. For **direct HTTP** (bypassing `bee proxy`) export:

```bash
export BEE_TOKEN="<personal token from bee login>"
export BEE_API_BASE="https://api.bee.computer"   # whatever CLI uses
```

### 4. Verify data access

```bash
bee me --json | jq '.id, .email'
bee today
```

## CLI Recipes

Markdown is the default output; add `--json` for structured data you want to
pipe into `jq` or into another tool call.

### Daily context

```bash
bee today                 # morning brief (calendar, email, summary)
bee now                   # conversations in the last ~10h
bee me                    # user profile
```

### Conversations

```bash
bee conversations list --limit 20
bee conversations list --json --limit 5 | jq '.[].id'
bee conversations show <id>
```

### Facts (personal knowledge Bee has inferred)

```bash
bee facts list
bee facts add "I prefer oat milk in my coffee"
bee facts update <id> --text "..." --confirmed
bee facts delete <id>
```

### Todos

```bash
bee todos list
bee todos add "Call the dentist" --alarm-at 2026-04-20T09:00:00Z
bee todos update <id> --completed
bee todos delete <id>
```

### Journals & daily summaries

```bash
bee journals list
bee daily list
bee daily show <id>
```

### Search

```bash
bee search --query "project roadmap"                  # fast BM25
bee search --query "what do you know about me" --neural   # semantic
```

### Local mirror (markdown files on disk)

Great for letting AssistClaw's `memory` subsystem ingest Bee content:

```bash
bee sync                         # writes ./bee-sync/**/*.md
bee sync --out ~/.assistclaw/bee
```

Then feed it into semantic memory:

```bash
assistclaw memory ingest ~/.assistclaw/bee --tag bee
```

## HTTP Proxy Recipes (`bee proxy`)

Start the proxy once per session (bind to loopback only — do **not** expose):

```bash
bee proxy --port 8787 >/tmp/bee-proxy.log 2>&1 &
BEE=http://127.0.0.1:8787/v1
```

### User & changes

```bash
curl -s "$BEE/me" | jq
curl -s "$BEE/changes" | jq                # since default window
curl -s "$BEE/changes?cursor=<id>" | jq
```

### Facts

```bash
curl -s "$BEE/facts" | jq
curl -s "$BEE/facts/<id>" | jq
curl -s -X POST "$BEE/facts" -H 'Content-Type: application/json' \
     -d '{"text":"I live in Rome"}' | jq
curl -s -X PUT "$BEE/facts/<id>" -H 'Content-Type: application/json' \
     -d '{"text":"Updated","confirmed":true}'
curl -s -X DELETE "$BEE/facts/<id>"
```

### Todos

```bash
curl -s "$BEE/todos" | jq
curl -s -X POST "$BEE/todos" -H 'Content-Type: application/json' \
     -d '{"text":"Buy groceries","alarm_at":"2026-04-18T17:00:00Z"}' | jq
curl -s -X PUT "$BEE/todos/<id>" -H 'Content-Type: application/json' \
     -d '{"completed":true}' | jq
curl -s -X DELETE "$BEE/todos/<id>"
```

### Conversations, journals, daily

```bash
curl -s "$BEE/conversations?limit=10" | jq '.[].id'
curl -s "$BEE/conversations/<id>" | jq
curl -s "$BEE/journals" | jq
curl -s "$BEE/daily" | jq
```

### Search

```bash
curl -s -X POST "$BEE/search/conversations" \
     -H 'Content-Type: application/json' \
     -d '{"query":"project roadmap","limit":20}' | jq

curl -s -X POST "$BEE/search/conversations/neural" \
     -H 'Content-Type: application/json' \
     -d '{"query":"anything about my parenting goals","limit":20}' | jq
```

### Real-time stream (SSE)

Use for `assistclaw cron` / event-driven automations.

```bash
curl -Ns "$BEE/stream?types=new-utterance,todo-created,journal-created" \
  | while IFS= read -r line; do
      case "$line" in
        data:*) echo "event=${line#data: }" ;;
      esac
    done
```

Common event types:

- `new-conversation`, `update-conversation`, `update-conversation-summary`,
  `delete-conversation`
- `new-utterance`
- `todo-created`, `todo-updated`, `todo-deleted`
- `journal-created`, `journal-updated`, `journal-deleted`, `journal-text`

## AssistClaw Integration Patterns

### Pattern 1 — "What did I just talk about?"

```bash
bee now --json | jq -r '.[0].summary // .[0].transcript'
```

Pipe into the agent's summariser skill if needed.

### Pattern 2 — Morning briefing on REPL open

Add to `~/.assistclaw/hooks/pre-session.sh`:

```bash
bee today 2>/dev/null || true
```

### Pattern 3 — Live capture to semantic memory

One-shot daemon (e.g. via `assistclaw cron`):

```bash
curl -Ns http://127.0.0.1:8787/v1/stream?types=new-utterance \
  | while read -r line; do
      [[ $line == data:* ]] || continue
      payload=${line#data: }
      text=$(echo "$payload" | jq -r '.utterance.text // empty')
      [[ -n $text ]] && assistclaw memory add --tag bee --text "$text"
    done
```

### Pattern 4 — Bee-driven todo → AssistClaw skill dispatch

Tail `todo-created` and route to the right downstream skill:

```bash
curl -Ns "$BEE/stream?types=todo-created" | jq -c --unbuffered '.' \
  | while read -r evt; do
      text=$(echo "$evt" | jq -r '.todo.text')
      assistclaw agent -m "Triage this new Bee todo and act: $text"
    done
```

### Pattern 5 — Write new facts learned during an AssistClaw session back
to Bee so the wearable stays in sync:

```bash
curl -s -X POST "$BEE/facts" -H 'Content-Type: application/json' \
     -d "$(jq -n --arg t "$FACT" '{text:$t}')"
```

### Pattern 6 — Persistent Bee stream ingester via `assistclaw cron`

This survives daemon restarts because `assistclaw cron add` persists jobs in
`~/.assistclaw/cron_jobs.json`.

```bash
assistclaw cron add "@every 1m" \
"pgrep -f 'bee proxy --port 8787' >/dev/null || bee proxy --port 8787 >/tmp/bee-proxy.log 2>&1 &"

assistclaw cron add "0 */5 * * * *" \
"curl -Ns http://127.0.0.1:8787/v1/stream?types=new-utterance \
| while read -r line; do
    [[ \$line == data:* ]] || continue
    payload=\${line#data: }
    text=\$(echo \"\$payload\" | jq -r '.utterance.text // empty')
    [[ -n \$text ]] && assistclaw memory add --tag bee --text \"\$text\"
  done"
```

For launchd/systemd users, keep one long-running stream process under the OS
supervisor instead of cron; cron works best for periodic sync jobs.

## Direct Cloud API (no proxy)

If you need to call Bee without `bee proxy` (e.g. from a remote server):

```bash
curl --cacert ./bee-ca.pem \
     -H "Authorization: Bearer $BEE_TOKEN" \
     "$BEE_API_BASE/v1/me"
```

> Bee uses a **private CA**. System trust store is **not** enough — pin
> `bee-ca.pem` (shipped by the CLI) in your HTTP client.

## MCP Alternative

If you prefer the lazy-loaded MCP path (Bee tools appear in AssistClaw's
`Map of Content`), register the community BeeMCP server once:

```bash
# Prereq: a Bee developer API key
export BEE_API_KEY="..."

assistclaw mcp add --name bee \
  --command "uvx beemcp" \
  --env BEE_API_KEY=$BEE_API_KEY

assistclaw mcp list-tools | grep ^bee:
```

Tools then show up as `bee:conversations.search`, `bee:facts.list`, etc.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `bee login` fails on "Developer Mode required" | Open iOS app → Settings → tap Version 5× |
| `bee proxy` returns 401 | Re-run `bee login`; check `bee status` |
| `curl` to cloud API fails with TLS error | Use `--cacert bee-ca.pem` (private CA) |
| `bee sync` empty | Confirm recent captures in the iOS app first |
| SSE stream hangs with no events | Use `curl -N` (no buffering) and filter `?types=` |

## Notes

- The proxy is **localhost-only**; never expose to the internet.
- Facts and todos round-trip both ways — updates from AssistClaw appear in the
  iOS app within seconds.
- Conversations cannot currently be created via API (read-only).
- Neural search is slower (~1-3s) but far better for fuzzy recall.
- Rate limit on cloud API ≈ 60 req/min per token; back off on 429.
