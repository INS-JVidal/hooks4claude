# Compaction Function Carry-Forward (Tree-sitter + Hook Traces)

**Status: HIGH POTENTIAL — investigate further**

## Core Idea

Use tree-sitter to extract the actual function bodies that were touched during a session, and carry them forward across context compaction — not just names, but the real code.

## The Problem

1. Claude works on `HandleHook` in `server.go` for 30 minutes
2. PreCompact fires — Claude's built-in compaction summary says *"I was modifying HandleHook to fix a race condition"*
3. Session resumes — Claude knows the *name* but lost the *code*
4. Claude re-reads `server.go` (and 37 other files) — the post-compaction burst (73% redundant re-reads from data analysis)

## The Proposal

1. PreCompact fires
2. Query hook traces for this session: all Read/Edit events → extract `file_path` + line ranges
3. Tree-sitter parses those files → identifies the actual functions at those line ranges
4. Carry forward the **function bodies**, not just names
5. Session resumes → Claude has both the summary *and* the code it was working on

**Before (compaction summary):** "I was fixing `HandleHook`."
**After (function carry-forward):** "Here's the actual 40-line function you were editing, plus the 3 other functions you were reading for context."

This eliminates the re-read burst entirely for the functions that matter.

---

## Hook Mechanics (Validated)

### Critical finding: PreCompact cannot inject context

PreCompact hooks are **side-effect only**. They cannot return `additionalContext` or inject anything into the compacted context. The Claude Code documentation explicitly categorizes PreCompact alongside SessionEnd and WorktreeRemove as "no decision control" hooks.

### Solution: two-step relay

**SessionStart** hooks fire after compaction and DO support `additionalContext`. The architecture becomes:

```
Step 1: PreCompact fires
    → hook-client triggers monitor to compute carry-forward payload
    → monitor caches it (keyed by session_id)

Step 2: compaction happens (Claude Code internal)

Step 3: SessionStart fires (same session_id, post-compaction)
    → hook-client queries monitor for cached carry-forward
    → returns via hookSpecificOutput.additionalContext
    → Claude has the function bodies in context
```

This is actually better than direct PreCompact injection:
- PreCompact can take time for heavy work (tree-sitter parsing) without blocking
- SessionStart just retrieves the cached payload — fast
- The `additionalContext` injection path is already proven (hook-client uses it for PreToolUse Read annotations)

### PreCompact stdin payload

```json
{
  "session_id": "abc123",
  "transcript_path": "/path/to/.claude/projects/.../uuid.jsonl",
  "cwd": "/path/to/project",
  "permission_mode": "default",
  "hook_event_name": "PreCompact",
  "trigger": "auto",
  "custom_instructions": ""
}
```

Notable: `transcript_path` points to the full conversation transcript JSONL file. This could be used as a secondary data source beyond hook traces.

### Hook output contract by type

| Hook Type | additionalContext? | Decision control? |
|-----------|:-:|:-:|
| **SessionStart** | Yes | No |
| **PreToolUse** | Yes | Yes (allow/deny/ask) |
| **PostToolUse** | Yes | No |
| **UserPromptSubmit** | Yes | No |
| **PreCompact** | **No** | **No** |
| **SessionEnd** | No | No |

---

## Three Components

The system has three distinct responsibilities:

### Component 1: The Payload

What content gets carried forward. Prioritized function bodies, file manifests, error context — the actual code Claude was working on, not just names and summaries. See [Priority System](#priority-system) below for the tiering.

### Component 2: Building the Payload

The payload can be built **incrementally during the session**, not just at PreCompact time. The monitor already processes every event through `AddEvent()`. Extending it to maintain a per-session "carry-forward state" is natural — the file cache (`SessionFileCache`) already does exactly this pattern: `RecordRead()` on every PostToolUse Read, accumulating state over time.

Incremental building has two advantages over batch-at-PreCompact:
- **No data loss**: The ring buffer holds 1000 events. Long sessions before compaction may overflow, losing early file interactions. Incremental tracking never loses data.
- **Zero latency at PreCompact**: When PreCompact fires, the payload is already built — just snapshot and cache. No ring buffer scan, no file reads, no tree-sitter parsing under time pressure.

The carry-forward state per session would track:
- Files touched (path → {read_count, was_edited, line_ranges[]})
- Functions identified (if tree-sitter is active: file → [Symbol])
- Recent errors (last N PostToolUseFailure messages)

### Component 3: Injecting the Payload

The injection must distinguish **post-compaction SessionStart** from other SessionStart triggers (fresh conversation, resume, subagent spawn). Two independent safety checks:

**Check 1 — Cache existence**: Only sessions that went through PreCompact will have a cached payload. The linking key is `session_id`, which is the same before and after compaction.

| SessionStart trigger | PreCompact preceded it? | Cached payload exists? | Inject? |
|---------------------|:-:|:-:|:-:|
| Fresh conversation | No | No | No |
| Resume / `--continue` | No | No | No |
| **Post-compaction** | **Yes** | **Yes** | **Yes** |
| Subagent spawn | No (different session) | No | No |

**Check 2 — Freshness TTL**: Post-compaction SessionStart fires within milliseconds of PreCompact. Resume/continue happens after seconds to hours. A **30-second TTL** on cached payloads catches any edge case where a stale payload wasn't cleaned up:

```
cache entry = {
    session_id: "abc123",
    payload:    "## Working Context...",
    created_at: time.Now(),
}

// On SessionStart query:
if time.Since(entry.created_at) > 30 * time.Second {
    delete(cache, session_id)  // stale — discard
    return nil
}
```

Both checks must pass for injection. Belt and suspenders.

### Full data flow

```
During session (incremental):
    Every PostToolUse Read/Edit/Write event
        → monitor updates carry-forward state for this session_id
        → tracks: files touched, line ranges, edit flags, functions (if tree-sitter)

PreCompact fires:
    → hook-client POSTs to monitor: POST /hook/PreCompact
    → monitor snapshots carry-forward state → formats payload
    → monitor caches payload (keyed by session_id, timestamped)
    → hook-client exits 0 (PreCompact is side-effect only)

Compaction happens (Claude Code internal)

SessionStart fires (same session_id, post-compaction):
    → hook-client queries monitor: GET /session/carry-forward?session=<id>
    → monitor checks: cached payload exists? fresh (< 30s)? → returns payload
    → hook-client writes to stdout:
      {
        "hookSpecificOutput": {
          "hookEventName": "SessionStart",
          "additionalContext": "## Working Context (carried forward)\n..."
        }
      }
    → Claude has function bodies in context — no re-read burst needed
    → monitor deletes cached payload (consumed, one-shot)
```

### Where each component lives

| Component | Location | Why |
|-----------|----------|-----|
| Carry-forward state (incremental) | `claude-hooks-monitor/internal/monitor/` | Extends existing AddEvent processing |
| Tree-sitter parsing | `claude-hooks-monitor/internal/symbols/` (new) | New package, used by monitor |
| Carry-forward cache | `claude-hooks-monitor/internal/monitor/` | In-memory, keyed by session_id, TTL 30s |
| HTTP endpoint | `claude-hooks-monitor/internal/server/` | New `GET /session/carry-forward` |
| hook-client SessionStart handler | `claude-hooks-monitor/cmd/hook-client/` | Retrieves cached payload, injects via additionalContext |

### Why monitor, not hooks-store

- Monitor has **local file access** (same machine as source code — tree-sitter needs this)
- Monitor has the **ring buffer** with recent events (no MeiliSearch query needed)
- Monitor already does **incremental per-event processing** (file cache pattern)
- Monitor is **low latency** (localhost HTTP)
- hooks-store may run in Docker without volume mounts to project dirs
- The carry-forward is per-session, current-session — no cross-session search needed

---

## Priority System

Not all touched functions deserve carry-forward budget. Priority order:

### Tier 1: Edited functions (highest priority)
Functions that were *modified* via Edit/Write tools. These are the active work — the thing Claude will need to continue editing. Include full function body.

### Tier 2: Heavily-read functions
Functions read 3+ times in the session. Repeated reads suggest these are central to the current task. Include full function body if budget allows, otherwise signature + first few lines.

### Tier 3: Context reads
Functions read 1-2 times. Likely read for context. Include signature only (name, parameters, return type).

### Token budget

| Budget | Fits | Use case |
|-------:|------|----------|
| 1000 tokens | ~3-4 full functions + 10 signatures | Conservative, minimal context impact |
| 2000 tokens | ~8-10 full functions + 15 signatures | Recommended default |
| 3000 tokens | ~15 full functions + 20 signatures | Aggressive, for complex sessions |

The budget is configurable via `hook_monitor.conf`:
```ini
[carry-forward]
token_budget = 2000
```

Compare: the post-compaction re-read burst is 38+ calls × ~500 tokens each = ~19,000 tokens. Even a 3000-token carry-forward is 6x more efficient.

---

## Tree-sitter Integration

### Package: `claude-hooks-monitor/internal/symbols/`

```go
type Symbol struct {
    Name     string // "HandleHook"
    Kind     string // "function", "method", "type", "const"
    Package  string // "server"
    StartLine int
    EndLine   int
    Signature string // "func HandleHook(w http.ResponseWriter, r *http.Request)"
    Body     string // full function body text
}

// IdentifySymbols parses a source file and returns symbols that overlap
// with the given line range. If lineStart==0 && lineEnd==0, returns all
// top-level symbols (for full-file reads).
func IdentifySymbols(filePath string, lineStart, lineEnd int) ([]Symbol, error)
```

### Go library: `github.com/smacker/go-tree-sitter`

Mature Go bindings. Supports Go, Python, JavaScript, TypeScript, Rust, C, C++, Java, Ruby, and more via grammar packages.

### Language detection

By file extension:
- `.go` → Go grammar
- `.py` → Python grammar
- `.js`, `.jsx` → JavaScript grammar
- `.ts`, `.tsx` → TypeScript grammar
- `.rs` → Rust grammar

Unknown extensions: fall back to raw line extraction (no function boundary detection).

### Fallback without tree-sitter

A simpler Phase 1 can work without tree-sitter at all:
- Extract line ranges from hook traces
- Read those raw lines from disk
- Include ±5 lines of surrounding context
- Less precise (might cut functions mid-body) but zero new dependencies

---

## Phased Implementation

### Phase 0: Validate the injection path
- Write a test SessionStart hook that returns static `additionalContext`
- Verify it appears in Claude's context after compaction
- **This is the make-or-break validation.** If `additionalContext` doesn't survive compaction or isn't visible to Claude, the approach needs rethinking
- Estimated effort: 1 hour

### Phase 1: File manifest carry-forward (no tree-sitter)
- PreCompact triggers monitor to scan ring buffer for session file activity
- Monitor builds a manifest: file names, read counts, edit indicators
- SessionStart injects the manifest via additionalContext
- **No tree-sitter, no function bodies** — just "you were working on these files"
- This alone may eliminate many re-reads (Claude knows *which* files to re-read, not *all* files)
- Estimated effort: 4-6 hours

### Phase 2: Raw line extraction
- Extend Phase 1 to include actual line content from touched ranges
- Read lines from disk based on tool_input offset/limit
- Include ±5 context lines
- Still no tree-sitter — raw line ranges may cut mid-function
- Estimated effort: 3-4 hours (incremental from Phase 1)

### Phase 3: Tree-sitter function identification
- Add `internal/symbols/` package with tree-sitter integration
- Parse files to identify function boundaries
- Carry forward complete function bodies instead of raw line ranges
- Clean, precise, respects function boundaries
- Estimated effort: 8-12 hours

### Phase 4: Measurement
- Compare pre-implementation vs post-implementation:
  - Number of re-reads after compaction
  - Time from compaction to productive work
  - Token usage difference
- Use existing MeiliSearch data for before/after analysis
- Estimated effort: 4-6 hours

---

## Why This Reframes Tree-sitter

The deep-memory research plan proposed tree-sitter as a general "symbol-level memory" system (entity graphs, cross-session tracking, concept hubs). This reframes it as a **compaction recovery tool** — one surgical purpose:

```
hook traces (line ranges) → tree-sitter (function identification) → carry forward (actual code)
```

Much smaller scope. Much higher impact per line of code.

---

## Why This Might Actually Work (Unlike General Deep Memory)

The research concluded that Claude won't query memory tools unprompted because it doesn't know what it lost. But this approach doesn't require Claude to query anything:

- **Trigger is automatic**: PreCompact fires → system extracts functions → SessionStart injects them
- **No tool call needed**: The content appears in context via `additionalContext`, not behind an MCP tool
- **Solves a measured problem**: 73% re-read rate, 38-call post-compaction bursts — concrete, quantified waste
- **Proven injection mechanism**: hook-client already uses `additionalContext` for PreToolUse Read annotations

## Relationship to Prior Research

- **20260228-1913_information-seeking-analysis.md**: Established the 73% redundant re-read finding and post-compaction burst of 38+ calls. This proposal directly targets that.
- **20260228-1928_compaction-carry-forward.md**: Proposed a ~600-token summary carry-forward. This extends it to actual code content. **Key correction**: that doc assumed PreCompact could inject context directly — it cannot. The two-step relay (PreCompact → cache → SessionStart → inject) is the correct mechanism.
- **20260228-1757_deep-memory-research.md**: Proposed tree-sitter for general symbol-level memory. This narrows the scope to compaction recovery only.
- **20260228-2037_conclusion.md**: Said deep memory for Claude doesn't work. This sidesteps that — it's not memory retrieval (Claude won't query), it's proactive injection at a known trigger point.

---

## Open Questions

### Resolved

- ~~**Can PreCompact inject context?**~~ No. PreCompact is side-effect only. Solution: two-step relay via SessionStart `additionalContext`. See [Hook Mechanics](#hook-mechanics-validated).
- ~~**How to link post-compaction session to pre-compaction session?**~~ Same `session_id` persists across compaction. Cache keyed by session_id.
- ~~**How to distinguish post-compaction from fresh start / resume / subagent?**~~ Two checks: (1) cached payload must exist for this session_id, (2) payload must be fresh (< 30s TTL). See [Component 3](#component-3-injecting-the-payload).
- ~~**Batch vs incremental payload building?**~~ Incremental during session, snapshot at PreCompact. Follows the file cache pattern. See [Component 2](#component-2-building-the-payload).
- ~~**Stale carry-forward?**~~ 30-second TTL + one-shot consumption (delete after retrieval).

### Remaining

1. **Phase 0 validation**: Does `additionalContext` from a SessionStart hook that fires post-compaction actually appear in Claude's context? The mechanism exists for SessionStart hooks generally, but does it survive the compaction boundary? **This is the go/no-go gate.**

2. **Token budget sweet spot**: What's the optimal carry-forward size? Too small = Claude still re-reads. Too large = wastes context window. Need empirical testing.

3. **Multi-language grammar bundling**: The monitor binary needs tree-sitter grammars compiled in. How much does this bloat the binary? Should grammars be lazy-loaded?

4. **Incremental tree-sitter**: Should tree-sitter parse files on every Read/Edit event (incremental, always up-to-date) or only at PreCompact snapshot time (batch, simpler)? Incremental adds per-event latency but makes the snapshot instant. Could defer to Phase 3 — Phase 1-2 work without tree-sitter.
