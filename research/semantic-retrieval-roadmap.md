# Semantic Retrieval Roadmap — Multi-Layer Prompt Recall Architecture

**Date:** 2026-03-03
**Status:** Design phase
**Depends on:** hooks-mcp, hooks-store, MeiliSearch, Ollama (optional)
**Related:** [semantic-prompt-retrieval.md](semantic-prompt-retrieval.md), [ml-topic-keyword-analysis.md](ml-topic-keyword-analysis.md)

---

## 1. Goal

Enable queries like "find the algorithm I was working on two days ago" where the user can't remember exact terms — combining structured retrieval, semantic ranking, and honest reporting of what can and can't be answered.

---

## 2. Pipeline Overview

```
User prompt
     |
     v
+-- 1. DISCRIMINATION LAYER ---------------------+
|  What can I answer? What can't I?               |
|  → Answerable: time, project, prompts, tools    |
|  → Not answerable: Claude's responses, reasoning |
|  → Notify user of gaps                           |
+-------------------------------------------------+
     |
     v
+-- 2. SCOPE RESOLUTION -------------------------+
|  Parse the prompt into queryable dimensions:     |
|  → Time scope     (when?)                        |
|  → Project scope  (which project?)               |
|  → Prompt scope   (what was asked?)              |
|  → Tools scope    (what tools were used?)        |
|  → Files scope    (what files were touched?)     |
+-------------------------------------------------+
     |
     v
+-- 3. RETRIEVAL (MeiliSearch) -------------------+
|  Filter by resolved scopes                       |
|  Return candidate events/prompts                 |
+-------------------------------------------------+
     |
     v
+-- 4. SEMANTIC RANKING (embeddings) -------------+
|  Rank candidates by similarity to user's intent  |
|  Reorder results by relevance                    |
+-------------------------------------------------+
     |
     v
+-- 5. RESPONSE (structured data → Claude) -------+
|  Return ranked results for Claude to reason over |
+-------------------------------------------------+
```

---

## 3. Data Inventory — What We Have and Don't Have

| Dimension | Available | Data Source | Queryable |
|---|---|---|---|
| When did I work on X? | Yes | `timestamp_unix`, `session_id` | Filter |
| Which project? | Yes | Project path in event metadata | Filter |
| What did I ask Claude? | Yes | `data.prompt` from `UserPromptSubmit` | Search + embeddings |
| What tools were used? | Yes | `tool_name` from `PreToolUse`/`PostToolUse` | Filter + facet |
| What files were read/written? | Yes | Tool call data (`file_path` in tool input) | Search |
| What errors occurred? | Yes | `PostToolUseFailure` events | Filter |
| What did Claude answer? | **No** | Not captured by hooks | — |
| Why did Claude choose X? | **No** | Not captured by hooks | — |
| Full conversation flow? | **Partial** | Prompts only, no responses | — |

**Key constraint:** Hooks capture user actions and tool usage, but not Claude's reasoning or responses. The system must be transparent about this gap.

---

## 4. Layer 1 — Discrimination Layer

### Purpose

Before doing any retrieval, classify what parts of the request are answerable from stored data. Notify the user of gaps.

### Output Format

```json
{
  "answerable": ["time", "project", "prompts", "tools", "files"],
  "not_answerable": ["claude_responses"],
  "notice": "Claude's responses are not stored. I can show what you asked and what tools/files were used, but not what Claude replied."
}
```

### Implementation

- New MCP tool: `analyze-query` (lightweight, no MeiliSearch needed)
- Input: user's natural language query
- Logic: pattern match against known dimensions (time references, project mentions, tool names, "Claude said/answered/recommended" → not answerable)
- Returns: answerable/not-answerable classification + user-facing notice

### Complexity: Low (~80 LOC Go)

---

## 5. Layer 2 — Scope Resolution

### Purpose

Parse the user's query into concrete MeiliSearch filters.

### Scope-to-Filter Mapping

| Scope | Parsed From | MeiliSearch Filter |
|---|---|---|
| **Time** | "two days ago", "last week", "yesterday" | `timestamp_unix >= X AND timestamp_unix <= Y` |
| **Project** | "in hooks-store", "this project" | `project_path = /home/opos/PROJECTES/hooks-store` |
| **Prompts** | "about authentication", "ML algorithms" | Full-text search on `data.prompt` |
| **Tools** | "when I used Bash", "file edits" | `tool_name = Bash` or `tool_name = Edit` |
| **Files** | "changes to server.go" | Search tool input data for file paths |

### Implementation

- Reuses existing `dateparse` package for time scope
- Project scope: match against known project paths (from session metadata facets)
- Tool scope: match against known tool names (from `tool_name` facets)
- Prompt scope: pass through as MeiliSearch search query
- File scope: search within tool call input data

### Complexity: Medium (~150 LOC Go, mostly parsing + known-value matching)

---

## 6. Layer 3 — Retrieval

### Purpose

Combine resolved scopes into a single MeiliSearch query, return candidate events.

### Example Query

```bash
# "find what I was working on in hooks-store two days ago"
POST /indexes/hook-events/search
{
  "q": "",
  "filter": "timestamp_unix >= 1740787200 AND timestamp_unix <= 1740873599 AND project_path = hooks-store",
  "sort": ["timestamp_unix:desc"],
  "limit": 50
}
```

### Implementation

- Existing MeiliSearch client in `hooks-mcp/internal/meili/` handles this
- Combine filters from scope resolution with AND
- Not every query uses every scope — only apply resolved ones

### Complexity: Low (mostly exists already)

---

## 7. Layer 4 — Semantic Ranking

### Purpose

After retrieval, rerank results by embedding similarity to the user's intent. Doesn't filter — **reorders**.

### Flow

```
50 retrieved events
     |
     v
Embed the user's current query → query_vector
Compare against each retrieved prompt's stored embedding
Sort by cosine similarity
     |
     v
Top 10 most semantically relevant results
```

### Prerequisites

- Embeddings stored at ingest time (hooks-store addition)
- Ollama running locally with `nomic-embed-text` (CPU is fine, ~30ms per prompt)
- MeiliSearch vector search configured (supported since v1.3)

### Enriched Document Schema

```json
{
  "id": "evt_abc123",
  "hook_type": "UserPromptSubmit",
  "prompt": "refactor the authentication handler to use middleware",
  "keywords": ["refactor", "authentication handler", "middleware"],
  "_vectors": {
    "prompt_embedding": [0.12, -0.45, 0.78, ...]
  },
  "session_id": "ses_xyz",
  "timestamp_unix": 1740787200
}
```

### MeiliSearch Vector Search Config

```bash
curl -X PATCH 'http://localhost:7700/indexes/hook-prompts/settings' \
  -d '{
    "filterableAttributes": ["keywords", "hook_type", "session_id", "timestamp_unix", "project_path"],
    "faceting": {"sortFacetValuesBy": {"keywords": "count"}},
    "embedders": {
      "prompt_embedding": {
        "source": "userProvided",
        "dimensions": 768
      }
    }
  }'
```

### Complexity: Medium (~50 LOC in hooks-store for Ollama HTTP call at ingest, ~100 LOC in hooks-mcp for vector reranking)

---

## 8. Layer 5 — Response Assembly

### Purpose

Return structured results for Claude to reason over.

### Output Format

```json
{
  "query_analysis": {
    "answerable": ["time", "project", "prompts", "tools", "files"],
    "not_answerable": ["claude_responses"],
    "notice": "Claude's responses are not stored."
  },
  "scopes_resolved": {
    "time": "2026-03-01",
    "project": "all"
  },
  "results": [
    {
      "timestamp": "2026-03-01T14:23:00Z",
      "prompt": "explore LDA for document clustering",
      "tools_used": ["WebSearch", "Read"],
      "files_touched": ["research/ml-topic-keyword-analysis.md"],
      "similarity_score": 0.91
    }
  ]
}
```

Claude receives this and produces a natural language answer for the user.

### Complexity: Low (~80 LOC Go, formatting + aggregation)

---

## 9. Collaboration Diagram

```
                    Discrimination    Scope         Retrieval      Semantic
User Query          Layer             Resolution    (MeiliSearch)  Ranking
─────────────────   ────────────────  ────────────  ─────────────  ──────────

"what algorithm     Can answer:       Time: Mar 1   50 events      Rerank by
 was I exploring    prompts, tools,   Project: all  from that day  similarity
 two days ago?"     files, time                                    to "algorithm"
                    Can't: Claude's                                → top 5
                    responses
                         |                 |              |             |
                         v                 v              v             v
                    Notify user       Build filters   Execute query  Sort results
                    of gaps           for MeiliSearch                → return to
                                                                      Claude
```

---

## 10. Architectural Decision: Local ML vs Claude

| Responsibility | Who | Why |
|---|---|---|
| **Storage & retrieval** | MeiliSearch + hooks-mcp | Fast, local, structured, always available |
| **Embedding (vectorization)** | Ollama on CPU (local) | Small model, ~30ms/prompt, no GPU needed |
| **Reasoning & synthesis** | Claude (active session) | Already running, already has context, already paid for |
| **Discrimination** | hooks-mcp tool (local) | Deterministic, pattern-based, no ML needed |
| **Scope parsing** | hooks-mcp tool (local) | Reuses dateparse, deterministic matching |

### What NOT to Run Locally (no GPU)

- Summarization → too slow on CPU
- Question answering → too slow on CPU
- Intent classification via LLM → too slow on CPU
- Any reasoning → Claude is already in the loop, let it do this

### Embedding Models on CPU — Fast Enough

| | Embedding model | LLM (reasoning) |
|---|---|---|
| Task | Text → vector (one pass) | Text → text (autoregressive) |
| Speed on CPU | 10-50ms per prompt | Seconds per token |
| Model size | 80-400 MB | 4-70 GB |
| GPU needed? | No | Practically yes |

---

## 11. Implementation Plan

### Phase 1 — RAKE Keywords at Ingest (hooks-store)

| Item | Detail |
|---|---|
| What | Add RAKE keyphrase extraction at ingest time |
| Where | New package `hooks-store/internal/keywords/` |
| Effort | ~100 LOC Go |
| Dependency | None (stop word list embedded as `map[string]bool`) |
| Output | `keywords` array field on each document |
| MeiliSearch | Add `keywords` to filterable + facetable attributes |

### Phase 2 — Ollama Embeddings at Ingest (hooks-store)

| Item | Detail |
|---|---|
| What | Call Ollama HTTP API to embed each prompt at ingest |
| Where | New package `hooks-store/internal/embed/` or added to ingest pipeline |
| Effort | ~50 LOC Go |
| Dependency | Ollama running locally (`ollama pull nomic-embed-text`) |
| Output | `_vectors.prompt_embedding` field on each document |
| MeiliSearch | Configure `embedders` in index settings |

### Phase 3 — Discrimination + Scope Resolution (hooks-mcp)

| Item | Detail |
|---|---|
| What | New MCP tool `analyze-query` — classifies answerable dimensions, resolves scopes |
| Where | `hooks-mcp/internal/tools/` |
| Effort | ~200 LOC Go |
| Dependency | Existing `dateparse` package, MeiliSearch facets for project/tool lists |
| Output | JSON with answerable/not-answerable + resolved filters |

### Phase 4 — Unified Retrieval + Ranking Tool (hooks-mcp)

| Item | Detail |
|---|---|
| What | New MCP tool `recall-prompts` — takes resolved scopes, retrieves + ranks |
| Where | `hooks-mcp/internal/tools/` |
| Effort | ~150 LOC Go |
| Dependency | Phases 1-3 |
| Output | Ranked results with prompts, tools, files, similarity scores |

### Phase Summary

| Phase | Component | LOC | Dependency |
|---|---|---|---|
| 1 | RAKE keywords (hooks-store) | ~100 | None |
| 2 | Ollama embeddings (hooks-store) | ~50 | Ollama |
| 3 | Discrimination + scope (hooks-mcp) | ~200 | dateparse, MeiliSearch |
| 4 | Unified retrieval tool (hooks-mcp) | ~150 | Phases 1-3 |
| **Total** | | **~500 LOC** | |

---

## 12. What's Missing from Current hooks-mcp (Gap Analysis)

| Have Today | Need to Add |
|---|---|
| `query-prompts` (text search) | Discrimination layer (classify what's answerable) |
| `query-sessions` (session listing) | Scope resolver (parse time/project/tools from natural language) |
| `search-events` (event search) | Embedding storage + vector reranking |
| `tool-usage` (tool stats) | File-touch tracking (extract file paths from tool data) |
| `dateparse` package | Unified retrieval tool that combines all scopes |
| `cost-analysis` (token costs) | `analyze-query` MCP tool (discrimination layer entry point) |
| `error-analysis` (failures) | `recall-prompts` MCP tool (scoped retrieval + ranking) |
