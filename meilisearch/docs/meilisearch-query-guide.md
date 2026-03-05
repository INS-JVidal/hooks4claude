# MeiliSearch Query Guide

Reference for querying hook event data stored in MeiliSearch.
Read this before writing any `curl` command against the database.

**Sandbox note:** MeiliSearch runs on localhost:7700. The Claude Code sandbox blocks localhost. All curl commands require `dangerouslyDisableSandbox: true`.

## Indexes

| Index | Documents | Purpose |
|-------|-----------|---------|
| `hook-events` | All events | Every hook event across all types. Full raw data + promoted fields. |
| `hook-prompts` | UserPromptSubmit only | Lean index for prompt analysis. No `data` blob. |
| `hook-sessions` | One per session | Pre-aggregated session summaries with tool counts, file lists, cost. |

Use `hook-prompts` when you only need user prompts (faster, cleaner schema).
Use `hook-sessions` for session-level queries (cost, duration, tool usage overview).
Use `hook-events` for event-level detail.

## Document Structure

### hook-events: Two-level architecture

Each document has **promoted top-level fields** for filtering and a **nested `data` object** with the complete raw payload.

**Rule: filter on top-level, extract detail from `data`.**

```
Top-level (filter/sort/facet)     Nested data.* (detail extraction)
─────────────────────────────     ─────────────────────────────────
session_id                        data.tool_input.command (Bash)
hook_type                         data.tool_input.file_path (Read/Edit)
tool_name                         data.tool_input.pattern (Grep)
timestamp_unix                    data.tool_input.offset, .limit
file_path                         data.tool_response.* (full output)
cwd                               data.last_assistant_message
project_dir                       data._monitor.* (metadata)
permission_mode                   data.prompt (on UserPromptSubmit)
cost_usd                          ... any field from the raw hook JSON
```

The `data_flat` field is a space-separated string of all leaf string values from `data`. It enables full-text search across nested content without knowing the field name.

### hook-events: Field reference

| Field | Type | Filterable | Sortable | Searchable | Present on |
|-------|------|:---:|:---:|:---:|------------|
| `id` | string | - | - | - | All (primary key) |
| `hook_type` | string | Y | - | Y | All |
| `timestamp` | string | - | - | - | All (ISO 8601) |
| `timestamp_unix` | int64 | Y | Y | - | All |
| `session_id` | string | Y | - | Y | All |
| `tool_name` | string | Y | - | Y | Tool events only |
| `file_path` | string | Y | - | - | Read/Edit/Write/Glob |
| `cwd` | string | Y | - | - | All |
| `project_dir` | string | Y | - | - | All |
| `project_name` | string | Y | - | Y | All (derived from project_dir) |
| `permission_mode` | string | Y | - | - | Most |
| `has_claude_md` | bool | Y | - | - | Most |
| `cost_usd` | float64 | Y | Y | - | Stop/SubagentStop |
| `input_tokens` | int64 | - | Y | - | Stop/SubagentStop |
| `output_tokens` | int64 | - | Y | - | Stop/SubagentStop |
| `prompt` | string | - | - | Y | UserPromptSubmit |
| `error_message` | string | - | - | Y | PostToolUseFailure |
| `last_message` | string | - | - | Y | Stop/SubagentStop |
| `source` | string | Y | - | - | SessionStart |
| `model` | string | Y | - | - | SessionStart |
| `task_subject` | string | - | - | Y | TaskCompleted |
| `transcript_path` | string | Y | - | - | All |
| `data_flat` | string | - | - | Y | All (full-text blob) |
| `data` | object | - | - | - | All (raw payload) |

### hook-prompts: Field reference

| Field | Type | Filterable | Sortable | Searchable |
|-------|------|:---:|:---:|:---:|
| `id` | string | - | - | - |
| `hook_type` | string | - | - | - |
| `timestamp` | string | - | - | - |
| `timestamp_unix` | int64 | Y | Y | - |
| `session_id` | string | Y | - | Y |
| `prompt` | string | - | - | Y |
| `prompt_length` | int | Y | Y | - |
| `cwd` | string | Y | - | - |
| `project_dir` | string | Y | - | - |
| `project_name` | string | Y | - | Y |
| `permission_mode` | string | Y | - | - |
| `has_claude_md` | bool | Y | - | - |

### hook-sessions: Field reference

| Field | Type | Filterable | Sortable | Searchable |
|-------|------|:---:|:---:|:---:|
| `id` | string | - | - | - |
| `session_id` | string | Y | - | Y |
| `project_name` | string | Y | - | Y |
| `project_dir` | string | - | - | - |
| `cwd` | string | - | - | - |
| `started_at` | string | Y | Y | - |
| `ended_at` | string | - | - | - |
| `duration_s` | float64 | Y | Y | - |
| `source` | string | Y | - | - |
| `model` | string | Y | - | - |
| `permission_mode` | string | Y | - | - |
| `has_claude_md` | bool | Y | - | - |
| `total_events` | int | Y | Y | - |
| `total_prompts` | int | Y | - | - |
| `compaction_count` | int | - | - | - |
| `read_count` | int | - | - | - |
| `edit_count` | int | - | - | - |
| `bash_count` | int | - | - | - |
| `grep_count` | int | - | - | - |
| `glob_count` | int | - | - | - |
| `write_count` | int | - | - | - |
| `agent_count` | int | - | - | - |
| `other_tool_count` | int | - | - | - |
| `total_cost_usd` | float64 | Y | Y | - |
| `input_tokens` | int64 | - | Y | - |
| `output_tokens` | int64 | - | Y | - |
| `prompt_preview` | string | - | - | Y |
| `files_read` | []string | - | - | Y |
| `files_written` | []string | - | - | Y |

Tool counts are from `PreToolUse` events only (avoids double-counting with PostToolUse). File lists are from `PostToolUse` (confirmed completions). `prompt_preview` is the first user prompt, truncated to 500 chars.

## Hook Types

| Hook Type | Count | Key fields | When |
|-----------|-------|------------|------|
| `PreToolUse` | ~4600 | tool_name, file_path | Before a tool runs |
| `PostToolUse` | ~4500 | tool_name, file_path | After a tool succeeds |
| `PostToolUseFailure` | ~87 | tool_name, error_message | After a tool fails |
| `UserPromptSubmit` | ~233 | prompt | User sends a message |
| `Stop` | ~210 | last_message, cost_usd, tokens | Claude finishes a turn |
| `SubagentStop` | ~225 | last_message, cost_usd, tokens | Subagent finishes |
| `SubagentStart` | ~61 | - | Subagent spawned |
| `SessionStart` | ~105 | source, model | Session begins |
| `SessionEnd` | ~73 | - | Session ends |
| `PreCompact` | ~28 | - | Context compaction triggered |
| `Notification` | ~97 | - | System notifications |
| `PermissionRequest` | ~158 | - | User prompted for permission |
| `TaskCompleted` | ~25 | task_subject | Todo item completed |

Use `PostToolUse` (not `PreToolUse`) when you want confirmed completed actions.

## Query Patterns

### 1. Day summary — facets with zero documents

The most efficient pattern. Returns distributions without transferring any documents.

```bash
curl -s 'http://localhost:7700/indexes/hook-events/search' \
  -H 'Content-Type: application/json' -d '{
  "filter": "timestamp_unix >= 1772323200 AND timestamp_unix < 1772409600",
  "facets": ["session_id", "hook_type", "tool_name"],
  "limit": 0
}'
```

Read `.estimatedTotalHits` for count, `.facetDistribution.*` for breakdowns.

### 2. Prompt timeline — what the user asked

```bash
curl -s 'http://localhost:7700/indexes/hook-prompts/search' \
  -H 'Content-Type: application/json' -d '{
  "filter": "timestamp_unix >= START AND timestamp_unix < END",
  "sort": ["timestamp_unix:asc"],
  "limit": 100,
  "attributesToRetrieve": ["session_id", "timestamp_unix", "prompt", "cwd"]
}'
```

Add `AND cwd = "/path/to/project"` to scope to a specific project.

### 3. File analysis — what Claude read and wrote

```bash
# Files read (completed reads only)
"filter": "session_id = \"abc\" AND tool_name = Read AND hook_type = PostToolUse"
# → use file_path (top-level) for grouping

# Files edited
"filter": "session_id = \"abc\" AND hook_type = PostToolUse"
# → jq: select(.tool_name == "Edit" or .tool_name == "Write")
```

Group by `file_path` with jq to get read/write counts per file.

### 4. Bash commands — what Claude executed

```bash
"filter": "session_id = \"abc\" AND tool_name = Bash AND hook_type = PreToolUse"
# → extract data.tool_input.command from each hit
```

Use `PreToolUse` here to see what was attempted, including commands that may have failed.

### 5. Full-text search — find anything by content

```bash
"q": "some keyword"
```

Searches across `data_flat` (all nested string values) plus `hook_type`, `tool_name`, `session_id`, `prompt`, `error_message`, `last_message`, `task_subject`. Good for discovery when you don't know which field holds what you're looking for.

### 6. Session cost and token usage

```bash
"filter": "session_id = \"abc\" AND hook_type = Stop",
"attributesToRetrieve": ["cost_usd", "input_tokens", "output_tokens", "timestamp_unix"],
"sort": ["timestamp_unix:asc"]
```

Each `Stop` event carries cumulative token counts and cost for that turn.

### 7. Errors in a session

```bash
"filter": "session_id = \"abc\" AND hook_type = PostToolUseFailure",
"attributesToRetrieve": ["tool_name", "error_message", "timestamp_unix"]
```

### 8. Session metadata

```bash
"filter": "session_id = \"abc\" AND hook_type = SessionStart",
"attributesToRetrieve": ["source", "model", "cwd", "project_dir", "has_claude_md"]
```

`source` tells you how the session started: `startup`, `resume`, `compact`, or `clear`.

## jq Recipes

```bash
# Format timestamps
.timestamp_unix | todate

# Shorten session IDs for display
.session_id[0:8]

# Strip path prefix
.file_path | sub("/home/user/project/"; "")

# Facet distribution → sorted list
.facetDistribution.tool_name | to_entries | sort_by(-.value)

# Group files by path and count
[.hits[] | .file_path] | group_by(.) | map({file: .[0], count: length}) | sort_by(-.count)

# Filter cwd client-side (when MeiliSearch filter is too strict)
[.hits[] | select(.cwd | test("hooks4claude|hooks-store"))]
```

## Pagination

MeiliSearch caps results at 1000 per request. For larger result sets, paginate:

```bash
# Page 1
"limit": 1000, "offset": 0
# Page 2
"limit": 1000, "offset": 1000
```

Max total hits configured at 10,000. Faceting max values per facet: 500.

## Gotchas

| Issue | Cause | Fix |
|-------|-------|-----|
| Sandbox blocks localhost | Default network restrictions | `dangerouslyDisableSandbox: true` on every curl |
| Index name is `hook-events` | Hyphen, not underscore | Don't use `hook_events` |
| `estimatedTotalHits` | MeiliSearch approximates for large sets | Exact under 1000 filtered results |
| `file_path` is null on Bash events | Only promoted from `tool_input.file_path` (Read/Edit/Write/Glob) | Use `data.tool_input.command` for Bash |
| `prompt` at different depths | Top-level on `hook-prompts`, in `data.prompt` on `hook-events` | Use `hook-prompts` index for prompt queries |
| `tool_input` not top-level | Only `file_path` was promoted from it | Access via `data.tool_input.*` for command, pattern, etc. |
| jq `--argjson` ARG_MAX | Large JSON arrays exceed shell limits | Use `--slurpfile` with temp files |

## Unix Timestamp Reference

```bash
# Get timestamps for a date range
date -d "2026-03-01 00:00:00 UTC" +%s  # → 1772323200
date -d "2026-03-02 00:00:00 UTC" +%s  # → 1772409600
```

## Common Tasks — Step by Step

### Reconstruct a day's activity

1. Get timestamps: `date -d "YYYY-MM-DD 00:00:00 UTC" +%s`
2. Facet query with `limit: 0` → session count, tool breakdown, hook types
3. Query `hook-prompts` with time range + `cwd` filter → what user asked
4. For interesting sessions, query `hook-events` for file and tool detail

### Compare read vs write on a session

1. Filter `tool_name = Read AND hook_type = PostToolUse` → group by `file_path`
2. Filter `(tool_name = Edit OR tool_name = Write) AND hook_type = PostToolUse` → group by `file_path`
3. Compare lists: files read but never written = reference files; files written more than read = target files

### Scope to a project

Use `project_name` for clean filtering across submodules:
- Filter: `project_name = "hooks4claude"` (matches all submodule sessions)
- Exact path: `project_dir = "/home/user/hooks4claude"` (single project_dir only)
- Exact cwd: `cwd = "/home/user/hooks4claude/hooks-store"` (single working dir)
- Full-text: `"q": "project-name"` (searches `data_flat`)

### Session overview

Query `hook-sessions` for pre-aggregated session summaries:

```bash
curl -s 'http://localhost:7700/indexes/hook-sessions/search' \
  -H 'Content-Type: application/json' -d '{
  "filter": "project_name = hooks4claude",
  "sort": ["started_at:desc"],
  "limit": 20,
  "attributesToRetrieve": ["session_id", "started_at", "duration_s", "total_events",
    "total_prompts", "total_cost_usd", "model", "prompt_preview"]
}'
```
