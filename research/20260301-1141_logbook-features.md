# Logbook Features — From Tool Call Log to Project Work History

**Date:** 2026-03-01
**Status:** Feature spec for review

## Vision

hooks4claude becomes a **project logbook** — a searchable, browsable history
of all work done with Claude Code, organized by project. Like a developer's
lab notebook: what was asked, what was done, what was decided, across sessions.

The logbook serves the **user**, not Claude. This aligns with the core finding:
hooks4claude's value is observability for humans, not memory for machines.

## Architecture

Three layers, each buildable independently:

```
┌─────────────────────────────────────────────────────────┐
│  Layer 3: PRESENTATION                                  │
│  hooks-log CLI / web dashboard / TUI history tab        │
├─────────────────────────────────────────────────────────┤
│  Layer 2: QUERY                                         │
│  Composed MeiliSearch queries that build session digests │
├─────────────────────────────────────────────────────────┤
│  Layer 1: DATA                                          │
│  transform.go extractions (untapped fields)             │
├─────────────────────────────────────────────────────────┤
│  Layer 0: CAPTURE (already done)                        │
│  hook-client → monitor → hooks-store → MeiliSearch      │
└─────────────────────────────────────────────────────────┘
```

Layer 0 is complete. Features are organized bottom-up: data first, then
queries, then presentation.

---

## Layer 1 Features: Data Extraction

### F1. Extract `last_assistant_message`

**What:** Extract from Stop and SubagentStop events as a top-level searchable field.

**Why:** This is the narrative — Claude's own words describing what it did.
Paired with UserPromptSubmit.prompt, it creates request→response pairs that
are the core of the logbook.

**Where:** `hooks-store/internal/store/transform.go`

**Effort:** Low — same pattern as existing `prompt` extraction.

### F2. Extract `transcript_path`

**What:** Extract from all events as a top-level filterable field.

**Why:** Enables conversation threading. Sessions sharing a transcript file
are continuations of the same conversation. A multi-day feature implementation
becomes one thread instead of disconnected sessions.

**Where:** `hooks-store/internal/store/transform.go`

**Effort:** Low.

### F3. Extract session metadata (`source`, `model`)

**What:** Extract `source` and `model` from SessionStart events.

**Why:** Distinguishes fresh sessions from resumed/compacted ones. Model
identification enables cost analysis. `source: "resume"` marks conversation
continuations.

**Where:** `hooks-store/internal/store/transform.go`

**Effort:** Trivial.

### F4. Extract task metadata (`task_subject`, `task_description`)

**What:** Extract from TaskCompleted events.

**Why:** When Claude uses the task system, completions are machine-structured
summaries of accomplished work — higher signal than free-form responses.

**Where:** `hooks-store/internal/store/transform.go`

**Effort:** Low.

### F5. Backfill existing data

**What:** Run a migration to extract untapped fields from the `data` map of
all existing documents (same pattern as the `has_claude_md` backfill).

**Why:** ~8000+ events already in MeiliSearch would immediately become
searchable by the new fields.

**Where:** `hooks-store/internal/store/migrate.go`

**Effort:** Low — existing migration pattern.

---

## Layer 2 Features: Query Composition

### F6. Session Digest Query

**What:** A composed query that returns a structured session summary:

```json
{
  "session_id": "abc123",
  "project": "/home/user/my-project",
  "started": "2026-03-01T10:00:00Z",
  "ended": "2026-03-01T11:30:00Z",
  "source": "startup",
  "model": "claude-opus-4-6",
  "thread": "/home/user/.claude/projects/.../abc123.jsonl",
  "prompts": ["Fix the race condition", "Also add a timeout"],
  "responses": ["Fixed by adding sync.Mutex...", "Added context.WithTimeout..."],
  "files_modified": ["worker.go", "worker_test.go"],
  "files_read": ["config.go", "main.go", "worker.go"],
  "tool_counts": {"Read": 12, "Edit": 5, "Bash": 8, "Glob": 2},
  "compactions": 0,
  "cost_usd": 0.42
}
```

**How:** Multiple MeiliSearch queries per session:
1. SessionStart event → project, source, model, thread
2. UserPromptSubmit events → prompts
3. Stop events → responses (last_assistant_message)
4. PostToolUse events → files, tool counts
5. SessionEnd event → end time, reason

**Where:** New package `hooks-store/internal/digest/` or standalone CLI tool.

**Effort:** Medium — query composition + aggregation logic.

### F7. Project Timeline Query

**What:** List all sessions for a project, ordered chronologically, with
one-line summaries.

```
2026-03-01 10:00  abc123  startup   3 prompts  12 files  "Fixed race condition..."
2026-03-01 14:30  def456  resume    1 prompt    4 files   "Added timeout config..."
2026-02-28 09:15  ghi789  startup   5 prompts  20 files  "Refactored auth module..."
```

**How:** Facet query on `session_id` filtered by `cwd`, then fetch first
prompt + last response per session.

**Effort:** Low — wraps existing MeiliSearch queries.

### F8. Conversation Thread Query

**What:** Group sessions by `transcript_path` into conversation threads.
Show which sessions continue from which.

```
Thread: abc123.jsonl (3 sessions, Mar 1-3)
  ├─ abc123  startup   Mar 1 10:00  "Started auth refactor"
  ├─ abc123  compact   Mar 1 11:45  (compaction, same session)
  └─ def456  resume    Mar 3 09:00  "Continued auth refactor"
```

**How:** Facet on `transcript_path`, then fetch SessionStart events per
thread sorted by time.

**Effort:** Low.

### F9. File History Query

**What:** For a given file path, show all sessions that touched it and what
was done.

```
src/auth.rs — 3 sessions, 8 edits, 14 reads

  Mar 1 abc123: 3 edits  "Added JWT validation"
  Feb 28 ghi789: 4 edits "Refactored token refresh"
  Feb 25 jkl012: 1 edit  "Fixed import"
```

**How:** Filter PostToolUse by `file_path`, facet by `session_id`, pair
with session digest.

**Effort:** Low.

### F10. Work Search

**What:** Full-text search across prompts AND Claude's responses.

```
$ hooks-log search "race condition"

  Mar 1 abc123 [prompt]: "Fix the race condition in the worker pool"
  Mar 1 abc123 [response]: "Fixed the race condition by adding a sync.Mutex..."
  Feb 20 xyz999 [response]: "...potential race condition in the connection pool..."
```

**How:** Search across `prompt` (hook-prompts index) and `last_message`
(new field) in MeiliSearch.

**Effort:** Low — MeiliSearch already does full-text search.

---

## Layer 3 Features: Presentation

### F11. `hooks-log` CLI Tool

**What:** A standalone CLI that queries MeiliSearch and presents logbook views.

```bash
# Project timeline
hooks-log timeline ~/PROJECTES/mdink

# Today's work across all projects
hooks-log today

# Weekly digest
hooks-log digest --week

# Search across all sessions
hooks-log search "authentication"

# File history
hooks-log file src/auth.rs

# Session detail
hooks-log session abc123

# Conversation thread
hooks-log thread abc123
```

**Where:** New binary. Options:
- `hooks-store/cmd/hooks-log/` (same repo, shared MeiliSearch config)
- `hooks4claude/hooks-log/` (separate tool in parent repo)

**Effort:** Medium — CLI framework + query composition + output formatting.

### F12. Daily/Weekly Digest Generator

**What:** Automated markdown report of work done.

```markdown
# Work Digest — Week of Mar 1, 2026

## mdink (3 sessions, $1.23)
- Fixed race condition in worker pool
- Added configurable timeout with context.WithTimeout
- Refactored auth module to use JWT

## hooks4claude (5 sessions, $2.45)
- Ran carry-forward A/B experiment (conclusion: approach abandoned)
- Documented untapped fields in hooks-store
- Designed logbook feature spec

### Files Most Modified
1. worker.go (8 edits across 3 sessions)
2. auth.rs (5 edits across 2 sessions)
```

**How:** Aggregation query over the past week, grouped by project, using
session digests (F6) and Claude's response summaries.

**Effort:** Medium — report generation + aggregation.

### F13. History Tab in TUI

**What:** Add a "History" tab to the existing hooks-store Bubble Tea TUI.
Browse past sessions, view digests, search.

**Effort:** High — significant TUI work, lower priority than CLI.

---

## Implementation Phases

### Phase 1: Data Foundation (Layer 1)
Features: F1, F2, F3, F4, F5
Effort: 1-2 sessions
Result: All untapped fields extracted and indexed. Existing data backfilled.

### Phase 2: Core Queries (Layer 2)
Features: F6, F7, F10
Effort: 2-3 sessions
Result: Session digest, project timeline, and work search functional.

### Phase 3: CLI Tool (Layer 3)
Features: F11 (with timeline, today, search, session subcommands)
Effort: 2-3 sessions
Result: `hooks-log` usable from terminal.

### Phase 4: Enrichment
Features: F8, F9, F12
Effort: 2-3 sessions
Result: Conversation threads, file history, weekly digests.

### Phase 5: Polish
Features: F13 (optional)
Effort: 3+ sessions
Result: TUI history view.

---

## What This Does NOT Include

- **Memory for Claude** — The carry-forward experiment proved this doesn't work.
  The logbook serves the user only.
- **Real-time streaming** — The existing TUI handles live events. The logbook
  is retrospective.
- **LLM-powered summarization** — Claude's `last_assistant_message` already IS
  the summary. No need to re-summarize with another LLM call.
- **Transcript file parsing** — Using indexed MeiliSearch data only. Parsing
  raw JSONL transcripts is a fallback for data not captured by hooks.

## Open Questions

1. **Where does `hooks-log` live?** Same repo as hooks-store (shared config)
   or separate tool in parent repo?
2. **Should session digests be pre-computed and stored?** Or always composed
   on-the-fly from MeiliSearch queries? Pre-computing would be faster but adds
   a materialization step.
3. **Output format?** Plain text, markdown, JSON? Configurable?
4. **How to handle projects with many sessions?** Pagination, time filtering,
   or both?
