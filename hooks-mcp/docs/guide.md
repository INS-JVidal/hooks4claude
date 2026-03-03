# hooks-mcp Usage Guide

hooks-mcp is an MCP server that gives Claude Code read-only access to your hook event data stored in MeiliSearch. It exposes 8 tools for querying sessions, prompts, costs, errors, and tool usage patterns. Communication happens over stdio transport — Claude Code spawns the binary as a subprocess and exchanges JSON on stdin/stdout.

## Prerequisites

- **Go 1.25+** installed
- **MeiliSearch** running (default `http://localhost:7700`)
- **hooks-store** pipeline active — data must be indexed before queries work
- `~/.local/bin` in your `PATH`

## Install

```bash
cd hooks-mcp
make build        # builds → bin/hooks-mcp
make install      # copies to ~/.local/bin/hooks-mcp
```

Verify:

```bash
hooks-mcp --version
```

## Register with Claude Code

```bash
claude mcp add --transport stdio --scope project hooks-mcp -- hooks-mcp
```

This writes to `.mcp.json` in your project root. The tools become available in the next Claude Code session.

To remove:

```bash
claude mcp remove hooks-mcp
```

## Configuration

All configuration is via environment variables (no CLI flags — stdio MCP constraint).

| Variable | Default | Description |
|----------|---------|-------------|
| `MEILI_URL` | `http://localhost:7700` | MeiliSearch endpoint |
| `MEILI_KEY` | _(empty)_ | MeiliSearch API key |
| `MEILI_INDEX` | `hook-events` | Events index name |
| `PROMPTS_INDEX` | `hook-prompts` | Prompts index name |
| `SESSIONS_INDEX` | `hook-sessions` | Sessions index name |

You can pass env vars through `.mcp.json`:

```json
{
  "mcpServers": {
    "hooks-mcp": {
      "command": "hooks-mcp",
      "env": {
        "MEILI_URL": "http://localhost:7700"
      }
    }
  }
}
```

On startup the server checks MeiliSearch health. If MeiliSearch is unreachable, the server exits with an error on stderr.

---

## Tool Reference

### query-sessions

List Claude sessions filtered by project, date, and model.

| Parameter | Type | Required | Default | Description |
|-----------|------|:--------:|---------|-------------|
| `project_name` | string | | | Filter by project name |
| `date_range` | string | | | Date range filter |
| `model` | string | | | Filter by model name |
| `sort_by` | string | | `started_at` | Sort field: `started_at`, `duration_s`, `total_cost_usd`, `total_events` |
| `limit` | int | | 20 | Max results (1-100) |

**Ask Claude:**

> Show me sessions from the last 3 days for hooks4claude, sorted by cost

**Example output:**

```
Sessions (12 total):

ID        Date              Duration  Prompts  Events  Cost    Model                           Preview
────────  ────────────────  ────────  ───────  ──────  ──────  ──────────────────────────────  ──────────────────────────────────────────────────
af7deb64  2026-03-02 14:20  1h23m     18       142     $2.41   claude-opus-4-5-20250514        Implement the following plan: hooks-mcp — Custom…
b3c91f02  2026-03-02 09:15  45m12s    12       89      $1.05   claude-sonnet-4-5-20250514      plan a deep code review for search of bugs, logi…
c8d42e19  2026-03-01 16:40  32m8s     8        54      $0.72   claude-sonnet-4-5-20250514      Fix the TUI crash when resizing terminal window
```

---

### query-prompts

Get user prompts chronologically, grouped by session.

| Parameter | Type | Required | Default | Description |
|-----------|------|:--------:|---------|-------------|
| `project_name` | string | | | Filter by project name |
| `session_id` | string | | | Session ID or prefix (e.g. `af7deb64`) |
| `date_range` | string | | | Date range filter |
| `query` | string | | | Full-text search within prompt text |
| `limit` | int | | 50 | Max results (1-200) |

**Ask Claude:**

> Show me the prompts from session af7deb64

**Example output:**

```
Prompts (5 total):

── Session af7deb64 ──
  1. [2026-03-02 14:20] Implement the following plan: hooks-mcp — Custom MCP Server for MeiliSearch Hook Data
  2. [2026-03-02 14:45] The build is failing with an undefined reference to jsonMarshal — can you fix that?
  3. [2026-03-02 15:02] Tests pass now. Add CLAUDE.md files for every package.
  4. [2026-03-02 15:18] plan a deep code review for search of bugs, logical errors, duplicated functionalities
  5. [2026-03-02 15:40] looks good, go ahead with the implementation
```

---

### session-summary

Detailed overview of a single session: metadata, tool breakdown, files, prompts, errors.

| Parameter | Type | Required | Default | Description |
|-----------|------|:--------:|---------|-------------|
| `session_id` | string | **yes** | | Session ID or prefix |

**Ask Claude:**

> Give me a summary of session af7deb64

**Example output:**

```
Session af7deb64-1234-5678-abcd-ef0123456789

Started:      2026-03-02 14:20
Duration:     1h23m
Project:      hooks4claude
Model:        claude-opus-4-5-20250514
Source:       vscode
Events:       142
Prompts:      18
Compactions:  2

Tool Usage:
Read    ████████████████████████████████ 48
Edit    ██████████████████████           22
Bash    █████████████████                17
Write   ████████████████                 16
Grep    █████████████                    13
Glob    ████████                          8
Agent   ██████                            6

Files Read (32 unique):
  internal/tools/costs.go
  internal/meili/client.go
  internal/format/format.go
  ...

Files Written (18 unique):
  internal/tools/costs.go
  internal/tools/helpers.go
  internal/format/format.go
  ...

Prompts:
  1. [14:20] Implement the following plan: hooks-mcp — Custom MCP Server for MeiliSearch Hook …
  2. [14:45] The build is failing with an undefined reference to jsonMarshal — can you fix that?
  3. [15:02] Tests pass now. Add CLAUDE.md files for every package.

Errors (3):
  [14:32] af7deb64  Edit: old_string not found in file
  [14:38] af7deb64  Bash: command failed with exit code 1
  [15:10] af7deb64  Edit: old_string is not unique in the file
```

---

### project-activity

Activity tree showing sessions grouped by day and submodule.

| Parameter | Type | Required | Default | Description |
|-----------|------|:--------:|---------|-------------|
| `project_name` | string | **yes** | | Project name |
| `date_range` | string | | `last 7 days` | Date range filter |

Submodules are classified by the files a session touched (not its cwd):
- `claude-hooks-monitor` — files under `claude-hooks-monitor/`
- `hooks-store` — files under `hooks-store/`
- `hooks-mcp` — files under `hooks-mcp/`
- `cross` — sessions touching multiple submodules
- `parent` — sessions not touching any submodule

**Ask Claude:**

> Show project activity for hooks4claude last 7 days

**Example output:**

```
Project: hooks4claude

2026-03-02 (3 sessions)
├── hooks-mcp
│   ├── af7deb64  1h23m  $2.41  Implement the following plan: hooks-mcp — Custom…
│   └── b3c91f02  45m12s $1.05  plan a deep code review for search of bugs, logi…
└── parent
    └── d4e56f78  12m30s $0.18  Update CLAUDE.md with hooks-mcp documentation

2026-03-01 (2 sessions)
├── hooks-store
│   └── c8d42e19  32m8s  $0.72  Add carry-forward experiment scripts
└── claude-hooks-monitor
    └── e9f01234  18m45s $0.35  Fix TUI crash when resizing terminal window
```

---

### search-events

Full-text search across all hook events. Searches `data_flat`, `prompt`, `error_message`, and other text fields.

| Parameter | Type | Required | Default | Description |
|-----------|------|:--------:|---------|-------------|
| `query` | string | **yes** | | Search query |
| `project_name` | string | | | Filter by project name |
| `session_id` | string | | | Session ID or prefix |
| `hook_type` | string | | | Filter: `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `UserPromptSubmit`, `Stop`, etc. |
| `tool_name` | string | | | Filter: `Read`, `Edit`, `Bash`, `Grep`, `Glob`, `Write`, `Agent`, etc. |
| `date_range` | string | | | Date range filter |
| `limit` | int | | 20 | Max results (1-100) |

**Ask Claude:**

> Search for "permission denied" errors in the last 3 days

**Example output:**

```
Events matching "permission denied" (7 total):

[2026-03-02 14:38] af7deb64  PostToolUseFailure  Bash: permission denied: /usr/local/bin/meili
[2026-03-02 09:22] b3c91f02  PostToolUseFailure  Write: permission denied: /etc/claude-code/settings.json
[2026-03-01 16:55] c8d42e19  PostToolUseFailure  Bash: mkdir: cannot create directory: Permission denied
```

---

### error-analysis

Analyze `PostToolUseFailure` events: frequency by tool, patterns, recent errors.

| Parameter | Type | Required | Default | Description |
|-----------|------|:--------:|---------|-------------|
| `project_name` | string | | | Filter by project name |
| `session_id` | string | | | Session ID or prefix |
| `date_range` | string | | | Date range filter |
| `limit` | int | | 50 | Max error events to analyze |

**Ask Claude:**

> Analyze errors for hooks4claude this week

**Example output:**

```
Error Analysis (23 total errors):

Errors by Tool:
Bash    ██████████████████████████████ 12
Edit    █████████████████              7
Write   ██████                         3
Read    █                              1

Recent Errors:
  [2026-03-02 15:10] af7deb64  Edit: old_string is not unique in the file — provide more context
  [2026-03-02 14:38] af7deb64  Bash: command failed with exit code 1
  [2026-03-02 14:32] af7deb64  Edit: old_string not found in file /home/user/project/main.go
  [2026-03-02 09:22] b3c91f02  Write: permission denied: /etc/claude-code/settings.json
  [2026-03-01 16:55] c8d42e19  Bash: mkdir: cannot create directory: Permission denied
```

---

### cost-analysis

Cost and token usage breakdown with totals, averages, and grouped views.

| Parameter | Type | Required | Default | Description |
|-----------|------|:--------:|---------|-------------|
| `project_name` | string | | | Filter by project name |
| `date_range` | string | | `last 7 days` | Date range filter |
| `group_by` | string | | `day` | Grouping: `day`, `session`, `model` |

**Ask Claude:**

> Show cost analysis for hooks4claude last 7 days grouped by model

**Example output:**

```
Cost Analysis (8 sessions, 8 total):

Total Cost:    $7.48
Average Cost:  $0.94 per session
Input Tokens:  1.2M
Output Tokens: 89.5K

Cost by Model:
Model                           Sessions  Cost
──────────────────────────────  ────────  ──────
claude-opus-4-5-20250514        3         $5.91
claude-sonnet-4-5-20250514      5         $1.57

Most Expensive Sessions:
ID        Date              Cost    Duration  Prompts  Model
────────  ────────────────  ──────  ────────  ───────  ──────────────────────────────
af7deb64  2026-03-02 14:20  $2.41   1h23m     18       claude-opus-4-5-20250514
b3c91f02  2026-03-02 09:15  $1.05   45m12s    12       claude-sonnet-4-5-20250514
c8d42e19  2026-03-01 16:40  $0.72   32m8s     8        claude-sonnet-4-5-20250514
```

---

### tool-usage

Tool distribution and file access patterns.

| Parameter | Type | Required | Default | Description |
|-----------|------|:--------:|---------|-------------|
| `project_name` | string | | | Filter by project name |
| `session_id` | string | | | Session ID or prefix |
| `date_range` | string | | | Date range filter |

**Ask Claude:**

> What tools were used in session af7deb64?

**Example output:**

```
Tool Usage (130 total tool calls):

Tool Distribution:
Read    ████████████████████████████████ 48
Edit    ██████████████████████           22
Bash    █████████████████                17
Write   ████████████████                 16
Grep    █████████████                    13
Glob    ████████                          8
Agent   ██████                            6

Most-Read Files (32 unique):
  internal/tools/costs.go
  internal/meili/client.go
  internal/format/format.go
  cmd/hooks-mcp/main.go
  internal/tools/sessions.go

Most-Written Files (18 unique):
  internal/tools/costs.go
  internal/tools/helpers.go
  internal/format/format.go
  internal/meili/client.go
```

When querying across multiple sessions (no `session_id`), files are aggregated with frequency counts:

```
Most-Read Files (84 unique):
   12x internal/tools/costs.go
    9x internal/meili/client.go
    8x CLAUDE.md
    7x internal/format/format.go
```

---

## Date Range Syntax

All tools that accept `date_range` support these formats:

| Format | Example | Meaning |
|--------|---------|---------|
| _(empty)_ | | No date filter (all data) |
| `today` | `today` | Current UTC day |
| `yesterday` | `yesterday` | Previous UTC day |
| `last N days` | `last 3 days` | N days including today |
| `last N hours` | `last 6 hours` | Last N hours from now |
| `YYYY-MM-DD` | `2026-03-01` | Single day |
| `YYYY-MM-DD..YYYY-MM-DD` | `2026-02-28..2026-03-02` | Date range (end-exclusive) |

All times are UTC. Case-insensitive. Whitespace-trimmed.

## Tips

- **Short session IDs work.** Type the first 8 characters instead of the full UUID — the server resolves the prefix automatically. If ambiguous, it returns an error listing the matches.
- **Default date ranges.** `cost-analysis` and `project-activity` default to `last 7 days` when no `date_range` is given. All other tools search everything.
- **Read-only.** All 8 tools are purely read-only. Nothing in MeiliSearch or your filesystem is modified.
- **Errors don't crash the server.** If MeiliSearch returns an error, the tool reports it as an MCP error result and the connection stays alive for subsequent calls.
- **stdout is reserved.** The binary uses stdout exclusively for the MCP protocol. Diagnostic messages go to stderr.
