# Skill Graphs in AssistClaw

> *Don't load skills. Navigate them.*

## What is a Skill Graph?

Traditionally, every skill's full text gets injected into the LLM's system prompt — every time. As you add more skills, your context window fills up with instructions the agent doesn't even need for the current task.

A **Skill Graph** flips this:
- The agent sees a **Map of Content**: short summaries of what's available.
- It reads specific nodes **on demand** using the `read_skill_node` tool.
- Your prompt stays small and precise, always.

Think of it like a book: you don't read every chapter before answering a question. You check the table of contents, find the right chapter, and read only that.

---

## Context Reduction: The Numbers

A benchmark with 6 nodes (~2,950 words of content each):

| | Size |
|---|---|
| **Old approach** (full text in prompt) | ~17,700 chars |
| **New approach** (Map of Content) | ~690 chars |
| **Reduction** | **96.1%** |

Using a 200K context window (Claude), the old approach would exhaust ~65 tokens per node per request. With larger skill libraries (20–50 skills), this compounds dramatically.

---

## Skill Directory Structure

A skill is a directory under your `skills_dir`. Every `.md` file is automatically indexed as a **node**.

```
~/.assistclaw/skills/
└── legal-ai/
    ├── SKILL.md              # Primary node — entry point, metadata
    ├── contracts.md          # Node: contract analysis
    ├── litigation.md         # Node: court proceedings
    ├── ip-law.md             # Node: intellectual property
    ├── risk-management.md    # Node: risk management
    └── scripts/
        └── extract_clauses.py
```

---

## Writing Skill Graph Files

### `SKILL.md` — The Entry Point

The frontmatter `description` field becomes the primary node's summary in the Map of Content.

```markdown
---
name: legal-ai
description: Legal compliance and drafting assistant
version: 1.0.0
metadata:
  openclaw:
    emoji: ⚖️
    requires:
      bins: [python3]
tools:
  - name: extract_clauses
    description: Extracts key clauses from a contract PDF
    command: python3 scripts/extract_clauses.py
---

# Legal AI — Full Instructions

You are a legal assistant. You specialize in contract review,
risk analysis, and regulatory compliance...
```

### Content Nodes — The Graph

Each `.md` file becomes a node. Use the frontmatter `summary` field to describe what the node contains. If you omit it, AssistClaw auto-derives it from the first heading or first sentence.

```markdown
---
summary: Analyzing contracts for risk and ambiguity across jurisdictions
---

# Contract Analysis

When reviewing a contract, focus on:
1. Indemnification clauses
2. Limitation of liability
3. Dispute resolution...

[[ risk-management ]]  ← wikilinks to related nodes (future navigation)
```

**Summary priority**: `frontmatter summary` → `first heading` → `first sentence`

---

## How the Agent Sees It

When a skill is active, the agent's system prompt receives the **Map of Content** instead of the full text:

```xml
<skill_graph>
You have access to the following Skill Graphs. Each graph contains
nodes you can read using the 'read_skill_node' tool.

<skill name="legal-ai" description="Legal compliance and drafting assistant">
  <nodes>
    <node name="SKILL" summary="Legal compliance and drafting assistant" />
    <node name="contracts" summary="Analyzing contracts for risk and ambiguity across jurisdictions" />
    <node name="litigation" summary="Managing court proceedings and dispute resolution" />
    <node name="ip-law" summary="Intellectual property rights and patent filings" />
    <node name="risk-management" summary="Legal risk score across regulatory frameworks" />
  </nodes>
</skill>

</skill_graph>
```

That's it. **~690 chars** instead of ~18KB.

---

## How the Agent Reads Nodes

When the agent needs to know contract analysis procedures, it calls:

```json
{
  "tool": "read_skill_node",
  "input": {
    "skill_name": "legal-ai",
    "node_name": "contracts"
  }
}
```

The full body of `contracts.md` is returned only at that moment. Other nodes remain unloaded.

---

## Implementation Reference

| File | Role |
|---|---|
| [`internal/skills/types.go`](file:///Users/elrosshinzo/Projects/Personal/AssistClaw/internal/skills/types.go) | `Skill`, `Node`, `Registry` interface |
| [`internal/skills/loader.go`](file:///Users/elrosshinzo/Projects/Personal/AssistClaw/internal/skills/loader.go) | Node indexing, `BuildContext`, `parseNodeFile`, summary extraction |
| [`internal/skills/node_tool.go`](file:///Users/elrosshinzo/Projects/Personal/AssistClaw/internal/skills/node_tool.go) | `read_skill_node` agent tool |
| [`internal/skills/loader_test.go`](file:///Users/elrosshinzo/Projects/Personal/AssistClaw/internal/skills/loader_test.go) | 10 tests covering all behaviors + reduction benchmark |
| [`cmd/assistclaw/main.go`](file:///Users/elrosshinzo/Projects/Personal/AssistClaw/cmd/assistclaw/main.go) | Tool registration |

---

## Behavior Summary

| Feature | Behavior |
|---|---|
| Summary source | `frontmatter.summary` → first heading → first sentence |
| `SKILL.md` summary | Always uses `description` from skill frontmatter |
| Node key | Always derived from **filename** (not frontmatter `name`) |
| Body loading | Lazy — only when agent calls `read_skill_node` |
| Broken nodes | Silently skipped, skill still loads |
| Missing skill directory | No error, graceful no-op |

---

## Running Tests

```bash
go test ./internal/skills/... -v -run Test
```

Expected output:
```
--- PASS: TestLoadAll_IndexesNodes
--- PASS: TestNodeSummary_FrontmatterTakesPriority
--- PASS: TestNodeSummary_HeadingFallback
--- PASS: TestNodeSummary_FirstLineFallback
--- PASS: TestSKILLNodeGetsDescriptionAsSummary
--- PASS: TestReadSkillNode_ReturnsFullBody
--- PASS: TestReadSkillNode_MissingSkill_ReturnsFalse
--- PASS: TestReadSkillNode_MissingNode_ReturnsFalse
--- PASS: TestBuildContext_ContainsOnlyNodeSummaries
    Context reduction: 96.1%
--- PASS: TestContextReduction_MeasuresTokenReduction
PASS
```

---

## See Also

- [**Context Engineering**](context-engineering.md) — How AssistClaw's tool graph + skill graph system reduces token overhead by ~66% using BFS traversal, session inertia, and provider-capability detection.
- [Skills and Tools](skills-and-tools.md) — Autonomous tool generation
- [Providers and Routing](providers-and-routing.md) — Provider configuration
