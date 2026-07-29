# Hermes Agent (github.com/nousresearch/hermes-agent) and Nous Portal

## What it is

Hermes Agent ("The Agent That Grows With You") is Nous Research's open-source (MIT), self-improving personal AI agent. Its differentiator is a built-in learning loop: it autonomously creates skills from experience, improves them during use, nudges itself to persist memories, searches its own past conversations (SQLite FTS5 plus LLM summarization), and builds a persistent user model across sessions. One agent identity spans CLI, Telegram, Discord, Slack, WhatsApp, Signal, and Email via a gateway process. Positioned as a successor/alternative to OpenClaw, with migration tooling that preserves MEMORY.md/USER.md.

Runtime: Python 3.11+ core agent plus Node.js (Electron desktop app; Tauri bootstrap installer), uv package management, with ripgrep and ffmpeg dependencies. Very high adoption (README claims ~222k stars — treat as approximate), v0.19.0, active development.

## Architecture

- Agent loop (agent/conversation_loop.py run_conversation): while api_call_count < max_iterations: check interrupt flag → build api_messages (inject ephemeral context: memory prefetch, plugin hooks) → pre-API compression check (should_compress(request_pressure_tokens)) → provider call with retry → dispatch tool calls → append results. A per-turn build_turn_context does one-time setup. _sanitize_api_messages runs unconditionally, stripping orphaned tool results and stubbing missing ones.
- Retry/failover: classify_api_error, activate fallback provider mid-turn, resync the system message, and re-apply provider-specific prompt-cache decorations on failover.
- Prompt-cache prefix stability is a first-class concern: the system prompt is restored from the session DB byte-for-byte; interrupt-and-redirect preserves streamed text and appends the user correction as a new message so cached messages stay byte-for-byte unchanged. If compression runs mid-turn it refunds the API-call budget and re-attempts.
- Tool system: 40+ tools (file ops, terminal, web search/extract, browser automation, vision, image gen, TTS, subagents, scheduling). Six terminal/sandbox backends: local, Docker, SSH, Singularity, Modal (serverless), Daytona. Toolsets can be reduced per config — recommended for slow/small models. Subagents get isolated conversations/terminals. Natural-language cron scheduling via the gateway.
- Context/memory: context_engine.py is an abstract, pluggable engine — should_compress() gate after each response, compress(messages) → shorter list, per-turn select_context(), and engines can expose their own tools. Defaults: compression threshold at 75% of context_length, protect first 3 messages (system + opening) and last 6 (recent turns). Preflight pruning trims old tool outputs (prune_tool_results_only) before full compression fires — a cheap first line of defense. Known gap handled in code: provider-reported prompt tokens lag a just-appended huge tool result, so pressure is also measured pre-request from actual request tokens.
- Memory layers: agent-curated memory files with periodic self-nudges (iteration counters trigger "persist what you learned" nudges), FTS5 full-text session search plus LLM summarization for cross-session recall, Honcho dialectic user modeling, and context files at project and user scope. Injected memory context is truncated middle-out (head + tail with a marker) to a max char budget.
- Learning loop: observe (episodic tracking of multi-step tasks) → distill (after ~3 similar successes, generate a SKILL.md per the agentskills.io open standard) → reuse → refine (patches its own skills when it hits dead ends; grades skills on usage telemetry; archives skills unused for 90 days; only self-authored skills are auto-modified).
- Providers: pluggable — Nous Portal, OpenAI, Anthropic, Bedrock, Azure, OpenRouter, any custom OpenAI-compatible endpoint. Model switching is a config/command change. A fallback-provider chain is built into the loop.
- Nous Portal integration: OAuth login, refresh token on disk, short-lived JWT minted per inference call (no long-lived API keys), auto-quarantine of invalidated credentials. A Tool Gateway makes one subscription cover web search/extract (Firecrawl), image gen (FAL), TTS (OpenAI), cloud browser (Browser Use), and cloud terminal (Modal).

## Efficiency and edge relevance

- Claim: "Run it on a $5 VPS, a GPU cluster, or serverless infrastructure that costs nearly nothing when idle." Serverless backends hibernate when idle. Runs on Termux/Android and ARM.
- But the runtime is Python + Node + ffmpeg — hundreds of MB, nontrivial RAM, slow cold start. No compiled single-binary story (that is AssistClaw/Go's advantage).
- Local models: any OpenAI-compatible endpoint (Ollama, LM Studio, vLLM, SGLang, llama.cpp server, LocalAI). Docs call llama.cpp "the lightweight CPU, Apple Metal, and edge-server route."
- Hard floor: the model must have ≥64,000-token context or Hermes rejects it at startup. Community hardware guidance: 32GB+ RAM for agent-capable local models. This is brutal on edge — KV cache for 64k context on a 7-14B model can exceed Pi-class RAM.
- For slow/small models, docs advise a smaller model plus reducing active toolsets (fewer tool schemas in the prompt = less context, fewer confusion points).
- Small-model steering comes mostly from the model side (the Hermes model line): hybrid reasoning with explicit <think></think> segments (skippable for speed), ChatML, XML-wrapped JSON tool calling (<tools> schema block in the system prompt, model emits <tool_call>{json}</tool_call>), recommended sampling temperature 0.6, top_p 0.95, top_k 20. Batch trajectory generation lets the agent log trajectories for training small models on agent work.

## Key design ideas worth stealing

1. Pluggable context-engine interface rather than a baked-in compressor: should_compress(pressure_tokens), compress(msgs), select_context(), prune_tool_results_only(), optional engine-provided tools. Defaults: 75% threshold, protect first 3 + last 6 messages, prune old tool results before summarizing. Cheap, model-light, ideal on edge. Measure pressure pre-request; don't trust lagging provider-reported token counts.
2. Prompt-cache-stability discipline: byte-for-byte stable system prompt and history restored from the session DB, per-provider cache decoration reapplied on failover, mid-turn redirects that never mutate cached messages.
3. Self-nudging learning loop as plain files: loop counters trigger "persist memory"/"create skill" nudges; skills are SKILL.md files; FTS5 session search for recall. AssistClaw already ships SQLite+FTS5, so cross-session recall and skill distillation are nearly free and fully offline.
4. Small-model ergonomics = fewer tools plus strict format: shrink the active toolset per deployment; use ChatML + XML-wrapped JSON tool calls that small Hermes/Qwen-family models are trained on; fixed sampling (0.6/0.95/20); optional think mode toggled per request for latency control; enforce a context floor per model instead of degrading silently.
5. Provider failover chain with error classification, plus one gateway subscription for tools. For AssistClaw: local-model-first with a cloud fallback chain is the availability-over-features play.

## Nous Portal

- inference-api.nousresearch.com/v1, OpenAI-compatible. OAuth + short-lived JWTs.
- Tiers: Free $0, Plus $20/mo, Super $100/mo, Ultra $200/mo (credits slightly exceed price; tiers discount usage).
- 300+ models including Claude, GPT, Gemini, DeepSeek, Qwen, Kimi; Hermes-4-70B at ~$0.05/1M input, $0.20/1M output.
- Open weights: Hermes 4 family = 14B (Qwen3-14B base), 70B, 405B (Llama-3.1 base); GGUF quantizations available, runs in llama.cpp/ollama.
- Their own docs caveat: Hermes-4 models are "not recommended for use inside Hermes Agent" — tuned for chat/reasoning, not tool-calling efficiency. Don't assume small Hermes models are good agentic tool-callers out of the box.
- Pi-class hardware: only Hermes-4-14B quantized (~8-9GB) is remotely feasible, and only on a 16GB Pi 5 at slow tok/s; the older Hermes 3 line had 8B/3B options better suited to Pi.

## Weaknesses and tradeoffs for edge

- Python+Node+ffmpeg runtime: large install, high baseline RAM, slow cold start versus a single Go binary.
- 64k-token minimum context requirement rejects small-context local models outright rather than degrading gracefully.
- Learning loop and memory summarization depend on LLM calls — background token spend, or latency when the only model is a slow local one.
- 40+ tools by default means a huge system prompt; must manually trim toolsets for small models.
- Docker/Singularity/Modal sandboxes assume server-class environments; wearable-class isolation not addressed.
- The multi-platform gateway is a separate long-running process (RAM cost).

## Graph facts

- hermes_agent|developed_by|Nous Research
- hermes_agent|written_in|Python + Node.js
- hermes_agent|successor_to|OpenClaw
- hermes_agent|context_management_via|pluggable ContextEngine
- context_engine|default_compression_threshold|75% of context_length
- context_engine|protects|first 3 and last 6 messages
- context_engine|preflight_strategy|prune old tool results first
- hermes_agent|memory_search_via|SQLite FTS5 + LLM summarization
- hermes_agent|user_modeling_via|Honcho dialectic pattern
- hermes_agent|learning_loop|observe, distill SKILL.md, reuse, refine
- hermes_agent|skills_conform_to|agentskills.io standard
- hermes_agent|terminal_backends|local, Docker, SSH, Singularity, Modal, Daytona
- hermes_agent|channels|CLI, Telegram, Discord, Slack, WhatsApp, Signal, Email
- hermes_agent|requires_model_context|64k tokens minimum
- hermes_agent|supports_local_inference_via|Ollama, LM Studio, vLLM, llama.cpp
- hermes_agent|failover|error classification + fallback provider + cache redecoration
- hermes_agent|optimizes_for|prompt-cache byte stability
- nous_portal|inference_endpoint|OpenAI-compatible v1 API
- nous_portal|model_count|300+
- nous_portal|auth_model|OAuth refresh token + short-lived JWT
- hermes_4|sizes|14B, 70B, 405B open weights
- hermes_4_14b|base_model|Qwen3 14B
- hermes_4|tool_call_format|ChatML + XML-wrapped JSON tool_call
- hermes_4|recommended_sampling|temp 0.6, top_p 0.95, top_k 20
- hermes_4|reasoning_mode|hybrid toggleable think tags
- hermes_4|caveat|not recommended for tool-calling inside Hermes Agent
