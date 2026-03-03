# Information-Seeking Behavior Analysis

**Date:** 2026-02-28
**Dataset:** 3755 events, 27 sessions with tool calls, MeiliSearch index `hook-events`

## Purpose

Analyze when Claude Code needs information and what it requests, to inform the design of a retrieval system (deep memory) that fights context rot.

---

## Tool Usage Distribution

| Tool            | Count | Pct   | Category          |
|-----------------|------:|------:|-------------------|
| Read            | 1459  | 44.4% | Info seeking       |
| Bash            | 1086  | 33.1% | Action             |
| Edit            |  190  |  5.8% | Action             |
| Glob            |  188  |  5.7% | Info seeking       |
| Grep            |  132  |  4.0% | Info seeking       |
| Write           |   60  |  1.8% | Action             |
| Task            |   44  |  1.3% | Info seeking       |
| WebSearch       |   16  |  0.5% | Info seeking       |
| WebFetch        |    4  |  0.1% | Info seeking       |
| Management      |  105  |  3.2% | Task/Plan/Ask      |

## Time Budget

```
Information seeking (Read/Glob/Grep/Web/Task):  1843  (56%)
Action taking (Edit/Write/Bash):                1336  (41%)
Management (Tasks/Plan/Ask):                     105  (3%)

Ratio: Claude spends 1.4x more time seeking info than acting
```

---

## Re-read Analysis

**73% of file reads are redundant re-reads** of files already read earlier in the same session.

```
Total reads with file_path: 1000 (sampled)
Unique file+session pairs:   271
Re-reads (redundant):        729 (73%)
```

### Most Re-read Files

| Re-reads | File |
|---------:|------|
| 81 | `claude-hooks-monitor/cmd/hook-client/main.go` |
| 68 | `claude-hooks-monitor/internal/monitor/monitor.go` |
| 60 | `claude-hooks-monitor/internal/server/server.go` |
| 44 | `claude-hooks-monitor/internal/filecache/filecache.go` |
| 37 | `claude-hooks-monitor/internal/config/config.go` |
| 34 | `claude-hooks-monitor/internal/hookevt/hookevt.go` |
| 28 | `claude-hooks-monitor/cmd/monitor/main.go` |
| 19 | `claude-hooks-monitor/internal/monitor/monitor_test.go` |
| 16 | `hooks-store/internal/store/store.go` |
| 16 | `.claude/plans/partitioned-jumping-kahn.md` |
| 14 | `hooks-store/internal/ingest/server.go` |
| 14 | `hooks-store/internal/store/meili.go` |
| 13 | `claude-hooks-monitor/internal/sink/sink.go` |
| 12 | `hooks-store/internal/hookevt/hookevt.go` |
| 12 | `claude-hooks-monitor/cmd/hook-client/client_test.go` |

**Interpretation:** These are the project's core files. Claude reads them repeatedly because context compaction loses the file content. A retrieval system that serves "here's what you already know about this file" would eliminate most of these.

---

## What Claude Searches For (Grep Patterns)

53 unique grep patterns. Dominant categories:

| Category | Examples | Count |
|----------|----------|------:|
| Package/type defs | `package sink`, `type HTTPSink struct` | 11 |
| Test discovery | `func Test`, `^func Test` | 6 |
| Interface search | `type.*Sink\|interface.*Sink`, `EventSink` | 4 |
| Concurrency | `sync\.(Mutex\|RWMutex)`, `atomic\.`, `make\(chan` | 5 |
| Data schema | `tool_input\|session_id`, `tool_name\|tool_use_id` | 4 |
| Config/infra | `MEILI_URL\|meili-url\|7700`, `limit\|maxHits` | 3 |
| Import tracing | `import.*sink`, `claude-hooks-monitor/internal/sink` | 3 |

**Interpretation:** These are structural queries — Claude is looking for type definitions, function signatures, interface implementations, and import relationships. Exactly the kind of information Tree-sitter AST parsing would make instantly queryable without reading full files.

---

## Session Workflow Pattern

Analysis of the busiest session (226 tool calls) reveals three distinct phases:

### Phase 1: Upfront orientation (steps 1-19)
Heavy reading burst (9 reads), then task creation and sequential edits. Claude builds initial context.

### Phase 2: Short read-edit cycles (steps 20-33)
1 read → 1-2 edits, repeating. Claude has context and works incrementally.

### Phase 3: Post-compaction recovery (step 34)
**Massive 38-call info burst:** 29 reads, 4 Task agents, 4 greps, 1 glob. This is almost certainly a post-compaction re-orientation — Claude lost context and is rebuilding it from scratch.

### Phase 4: Interleaved info-action (steps 35-75)
Constant alternation: 1-5 reads then 1-3 actions. Claude has partial context and re-reads files before each edit.

```
 1. [INFO] ( 9 calls) 9xRead                          ← orientation
 2. [ MGT] (10 calls) 9xTaskCreate, 1xTaskUpdate
 3-19.     ... sequential edits with task updates ...
20-33.     ... short read→edit cycles ...
34. [INFO] (38 calls) 29xRead, 4xTask, 4xGrep, 1xGlob ← post-compaction burst
35-75.     ... interleaved 1-5 reads → 1-3 actions ...
```

---

## Read Intensity Per Session

| Reads | Session (truncated) |
|------:|---------------------|
| 150 | `b5fce71a-65b0-47...` |
| 120 | `e2a5347d-1b47-4b...` |
| 112 | `cc18f85d-0e4d-45...` |
| 112 | `f1f87bfb-0fe8-4d...` |
| 108 | `70b4739e-e39e-42...` |
| 104 | `0721cd26-d748-42...` |
| 100 | `4c7a1c85-9fef-45...` |

Average: 54 reads per session across 27 sessions.

---

## Implications for Deep Memory Retrieval

### The core problem quantified
- **73% of reads are wasted** on files Claude already read in the session
- **56% of all tool calls** are information seeking
- Post-compaction info bursts (38 calls) show the exact moment Claude loses context

### What a retrieval system should provide

1. **File content cache** — "You read this file 3 calls ago, here's what was in it" (eliminates 73% re-reads)
2. **Symbol lookup** — "Here's the definition of `HandleHook`" without reading the whole file (serves the structural grep patterns)
3. **Session history** — "Last time you worked on this file cluster, you did X" (prevents repeating past mistakes)
4. **Post-compaction injection** — Detect compaction events (PreCompact hook) and inject a summary of what was lost

### What enrichment enables for retrieval precision

Without enrichment, retrieval query for "HandleHook" searches `data_flat` → noisy results.
With `symbol_name` field, retrieval query filters `symbol_name = HandleHook` → exact results, minimal tokens.

**The 73% re-read rate is the single strongest argument for deep memory.** Every re-read is a token cost that a retrieval system would eliminate.
