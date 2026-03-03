# Work Log Challenges: Feasibility Research

**Date:** 2026-03-01
**Status:** Research — 4 challenges assessed individually

---

## Challenge 1: No Assistant Responses Captured

### Current State
Hooks capture what Claude *did* (tool calls) but not what Claude *said* (explanations, decisions, analysis).

### Feasibility: SOLVED — Two mechanisms exist

#### A. `last_assistant_message` field (direct)

The **Stop** and **SubagentStop** hooks both receive a `last_assistant_message` field containing Claude's final text response:

```json
{
  "hook_event_name": "Stop",
  "session_id": "abc123",
  "last_assistant_message": "I've completed the refactoring. Here's a summary..."
}
```

**Limitation:** Only captures the *last* response before Claude stops. In a multi-turn agentic loop (prompt → tool → response → tool → response → stop), intermediate responses are NOT individually hooked. You get the final text block only.

**What this gives us:** The summary/conclusion of each turn. Often the most valuable part — Claude tends to summarize what it did at the end of each turn.

#### B. `transcript_path` field (indirect, full history)

Every hook event includes `transcript_path` pointing to the session's JSONL transcript file. Each line contains:

```json
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Full response..."}]},"timestamp":"2025-06-02T18:47:06.267Z"}
```

A hook script can parse this file to extract ALL assistant messages, not just the last one.

**Limitation:** Requires file I/O and JSON parsing. Not real-time per-message. The transcript grows large.

### Recommended Approach

**Use both:**
1. Capture `last_assistant_message` from the **Stop** hook on every turn — lightweight, gives you the summary
2. Optionally parse `transcript_path` from **SessionEnd** for the full conversation — heavyweight, for archival

### Implementation

In hooks-store:
- Add `last_assistant_message` extraction in `transform.go` for Stop events
- Index as a searchable field in MeiliSearch
- Add a `stop_messages` index for full-text search across all Claude responses

In hook-client:
- Already forwards Stop events — no changes needed

### Status: Feasible, moderate effort

---

## Challenge 2: No Semantic Summaries

### Current State
Events are tool-level: "Read file X", "Edit file Y". No higher-level record of "Implemented authentication module."

### Feasibility: PARTIALLY SOLVED via existing data

#### What we already have that enables summarization

1. **User prompts** (`UserPromptSubmit.prompt`) — what the user asked for
2. **`last_assistant_message`** from Stop hook — Claude's own summary of what it did
3. **`TaskCompleted`** events — structured task completion with `task_subject` and `task_description`
4. **Tool call patterns** — sequence of Read/Edit/Write calls reveals what files were modified

#### Approach A: Claude-generated summaries (zero cost, already there)

`last_assistant_message` from the Stop hook IS the semantic summary. Claude naturally ends each turn with a summary like "I've refactored the authentication module, updated the tests, and fixed the race condition in the worker pool."

Pair this with the user's prompt and you have:
- **User asked:** "Fix the race condition in auth" (UserPromptSubmit)
- **Claude did:** Read 5 files, edited 3, ran tests (PostToolUse events)
- **Claude said:** "Fixed the race condition by adding a mutex..." (Stop.last_assistant_message)

This is already a semantic summary. We just need to capture and index it.

#### Approach B: Post-hoc clustering (medium effort)

Group tool calls into logical tasks by:
- Time proximity (calls within the same burst belong together)
- File proximity (edits to files in the same package = one task)
- Session boundaries (SessionStart to Stop = one unit of work)

This gives you "session summaries" derived from tool patterns without any LLM processing.

#### Approach C: LLM-powered summarization (expensive)

Feed session events to an LLM and ask for a summary. Effective but adds cost and latency. Not needed if Approach A (capturing `last_assistant_message`) provides sufficient quality.

### Recommended Approach

**Approach A first.** Capture `last_assistant_message` + `UserPromptSubmit.prompt` pairs. This is the lowest-effort path to semantic summaries and uses Claude's own words.

### Status: Feasible, low-to-moderate effort

---

## Challenge 3: No Dashboard or Timeline View

### Current State
The TUI shows live events as they stream in. No way to browse historical sessions, see a timeline, or get weekly summaries.

### Feasibility: STRAIGHTFORWARD — query layer over MeiliSearch

#### What MeiliSearch already provides

All the data is indexed and queryable:
- Filter by `session_id`, `hook_type`, `tool_name`, `cwd`, `timestamp_unix`
- Sort by `timestamp_unix`
- Facet on any filterable field
- Full-text search across `data_flat` and `prompt`

#### Possible implementations

| Option | Stack | Effort | Notes |
|--------|-------|--------|-------|
| CLI tool | Go or Bash + jq | Low | `hooks-log --project mdink --today` |
| Web dashboard | HTML + MeiliSearch REST API | Medium | MeiliSearch has a built-in REST API, no backend needed |
| Extended TUI | Bubble Tea (existing) | Medium | Add a "history" tab to the existing hooks-store TUI |
| Report generator | Bash script | Low | Weekly markdown report from MeiliSearch queries |

#### Example queries (already possible today)

```bash
# All sessions for a project, sorted by time
curl "$MEILI/indexes/hook-events/search" -d '{
  "filter": "cwd = \"/home/user/project\"",
  "facets": ["session_id"],
  "limit": 0
}'

# Files edited in last 24 hours
curl "$MEILI/indexes/hook-events/search" -d '{
  "filter": "tool_name = Edit AND timestamp_unix > 1709200000",
  "attributesToRetrieve": ["session_id", "file_path", "timestamp"],
  "sort": ["timestamp_unix:desc"]
}'

# All user prompts this week
curl "$MEILI/indexes/hook-events/search" -d '{
  "filter": "hook_type = UserPromptSubmit AND timestamp_unix > 1709000000",
  "attributesToRetrieve": ["prompt", "session_id", "timestamp", "cwd"]
}'
```

### Recommended Approach

Start with a **CLI report tool** — a Go or Bash script that outputs session summaries, file activity, and work timelines. Graduate to a web dashboard later if needed.

### Status: Feasible, low effort for CLI, medium for web

---

## Challenge 4: No Cross-Session Linking

### Current State
When a session is continued from a previous one (via `--resume` or `--continue`), the two sessions share no explicit link in the event data. A multi-day feature spans multiple sessions with no thread connecting them.

### Feasibility: PARTIALLY SOLVED via existing signals

#### What we already have

1. **`SessionStart.source`** field distinguishes:
   - `"startup"` — fresh session
   - `"resume"` — continued from a previous session
   - `"compact"` — post-compaction restart (same session_id)
   - `"clear"` — conversation cleared

2. **`session_id` survives compaction** — a compaction triggers SessionEnd + SessionStart with the SAME session_id. This is already linked.

3. **`transcript_path`** — resumed sessions may point to the same or related transcript file.

4. **`cwd` overlap** — sessions in the same project working on the same files are implicitly related.

#### What's missing

- **Explicit parent session ID** — when using `--resume <session_id>`, the new SessionStart doesn't carry the parent session's ID in the hook payload. The link exists in Claude Code's internal state but isn't exposed to hooks.
- **Continuation context** — the "session continuation summary" that Claude Code generates when resuming isn't available in hook events.

#### Possible approaches

| Approach | How | Reliability |
|----------|-----|-------------|
| **Transcript path matching** | Sessions sharing the same `transcript_path` are the same conversation | High — direct link |
| **Temporal + project heuristic** | Sessions in same `cwd` within N hours of each other = likely related | Medium — may false-match |
| **File overlap analysis** | Sessions that touch the same files in the same project = likely related | Medium — may over-link |
| **Parse transcript for resume context** | Read transcript JSONL for continuation markers | High — requires file parsing |
| **Hook-client enrichment** | In SessionStart handler, check if transcript file pre-exists (= resumed) and extract parent session info | High — custom logic |

#### The transcript_path approach

This is the most reliable. When Claude resumes a session:
- The `transcript_path` points to the existing transcript JSONL file
- The same file gets new entries appended
- A new `session_id` may or may not be assigned

If we capture `transcript_path` in hooks-store (it's already in the event data, just not extracted as a top-level field), we can:
1. Group all sessions sharing the same `transcript_path` into a conversation thread
2. Detect resumed sessions when a SessionStart with `source: "resume"` fires
3. Build a conversation timeline showing how work progressed across restarts

### Recommended Approach

1. **Extract `transcript_path` as a top-level indexed field** in hooks-store
2. **Extract `source` from SessionStart** as a top-level field
3. **Group sessions by transcript_path** for conversation threading
4. Optionally: in hook-client, detect resumed sessions and inject parent session metadata

### Status: Feasible, moderate effort

---

## Priority Assessment

| Challenge | Feasibility | Effort | Value | Priority |
|-----------|-------------|--------|-------|----------|
| 1. Assistant responses | High (data already available) | Low | High | **1st** |
| 2. Semantic summaries | High (piggybacks on #1) | Low | High | **2nd** |
| 4. Cross-session linking | High (transcript_path) | Medium | Medium | **3rd** |
| 3. Dashboard/timeline | High (query layer only) | Medium | Medium | **4th** |

Challenges 1 and 2 are tightly coupled — capturing `last_assistant_message` from Stop hooks solves both. This should be the first implementation target.

---

## Key Data Fields Not Yet Captured

These fields exist in hook events but aren't extracted as top-level indexed fields in hooks-store:

| Field | Hook Type | Value for Work Log |
|-------|-----------|-------------------|
| `last_assistant_message` | Stop, SubagentStop | Claude's response text — the narrative |
| `transcript_path` | All events | Links sessions into conversation threads |
| `source` | SessionStart | Distinguishes fresh/resumed/compacted sessions |
| `task_subject` | TaskCompleted | Structured task descriptions |
| `task_description` | TaskCompleted | Detailed task requirements |
| `model` | SessionStart | Which Claude model was used |
| `notification_type` | Notification | Type of system notification |
| `agent_type` | SubagentStart/Stop | Role of spawned subagents |

Extracting these in `transform.go` would immediately enrich the work log without any changes to the capture pipeline.
