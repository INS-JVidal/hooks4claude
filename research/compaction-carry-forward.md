# Compaction Carry-Forward Strategy

**Date:** 2026-02-28
**Prerequisite reading:** [information-seeking-analysis.md](information-seeking-analysis.md)

## Problem

Context compaction is lossy. After compaction, Claude loses exact file contents,
decision rationale, error context, and the mental model built through cumulative
reads. The data shows this directly: a 38-call info burst immediately after
compaction in session `b5fce71a`, and a 73% re-read rate overall.

The PreCompact hook fires before every compaction (5 observed events, all with
`trigger: "auto"`). This is the intervention point.

## What compaction preserves vs loses

### Preserved (re-injected automatically)

- CLAUDE.md instructions (system prompt)
- Recent messages (tail of conversation)
- A lossy summary of earlier exchanges

### Lost

- Exact file contents from Read calls
- Specific code snippets and line numbers
- Grep/search results and their context
- Bash outputs (test results, build errors, command output)
- The sequence of decisions and their rationale
- Error messages and how they were resolved
- The "mental model" built through cumulative reads
- Symbol relationships discovered through code exploration

## What to carry forward

Ranked by re-read elimination value (from [information-seeking-analysis.md](information-seeking-analysis.md)):

### 1. Active file manifest

Files read/edited this session, with structural summary:

```
server.go: HandleHook (func, L109-117), AuthMiddleware (func, L154-160)
  - Edited at 20:15: added case-insensitive routing via hookLookup map
monitor.go: AddEvent (method, L45-67), CloseChannel (method, L89-95)
  - Read 4x, not modified
config.go: ReadConfig (func, L12-45), ReadSinkConfig (func, L78-95)
  - Read 2x, not modified
```

**Why:** Eliminates the 73% re-reads. Claude knows what's in the files
without reading them again. The most re-read file (`hook-client/main.go`,
81 re-reads) would benefit most.

### 2. Modification log

What changed, where, and why:

```
- server.go:109 HandleHook: added hookLookup map for case-insensitive routing
- config.go:45 ReadConfig: changed default to fail-open (missing key = enabled)
- monitor_test.go: added TestAddEvent_concurrent with sync.WaitGroup
```

**Why:** The compacted summary says "modified X" but loses the exact diff
and the rationale. Without this, Claude may re-read the file to understand
what it already did.

### 3. Error context

Failed attempts and their resolutions:

```
- Bash `go test ./...`: race condition in TestAddEvent (fixed with sync.Once)
- Edit server.go L120: wrong line range caused unique match failure, re-read to fix
- Bash `curl localhost:8080/hook/test`: 404 — hookType validation is case-sensitive
```

**Why:** Without error history, Claude may repeat the same failed approaches.
The data shows 51 PostToolUseFailure events — these are expensive lessons
that should not be re-learned.

### 4. Pending intent

What was planned but not yet completed:

```
- TODO: update CLAUDE.md after API changes to server package
- TODO: run integration tests with hooks-store
- In progress: refactoring sink package (HTTPSink → generic EventSink interface)
```

**Why:** Compaction loses the task context. Claude may start fresh instead of
continuing where it left off.

### 5. Symbol dependency map

Structural relationships discovered through code exploration:

```
HandleHook → monitor.AddEvent → sink.Send (forwarding chain)
AuthMiddleware wraps mux via http.Handler (middleware chain)
HookConfig.IsEnabled reads [hooks] section (fail-open on missing)
```

**Why:** The grep pattern analysis shows Claude repeatedly searching for type
definitions, interface implementations, and import relationships. These are
the structural queries (`type HTTPSink struct`, `EventSink`, `import.*sink`)
that Tree-sitter would make instant — but even without Tree-sitter, carrying
the discovered relationships forward prevents re-discovery.

## How to build the carry-forward payload

When PreCompact fires, hook-client queries MeiliSearch for the current session:

```
1. Filter: session_id = current AND hook_type = PreToolUse
   → Extract unique file_paths (working set)

2. Filter: session_id = current AND tool_name IN (Edit, Write)
   → Extract modification log (what changed)

3. Filter: session_id = current AND hook_type = PostToolUseFailure
   → Extract error context (what went wrong)

4. Filter: session_id = current AND hook_type = UserPromptSubmit
   → Extract user prompts (intent and pending work)

5. Compress into structured summary (~500-1000 tokens)

6. Return via PreCompact hook stdout
   → Injected into compacted context
```

### Token budget

The carry-forward must be compact. Estimated sizes:

| Component | Est. tokens |
|-----------|------------:|
| File manifest (10 files) | 200 |
| Modification log (5 edits) | 150 |
| Error context (3 errors) | 100 |
| Pending intent | 50 |
| Symbol map (10 relationships) | 100 |
| **Total** | **~600** |

This is ~0.3% of a 200K context window. Negligible cost, high value.

## What carry-forward CANNOT cover

Some information is too large or too contextual to inject:

- **Full file contents** — would blow up the carry-forward payload
- **Complete bash output logs** — test outputs can be thousands of lines
- **Multi-step reasoning chains** — the "why" behind a sequence of decisions
- **Previous sessions' knowledge** — carry-forward only covers the current session

These require **Layer 2: on-demand retrieval** (see below).

## Two-layer architecture

```
┌─────────────────────────────────────────────┐
│ Layer 1: PreCompact carry-forward           │
│                                             │
│ Trigger: PreCompact hook fires              │
│ Scope: current session only                 │
│ Latency: immediate (injected into context)  │
│ Solves: post-compaction info burst          │
│         (the 38-call re-orientation)        │
│                                             │
│ "Here's what you were working on            │
│  10 seconds ago"                            │
└─────────────────────────────────────────────┘
                    +
┌─────────────────────────────────────────────┐
│ Layer 2: On-demand retrieval (MCP / hooks)  │
│                                             │
│ Trigger: Claude queries deep memory         │
│ Scope: all sessions, all projects           │
│ Latency: per-query (MCP tool call)          │
│ Solves: cross-session knowledge loss        │
│         (the 73% re-reads across sessions)  │
│                                             │
│ "Here's what happened last week             │
│  with this file"                            │
└─────────────────────────────────────────────┘
```

Layer 1 is implementable today with the existing PreCompact hook and
MeiliSearch data. Layer 2 requires an MCP server or SessionStart injection.

## Next steps

1. Validate that PreCompact hook stdout is actually injected into the
   compacted context (test with a simple hook that returns static text)
2. Build a prototype carry-forward query in hook-client
3. Measure: does the info burst disappear after carry-forward injection?
4. Design the Layer 2 retrieval API (see [deep-memory-research.md](deep-memory-research.md))
