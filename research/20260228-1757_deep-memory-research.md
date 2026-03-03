# Research: Tree-sitter, Spectral Clustering, and Obsidian Linking for hooks4claude Deep Memory

## Context

hooks4claude captures every Claude Code interaction (15 hook types, ~3700+ events across 34+ sessions) and indexes them in MeiliSearch. It already tracks: session_id, tool_name, file_path, prompts, errors, cwd, project_dir, permission_mode, timestamps, and flattened event data (`data_flat`).

But right now it's a **recording system** — it captures and lets you search. The question: can Tree-sitter AST parsing, Spectral Clustering, and Obsidian-style linking transform it into a **memory system** that understands structure, recognizes patterns, and connects knowledge?

## What is Context+

Context+ is an MCP server that combines all three techniques to give AI agents deep codebase understanding. Its architecture:

1. **Tree-sitter** parses code into ASTs, extracting symbols (functions, types, imports) across 14+ languages via WASM grammars
2. **Spectral Clustering** groups semantically related code into "feature clusters" using graph Laplacian eigenvectors on call/import graphs
3. **Obsidian-style `[[wikilinks]]`** create bidirectional feature hubs — navigable nodes where related symbols, files, and concepts converge

The key insight from Context+: **minimal context bloat** — read structural skeletons first, full files only when needed. This maps directly to hooks4claude's philosophy of summarizing events into searchable extracted fields rather than storing everything flat.

---

## Proposal 1: Tree-sitter — Symbol-Level Memory

### What it adds

Currently hooks4claude knows "Claude read `server.go` lines 50-80". With Tree-sitter, it would know "Claude read function `HandleHook` in package `server`". Memory moves from the **file:line** level to the **concept** level.

### Concrete features

**Event enrichment** — When a PreToolUse/PostToolUse event references a file_path, parse that file with tree-sitter and annotate the event with the symbol(s) at the relevant line range. New MeiliSearch fields: `symbol_name`, `symbol_type` (function/method/type/const), `symbol_package`.

**Codebase skeleton index** — Parse the project once per session (triggered by SessionStart), store function signatures, type definitions, and import graphs in a new `code-symbols` MeiliSearch index. Lightweight — just names and line ranges, not full source.

**Cross-session symbol tracking** — Query: "How many sessions touched the `AddEvent` function?" "Which prompts led to modifications of `HookMonitor`?" Currently impossible without grepping `data_flat`.

**Change impact annotation** — On Edit/Write events, parse the diff to identify which symbols were modified. Store as `symbols_modified: ["AddEvent", "CloseChannel"]`.

### Where it fits in the pipeline

```
PostToolUse event (tool_name=Read, file_path=server.go)
    ↓
tree-sitter parse server.go → AST
    ↓
match line range → symbol: HandleHook (function, package server)
    ↓
enrich Document with symbol_name, symbol_type, symbol_package
    ↓
MeiliSearch index (filterable by symbol_name)
```

### Implementation approach

- **Go library**: `github.com/smacker/go-tree-sitter` — mature Go bindings for tree-sitter with grammar support for Go, Python, JS/TS, Rust, etc.
- **Where**: New package `hooks-store/internal/symbols/` that runs tree-sitter on file paths from events
- **When**: In the `HookEventToDocument` transform pipeline, after extracting `file_path`
- **Constraint**: Needs access to the actual source files. In Docker, hooks-store would need a volume mount to the project directory, or the monitor could enrich events before forwarding

### Value for deep memory

**Before**: "Claude read a file 47 times across 12 sessions"
**After**: "Claude read the `HandleHook` function 23 times and `AuthMiddleware` 8 times. It modified `HandleHook` in 3 sessions, always after prompts mentioning 'hook routing'."

---

## Proposal 2: Spectral Clustering — Pattern Memory

### What it adds

Currently sessions are flat lists of events. Spectral clustering discovers **session archetypes** (debugging vs feature dev vs refactoring), **file feature groups** (files that belong together conceptually), and **prompt patterns** (recurring request types).

### Concrete features

**Session classification** — Build a feature vector per session: tool distribution (% Read, Edit, Bash, Glob...), error rate, unique files touched, prompt count, duration. Cluster into types: "debugging", "feature development", "exploration", "refactoring". Store as `session_type` on each event.

**File co-access graph** — Build adjacency matrix: files A and B get edge weight = number of sessions that accessed both. Spectral clustering on this graph reveals implicit feature groups — files that always appear together during work. These are the project's real architectural modules, discovered from usage, not from directory structure.

**Prompt archetypes** — Embed prompts (via simple TF-IDF or sentence embeddings), build similarity matrix, cluster. Discover recurring patterns: "fix bug in X", "add feature Y", "explain Z", "refactor W". Annotate each prompt with its archetype.

**Workflow fingerprints** — Sequence of tool types in a session (Read→Read→Edit→Bash→Read) forms a "workflow". Cluster similar workflows to find common patterns and anti-patterns (e.g., sessions with many failed Bash calls before success).

### Where it fits

Spectral clustering is a **batch analysis** — not real-time enrichment. It runs periodically (or on-demand) over accumulated data.

```
MeiliSearch (accumulated events)
    ↓
batch query: all sessions with their events
    ↓
build feature vectors / adjacency matrices
    ↓
spectral clustering (graph Laplacian → eigenvectors → k-means)
    ↓
write cluster labels back to MeiliSearch (session_type, file_group, prompt_archetype)
```

### Implementation approach

- **Language**: Python is the natural fit (scikit-learn `SpectralClustering`, numpy, scipy for sparse matrices). Could be a standalone script in `hooks-store/scripts/` or a separate service
- **Trigger**: CLI command (`hooks-store --analyze`) or cron job after N new sessions
- **Input**: MeiliSearch facet queries to build matrices
- **Output**: Writes cluster labels back to MeiliSearch documents via update API

### Value for deep memory

**Before**: "34 sessions, 3700 events"
**After**: "12 debugging sessions (avg 8 errors each), 15 feature-dev sessions (avg 22 files touched), 7 exploration sessions (90% Read tools). The files `monitor.go`, `server.go`, and `config.go` form a tight cluster — they're always modified together."

---

## Proposal 3: Obsidian-Style Linking — Associative Memory

### What it adds

Currently, cross-referencing requires ad-hoc MeiliSearch queries. Obsidian-style linking creates **first-class entities** (Files, Symbols, Sessions, Prompts, Projects) with **bidirectional links** between them. Every entity knows what references it and what it references.

### Concrete features

**Entity index** — New MeiliSearch index `hook-entities` with types: File, Symbol, Session, Prompt, Project. Each entity aggregates metadata from all events that reference it.

**Backlink tracking** — File entity for `server.go` automatically knows: 23 Read events, 3 Edit events, across sessions [S1, S5, S12], prompted by ["fix routing", "add auth", "refactor handlers"]. This is the "backlinks pane".

**Link density heatmap** — Entities with the most inbound links are "hot spots" — the parts of the codebase Claude returns to most. High link density suggests: the code is complex, poorly documented, or central to many features. Actionable: these files need better CLAUDE.md coverage.

**Concept hubs** — Auto-generated or user-defined groupings like "authentication", "TUI", "MeiliSearch integration". Each hub links to all related files, symbols, sessions, and prompts. These become navigable "chapters" of the deep memory.

**Session narrative** — Reconstruct a session as a linked story: "User asked → Claude searched → found files → read symbols → edited function → ran tests → committed." Each step links to the entities it touched.

### Where it fits

Entity creation happens during event ingestion (real-time) and enrichment happens in batch.

```
Real-time (per event):
  PostToolUse(Read, server.go) → upsert File entity for server.go, add backlink

Batch (periodic):
  For each File entity → count backlinks by type → compute link density
  For each Session entity → reconstruct tool sequence → generate narrative
  Auto-detect concept hubs via spectral clustering file groups
```

### Implementation approach

- **Storage**: New MeiliSearch index `hook-entities` with fields: `entity_type`, `entity_id`, `name`, `backlinks` (array of event IDs), `link_count`, `sessions` (array), `related_entities` (array)
- **Where**: Extend `hooks-store/internal/store/` with entity upsert logic in the ingest pipeline
- **Concept hubs**: Initially manual (user tags events/files); later auto-generated from spectral clustering output

### Value for deep memory

**Before**: "Search for 'server.go' in events and mentally piece together what happened"
**After**: Navigate to `server.go` entity → see 23 interactions across 8 sessions → follow backlink to session S5 → see it was a debugging session where `HandleHook` was modified → follow link to the prompt "fix the race condition" → see related files `monitor.go` and `config.go` were also modified

---

## How the Three Combine

```
                    Tree-sitter                    Spectral Clustering
                    (structural understanding)     (pattern recognition)
                         │                              │
                         ▼                              ▼
                    symbol_name                    session_type
                    symbol_type                    file_group
                    symbol_package                 prompt_archetype
                         │                              │
                         └──────────┬───────────────────┘
                                    │
                                    ▼
                         Obsidian-style Linking
                         (associative navigation)
                                    │
                                    ▼
                              Entity Graph
                    ┌─────────────────────────┐
                    │  File: server.go         │
                    │  Symbols: HandleHook,    │
                    │    AuthMiddleware         │
                    │  Sessions: [S1,S5,S12]   │
                    │  Feature: "HTTP routing" │
                    │  Link density: HIGH      │
                    │  Backlinks: 26 events    │
                    └─────────────────────────┘
```

The practical output — when Claude starts a new session on a project, deep memory provides:

> "This file (`server.go`) has been edited in 8 previous sessions. The function `HandleHook` was modified 3 times. The last modification (session S12) was prompted by 'fix the race condition in hook handling'. Sessions working on this area typically also touch `monitor.go` and `config.go` — they form feature cluster #2 ('core event pipeline'). Previous debugging sessions in this area had a 40% error rate with the Edit tool."

---

## Incremental Adoption Path

These can be implemented independently and compose when combined:

| Phase | What | Effort | Requires |
|-------|------|--------|----------|
| **A** | Entity index + backlinks (Obsidian linking, lightweight) | Medium | No new dependencies — pure MeiliSearch |
| **B** | File co-access clustering (simplest spectral clustering) | Medium | Python + scikit-learn, batch script |
| **C** | Tree-sitter event enrichment (symbol extraction) | High | go-tree-sitter + language grammars, file access from hooks-store |
| **D** | Session classification + prompt archetypes (full spectral) | Medium | Builds on B's infrastructure |
| **E** | Auto-generated concept hubs (combines all three) | Low | Combines outputs of B, C, D into entity index from A |

**Recommended start**: Phase A (entity index) — it adds immediate navigation value with zero new dependencies, and creates the data model that phases B-E plug into.

---

## Open Questions

1. **File access from hooks-store**: Tree-sitter needs the actual source files. Should the monitor enrich events before forwarding (has local access), or should hooks-store have a volume mount to the project?
2. **Embedding model for prompt clustering**: Simple TF-IDF, or use Ollama local embeddings (like Context+ does)? TF-IDF is dependency-free; Ollama is more semantic but adds a service.
3. **Entity graph storage**: Keep everything in MeiliSearch (simple, one database), or add a graph database like embedded DGraph/BadgerDB for native graph queries?
4. **Real-time vs batch**: Should entity creation happen per-event (real-time upsert) or in periodic batch runs? Real-time adds latency to ingest; batch means entities are slightly stale.
