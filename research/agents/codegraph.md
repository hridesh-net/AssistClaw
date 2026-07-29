# codegraph (github.com/colbymchenry/codegraph)

## What it is

codegraph is a local-first code-intelligence tool (library + CLI + MCP server) that pre-indexes a codebase into a symbol-level knowledge graph so AI coding agents (Claude Code, Cursor, Codex) get "surgical context" — relevant symbols' verbatim source, call paths, and blast radius — in one tool call instead of grep/read loops. Claimed benchmarks: 89% fewer tool calls, 69% fewer tokens, file reads drop to zero. Its stated optimization target is wall-clock latency plus tool-call count, not token cost.

Stack: TypeScript (Node) for orchestration/CLI/MCP; a Rust kernel (codegraph-kernel) using tree-sitter grammars for roughly 20-30 languages; storage in SQLite (node:sqlite, WAL mode, FTS5). 100% local, no external APIs.

## Data model

Node kinds (22): file, module, class, struct, interface, trait, protocol, function, method, property, field, variable, constant, enum, enum_member, type_alias, namespace, parameter, import, export, route, component. "route" and "component" are framework-aware kinds.

Edge kinds (12): contains, calls, imports, exports, extends, implements, references, type_of, returns, instantiates, overrides, decorates.

Node metadata: id (hash of filePath + qualifiedName), kind, name, qualified_name, file_path, language, line/column ranges, docstring, signature, visibility, is_exported, is_async, is_static, is_abstract, decorators, type_parameters, return_type, updated_at. Key design choice: no source code stored in the graph — only structure, location, and cross-references; actual source is read from disk on demand.

Edge metadata: source, target, kind, metadata JSON, line, col, provenance in {tree-sitter, scip, heuristic}. Heuristic edges carry metadata naming the heuristic that synthesized them (e.g. swift-objc-bridge). Uniqueness key: (source, target, kind, line, col).

Supporting tables: files (content_hash, language, size, modified_at, indexed_at, node_count, errors — drives incremental sync), unresolved_refs (deferred link resolution ledger with candidates and pending/failed status), nodes_fts (FTS5 over name/qualified_name/docstring/signature), name_segment_vocab (camelCase/snake_case sub-word search), project_metadata. Output lives in project-root/.codegraph/codegraph.db.

## Ingestion pipeline

files → ExtractionOrchestrator (tree-sitter) → DB (nodes/edges/files) → ReferenceResolver (imports, name-matching, framework patterns) → GraphQueryManager / GraphTraverser (callers, callees, impact) → ContextBuilder (markdown/JSON for AI consumption).

- Discovery walks the repo respecting .gitignore, excluding vendor/build dirs and files over 1MB.
- Parsing is tree-sitter, parallel, per-file; syntax errors degrade per-file, not globally.
- Granularity is symbol level, with file and module nodes as containers ("contains" edges give hierarchy file→class→method).
- Linking is a separate second phase: extraction emits nodes plus unresolved references; ReferenceResolver then maps calls to definitions (import resolution including tsconfig path aliases, then name-matching), completes inheritance chains, and applies framework pattern handlers (17 frameworks: Express, Django, Rails, FastAPI, React Router...) to link route nodes to handler functions, plus cross-language bridge heuristics (Swift↔ObjC, React Native TurboModules/Fabric, Expo).
- Freshness: OS file watchers (FSEvents/inotify) with 2s debounce; incremental sync keyed on content_hash/mtime/size; staleness banners while sync is pending.

## Query and agent use

MCP server exposes 8 read-only tools with a funnel design pushing agents to one primary tool:

- codegraph_explore — primary; NL or symbol query returns verbatim source of relevant symbols grouped by file, call paths (including dynamic-dispatch hops), and a blast-radius summary. Dynamically budgeted output 13K-24K chars.
- codegraph_search (name lookup, locations only), codegraph_callers / codegraph_callees (one-hop call edges), codegraph_impact (reverse-dependency traversal, default depth 2), codegraph_node (file with line numbers or one symbol's definition plus caller/callee trails), codegraph_files (indexed file tree), codegraph_status (index health).

All outputs are hard-capped (15K/24K chars) to protect the agent's context. The MCP initialize response carries server instructions steering agents to prefer explore over grep/read.

ContextBuilder formats output for LLMs: entry points → related symbols grouped by container → verbatim excerpts with location headers, with a JSON twin of the same structure and an ASCII call-tree option. Generated files (protobufs, mocks) rank last; related items are capped (~10) per section.

## Core concepts worth replicating

1. Typed entities with a small closed vocabulary of node kinds; typed relations with their own closed vocabulary.
2. Stable deterministic node IDs — hash(container_path + qualified_name) — enabling idempotent re-ingestion and cross-file linking.
3. The graph stores structure and pointers, not content: nodes carry location, signature, docstring; body fetched on demand. Keeps the graph small and never stale-in-content.
4. Two-phase build: extract nodes + raw references per source independently, then resolve references into edges. Keep an unresolved_refs ledger instead of dropping unlinkable references.
5. Provenance on edges: parsed/exact vs heuristic/inferred, with metadata naming the synthesizing heuristic. Critical when mixing hard facts with guesses.
6. Containment hierarchy as edges so one edge model covers both structure and semantics.
7. Three query primitives cover almost everything: name/full-text search, one-hop neighbors, bounded-depth traversal.
8. One "explore" entry point composing search + traversal + content fetch into a single capped response, with agent instructions funneling toward it.
9. Per-source freshness records for incremental re-index, with staleness surfaced to the consumer.
10. Output formatted for LLMs with an explicit budget.

## Graph facts

- codegraph|is_a|code knowledge-graph indexer for AI agents
- codegraph|authored_by|colbymchenry
- codegraph|implemented_in|TypeScript
- codegraph|has_component|codegraph-kernel (Rust)
- codegraph-kernel|uses|tree-sitter
- codegraph|stores_graph_in|SQLite
- codegraph|uses|FTS5 full-text index
- codegraph|exposes_interface|MCP server
- codegraph|exposes_interface|CLI
- codegraph|primary_tool|codegraph_explore
- codegraph_explore|returns|verbatim symbol source + call paths + blast radius
- codegraph|node_kinds|22
- codegraph|edge_kinds|12
- codegraph|node_id_derived_from|hash(file_path + qualified_name)
- codegraph|edges_carry|provenance (tree-sitter, scip, heuristic)
- codegraph|nodes_store|locations and signatures, not source code
- ExtractionOrchestrator|feeds|ReferenceResolver
- ReferenceResolver|resolves|unresolved_refs into typed edges
- ReferenceResolver|links|framework routes to handler functions
- codegraph|sync_via|file watchers + content_hash reconciliation
- ContextBuilder|emits|markdown, JSON, ASCII-tree formats
- codegraph_impact|computes|reverse-dependency traversal depth 2
