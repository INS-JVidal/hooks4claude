# Progressive Disclosure for Deep Memory

**Date:** 2026-02-28
**Prerequisite reading:** [compaction-carry-forward.md](compaction-carry-forward.md), [mcp-tool-design.md](mcp-tool-design.md)

## Session Lifecycle Discovery

Investigation of session `677e4783` reveals the actual lifecycle:

```
SessionStart → work → Stop → SessionEnd
    ↓ (3 seconds, same session_id)
SessionStart → work → PreCompact → SessionStart → work → Stop → SessionEnd
    ↓ (3 seconds, same session_id)
SessionStart → work → Stop → SessionEnd
```

Key findings:
- **SessionStart fires after PreCompact.** Compaction creates a fresh context within the same session.
- **Same session_id survives SessionEnd → SessionStart.** The 3-second gap is CLI restart (conversation continue/resume). Not a new conversation.
- **Each "sub-session" is short** — 30-40 seconds of reads, then stop. Short question-answer cycles.
- **SessionStart means "context initialized"**, not "new conversation." It fires on: fresh start, resume, post-compaction, subagent spawn.

## The Core Question

After compaction + SessionStart, Claude gets a fresh context with CLAUDE.md and a lossy summary. Then it does a **38-call info burst** — 29 full file reads, 4 subagents, 4 greps, 1 glob.

Why does it read 29 full files when it only needs to know what it was working on?

**Because it has no choice.** Without a summary of what was happening, Claude's only option is to re-read everything and reconstruct context from raw source files. It's doing Level 2 retrieval (full content) when Level 0 (summary) would suffice.

## Progressive Disclosure: Context+ Applied to Memory

Context+'s core philosophy: **never load full files when a summary will do.** It applies progressive disclosure to codebase structure:

| Level | Context+ (space) | Tokens | Deep Memory (time) | Tokens |
|-------|-------------------|-------:|--------------------|-------:|
| 0 | Context tree: file list + headers | ~200 | Session summary: what was being done, by whom, why | ~100 |
| 1 | File skeleton: function signatures, no bodies | ~500 | Active manifest: files, modifications, errors, pending intent | ~500 |
| 2 | Full file: complete source code | ~2000 | Full event history: every tool call with inputs/outputs | ~5000 |

Context+ does progressive disclosure on **space** — start with the tree, zoom into skeletons, read full files only when editing.

Deep memory should do progressive disclosure on **time** — start with what happened, zoom into specific changes, retrieve full event data only when debugging.

## Why This Beats Re-Reading

The 38-call post-compaction burst is Claude jumping straight to Level 2:

```
Without memory:
  compaction → fresh context → ??? → read 29 files (Level 2)
  Cost: 29 tool calls, ~58,000 tokens of file content, ~30 seconds

With progressive disclosure:
  compaction → PreCompact injects Level 0-1 → Claude knows what it was doing
  Cost: ~600 tokens injected, 0 tool calls, immediate

  Claude reads a full file ONLY when it needs to edit it.
```

The injected summary doesn't replace file reads — it **prevents undirected reads**. Claude still reads `server.go` when it needs to edit line 109. But it doesn't read 28 other files just to figure out what it was doing.

## The Three Levels in Practice

### Level 0: Session summary (~100 tokens, injected via PreCompact)

```
Working on: case-insensitive hook routing in server.go
User intent: fix routing so hookType matching isn't case-sensitive
Status: HandleHook modified, tests passing, integration test pending
```

This tells Claude *what* it was doing. Enough to continue without any reads.

### Level 1: Active manifest (~500 tokens, injected via PreCompact)

```
Files:
  server.go: HandleHook (L109-117) — added hookLookup map
  monitor.go: AddEvent (L45-67) — read 4x, not modified
  config.go: ReadConfig (L12-45) — changed fail-open default

Errors:
  go test ./...: race condition in TestAddEvent (fixed with sync.Once)
  Edit server.go L120: wrong line range, re-read to fix

Pending:
  TODO: run integration tests with hooks-store
```

This tells Claude *what changed and what went wrong*. Eliminates re-reading files just to see what it already did.

### Level 2: Full event history (on-demand via MCP `search_memory`)

Only needed when Claude needs specific details: "What exact error message did the race condition produce?" or "What was the user's exact prompt about routing?"

This is where the MCP tools live — they're Level 2 retrieval for rare cases.

## What This Means for Implementation

| Level | Mechanism | When | Trigger |
|-------|-----------|------|---------|
| 0 | PreCompact hook stdout | Every compaction | Automatic |
| 1 | PreCompact hook stdout | Every compaction | Automatic (bundled with Level 0) |
| 2 | MCP tool `search_memory` | Rare, specific queries | Claude decides (or user asks) |

Levels 0 and 1 together fit in ~600 tokens. They're injected automatically, every time. This is the high-value, high-reliability intervention.

Level 2 is the safety net — available when summaries aren't enough, but rarely needed.

## Connection to Context+ Methods

| Context+ technique | How it applies to deep memory |
|--------------------|-------------------------------|
| **Tree-sitter skeletons** | Session skeletons: key events only, not full history. Function names instead of line ranges. |
| **Feature clusters** (spectral) | Activity clusters: group events by what was being worked on, not chronological order. |
| **Progressive disclosure** | Time-based disclosure: summary → manifest → full history. |
| **Obsidian hubs** | Entity hubs: file entities aggregate all interactions across sessions. Navigate by file, not by timestamp. |

The philosophical alignment: Context+ says "don't load the whole codebase to find one function." Deep memory says "don't replay the whole session to know what you were doing."

Both are fighting the same enemy: **context waste from undirected loading.**
