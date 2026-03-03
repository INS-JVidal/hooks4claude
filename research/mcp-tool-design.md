# MCP Tool Design for Deep Memory

**Date:** 2026-02-28
**Prerequisite reading:** [compaction-carry-forward.md](compaction-carry-forward.md)

## Architecture: Hooks vs MCP Tools

Two separate mechanisms, not one:

```
Layer 1: Hooks (automatic, no tool call)
  PreCompact  → carry-forward summary (~600 tokens)
  SessionStart → project memory snapshot

Layer 2: MCP Tools (Claude calls on demand)
  Claude queries historical context that
  doesn't exist in files or CLAUDE.md
```

Hooks are push. MCP tools are pull. Don't conflate them.

## MCP Tool Reference

Grounded in MeiliSearch's current filterable fields: `hook_type`, `session_id`,
`tool_name`, `file_path`, `timestamp_unix`.

| # | Tool | What it answers | Query | ~Tokens |
|---|------|-----------------|-------|--------:|
| 1 | `search_memory` | Free-text search across all past events | MeiliSearch full-text on `data_flat` | 200-400 |
| 2 | `recall_session` | "What happened in session Y?" | `session_id = Y`, sort by timestamp → tool sequence + prompts | 200-400 |
| 3 | `recall_file` | "What happened with this file across sessions?" | `file_path = X`, group by session_id → edit/error counts | 150-300 |
| 4 | `recall_errors` | "What went wrong before with this file/tool?" | `hook_type = PostToolUseFailure AND file_path = X` | 100-200 |

### Why these four

- **`search_memory`** — MeiliSearch's core competency. General-purpose, covers 80% of cases.
- **`recall_session`** — The "continue where we left off" tool. Makes deep memory feel like memory.
- **`recall_file`** — Cross-session file history. Something Read literally cannot provide.
- **`recall_errors`** — Prevents repeating past mistakes. 51 PostToolUseFailure events in dataset.

### What's deliberately absent

- **`get_related_files`** — Requires precomputed co-access graph (spectral clustering). Add when Phase B data exists.
- **`get_project_overview`** — CLAUDE.md already provides project orientation. SessionStart hook can add a one-liner.
- **`get_session_context`** — This is Layer 1 (PreCompact hook), not an MCP tool.
- **Codebase structure tools** — Context+ handles that. Deep memory is about *what happened*, not *what the code looks like*.

## When Claude Would Call Each Tool

| Tool | Trigger | Natural? |
|------|---------|----------|
| `search_memory` | Needs historical context not in current files | Yes — general-purpose fallback |
| `recall_session` | User says "remember when..." or "continue from last time" | Yes — clear user trigger |
| `recall_file` | Before modifying a file not yet seen this session | Needs INSTRUCTIONS.md guidance |
| `recall_errors` | After hitting an error, before retrying | Hardest to trigger naturally |

## Minimum Viable Set

Ship two tools first:

1. **`search_memory`** — general-purpose, leverages MeiliSearch's strength
2. **`recall_session`** — the experience that makes memory *feel* like memory

The other two add precision but aren't essential for v1.

## Value Proposition

| Capability | Read tool | Layer 1 (hooks) | Layer 2 (MCP) |
|------------|-----------|-----------------|---------------|
| Current file content | Yes | No | No |
| Post-compaction recovery | 38+ calls | ~600 tokens, 0 calls | N/A |
| Past session history | No | No | Yes |
| Error patterns | No | Current session only | All sessions |
| Cross-session file history | No | No | Yes |

**Layer 1 reduces waste. Layer 2 creates new capability. Neither replaces Read.**

## Open Questions

1. **Session ID discovery** — How does the MCP server know the current session ID? Claude must pass it, or the server discovers it.
2. **Response format** — Structured JSON (flexible) or natural language summaries (cheaper for Claude to process)?
3. **Rate limiting** — Without guidance, Claude may over-query and bloat context worse than re-reads.
4. **Knowledge ranking** — Not all history is equal. Decisions and rationale > file contents. How to surface the most valuable knowledge first?
