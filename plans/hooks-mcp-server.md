# Plan: hooks-mcp — Custom MCP Server for MeiliSearch Hook Data

**Date:** 2026-03-02
**Status:** Planned
**Language:** Go
**Type:** New standalone binary (separate go.mod, no imports from hooks-store or monitor)

## Context

Querying MeiliSearch from Claude Code currently uses raw `curl + jq` in Bash, causing
consistent friction: sandbox blocks localhost, filter syntax errors, session ID prefix
mismatches, boilerplate repetition, manual timestamp arithmetic, and no schema awareness.

This plan builds a standalone Go binary (`hooks-mcp`) that exposes domain-specific MCP
tools wrapping MeiliSearch. It connects via stdio transport and is registered via `.mcp.json`.
All tools are read-only queries — never writes to MeiliSearch.

See also: [docs/meilisearch-integration-strategy.md](../docs/meilisearch-integration-strategy.md) for the analysis that led to this decision.

## Dependencies

- `github.com/modelcontextprotocol/go-sdk v1.4.0` — official Go MCP SDK
- `github.com/meilisearch/meilisearch-go v0.36.1` — same version as hooks-store

## Module Structure

```
hooks-mcp/                            <- new directory in hooks4claude parent repo
├── go.mod                            <- module hooks-mcp, go 1.25.0
├── Makefile
├── CLAUDE.md
├── cmd/hooks-mcp/
│   ├── main.go                       <- entry: env config, meili connect, register tools, stdio serve
│   └── CLAUDE.md
└── internal/
    ├── CLAUDE.md
    ├── meili/
    │   ├── client.go                 <- typed MeiliSearch wrapper (Searcher interface + MeiliClient)
    │   ├── client_test.go
    │   └── CLAUDE.md
    ├── dateparse/
    │   ├── dateparse.go              <- "today", "last 3 days", "2026-02-28..03-01" -> unix range
    │   ├── dateparse_test.go
    │   └── CLAUDE.md
    ├── resolve/
    │   ├── session.go                <- session ID prefix -> full UUID resolution
    │   ├── session_test.go
    │   └── CLAUDE.md
    ├── format/
    │   ├── format.go                 <- Table, Tree, BarChart, FormatDuration, FormatCost, ShortID
    │   ├── format_test.go
    │   └── CLAUDE.md
    └── tools/
        ├── register.go               <- RegisterAll(server, client) wiring
        ├── sessions.go               <- query-sessions
        ├── prompts.go                <- query-prompts
        ├── summary.go                <- session-summary
        ├── activity.go               <- project-activity
        ├── events.go                 <- search-events
        ├── errors.go                 <- error-analysis
        ├── costs.go                  <- cost-analysis
        ├── toolstats.go              <- tool-usage
        └── CLAUDE.md
```

## Configuration (env vars only — no CLI flags for stdio MCP servers)

| Env Var | Default | Description |
|---------|---------|-------------|
| `MEILI_URL` | `http://localhost:7700` | MeiliSearch endpoint |
| `MEILI_KEY` | `""` | MeiliSearch API key |
| `MEILI_INDEX` | `hook-events` | Events index name |
| `PROMPTS_INDEX` | `hook-prompts` | Prompts index name |
| `SESSIONS_INDEX` | `hook-sessions` | Sessions index name |

## Shared Infrastructure

### Searcher Interface (`internal/meili/client.go`)

Decouples tools from concrete MeiliSearch client — enables mock testing.

```go
type Searcher interface {
    SearchSessions(ctx context.Context, opts SessionSearchOpts) ([]SessionHit, int64, error)
    SearchPrompts(ctx context.Context, opts PromptSearchOpts) ([]PromptHit, int64, error)
    SearchEvents(ctx context.Context, opts EventSearchOpts) ([]EventHit, int64, error)
    FacetEvents(ctx context.Context, filter string, facets []string) (*FacetResult, error)
    ResolveSessionPrefix(ctx context.Context, prefix string) (string, error)
}
```

Hit types mirror the MeiliSearch document schemas from `hooks-store/internal/store/store.go`
(SessionDocument, PromptDocument, Document) but are independent types — no import from hooks-store.

Filter builder handles quote escaping and timestamp ranges internally. Search methods
accept `Limit` and `Offset` with sensible defaults.

### Date Parsing (`internal/dateparse/dateparse.go`)

```go
func ParseRange(input string) (start, end int64, err error)
```

Supports: `""` (no filter), `"today"`, `"yesterday"`, `"last N days"`, `"last N hours"`,
`"2026-02-28"`, `"2026-02-28..2026-03-01"`. All UTC. Returns unix timestamps. Must also
produce ISO 8601 strings for the sessions index (`started_at` is a string filter).

### Session ID Resolution (`internal/resolve/session.go`)

```go
func Resolve(ctx context.Context, s Searcher, prefix string) (string, error)
```

If 36 chars -> return as-is. Otherwise search sessions index with `q: prefix`, confirm
prefix match client-side (MeiliSearch is fuzzy), return full UUID. Error on 0 or 2+ matches.

### Format Helpers (`internal/format/format.go`)

`Table`, `Tree`, `BarChart`, `FormatDuration`, `FormatCost`, `FormatTokens`, `ShortID`,
`FormatTimestamp`, `TruncatePrompt`. Pure functions, no dependencies.

## Tools (8 total)

### 1. query-sessions

List sessions filtered by project, date, model.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| project_name | string | no | Filter by project (e.g. "hooks4claude") |
| date_range | string | no | "today", "last 3 days", "2026-02-28", etc. |
| model | string | no | Filter by model |
| sort_by | string | no | started_at (default), duration_s, total_cost_usd, total_events |
| limit | int | no | Max results (default 20, max 100) |

Returns: formatted table with ID, date, duration, prompts, events, cost, model, preview.

### 2. query-prompts

Get user prompts chronologically, grouped by session.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| project_name | string | no | Filter by project |
| session_id | string | no | Session ID or prefix (e.g. "af7deb64") |
| date_range | string | no | Human-friendly date range |
| query | string | no | Full-text search within prompt text |
| limit | int | no | Max results (default 50, max 200) |

Returns: chronological prompt list grouped by session with timestamps.

### 3. session-summary

Detailed overview of one session. Multi-index query (sessions + prompts + events).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| session_id | string | **yes** | Session ID or prefix |

Returns: metadata, tool breakdown, files read/written, errors, prompt list, cost/tokens.

### 4. project-activity

Activity tree showing submodule hierarchy.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| project_name | string | **yes** | Project name |
| date_range | string | no | Date range (default "last 7 days") |

Classifies sessions by submodule using `cwd` and `files_read`/`files_written` paths.
Returns: ASCII tree grouped by day -> submodule -> session.

### 5. search-events

Full-text search across all hook event data.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| query | string | **yes** | Search query (searches data_flat, prompt, error_message, etc.) |
| project_name | string | no | Filter by project |
| session_id | string | no | Session ID or prefix |
| hook_type | string | no | PreToolUse, PostToolUse, Stop, etc. |
| tool_name | string | no | Read, Edit, Bash, Grep, etc. |
| date_range | string | no | Date range |
| limit | int | no | Max results (default 20, max 100) |

Returns: event list with type, timestamp, tool, session, relevant snippet.

### 6. error-analysis

Analyze PostToolUseFailure events.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| project_name | string | no | Filter by project |
| session_id | string | no | Session ID or prefix |
| date_range | string | no | Date range |
| limit | int | no | Max results (default 50) |

Returns: error frequency by tool, common error patterns, recent errors list.

### 7. cost-analysis

Cost and token usage analysis.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| project_name | string | no | Filter by project |
| date_range | string | no | Date range (default "last 7 days") |
| group_by | string | no | day (default), session, model |

Returns: totals, averages, grouped breakdown, most expensive sessions.

### 8. tool-usage

Tool distribution and file access patterns.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| project_name | string | no | Filter by project |
| session_id | string | no | Session ID or prefix |
| date_range | string | no | Date range |

Returns: tool distribution with bar chart, most-read files, most-written files.

## Tool Registration Pattern

All tools use the official SDK's generic `mcp.AddTool[In, Out]` with typed input structs:

```go
// tools/register.go
func RegisterAll(server *mcp.Server, searcher meili.Searcher) {
    mcp.AddTool(server, &mcp.Tool{
        Name:        "query-sessions",
        Description: "List Claude sessions filtered by project, date, model...",
    }, makeQuerySessions(searcher))
    // ... 7 more tools
}

// tools/sessions.go
type QuerySessionsInput struct {
    ProjectName string `json:"project_name,omitempty" jsonschema:"filter by project name"`
    DateRange   string `json:"date_range,omitempty"   jsonschema:"today, last 3 days, 2026-02-28, etc."`
    // ...
}

func makeQuerySessions(s meili.Searcher) mcp.ToolHandlerFor[QuerySessionsInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, in QuerySessionsInput) (*mcp.CallToolResult, any, error) {
        // parse dates, build filter, search, format
        return &mcp.CallToolResult{
            Content: []mcp.Content{&mcp.TextContent{Text: result}},
        }, nil, nil
    }
}
```

Output type is `any` (no output schema needed — tools return formatted text in CallToolResult).

## Build and Installation

```makefile
build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/hooks-mcp ./cmd/hooks-mcp

install: build
	cp bin/hooks-mcp ~/.local/bin/hooks-mcp
```

Register with Claude Code:
```bash
claude mcp add --transport stdio --scope project hooks-mcp -- hooks-mcp
```

Or commit `.mcp.json` to the repo:
```json
{
  "mcpServers": {
    "hooks-mcp": {
      "command": "hooks-mcp",
      "env": {
        "MEILI_URL": "${MEILI_URL:-http://localhost:7700}"
      }
    }
  }
}
```

## Implementation Order

1. **Skeleton** — go.mod, main.go (env config, meili health check, empty server, stdio). Verify: starts and connects.
2. **dateparse** — full test suite, no deps. Verify: all date formats.
3. **format** — full test suite, no deps. Verify: tables, trees, durations.
4. **meili client** — Searcher interface + MeiliClient implementation. Verify: searches return typed hits.
5. **resolve** — session prefix resolution. Verify: prefix lookup works.
6. **query-sessions** — first end-to-end tool. Verify: `claude mcp add` + invoke from Claude Code.
7. **session-summary** — multi-index queries. Verify: detailed output for known session.
8. **query-prompts** — prompts index. Verify: chronological grouped output.
9. **search-events** — full-text search. Verify: finds known content.
10. **project-activity** — submodule classification + tree. Verify: tree matches manual curl results.
11. **error-analysis, cost-analysis, tool-usage** — remaining tools.
12. **Makefile, .mcp.json, CLAUDE.md files** — polish.

## Key Reference Files

- `hooks-store/internal/store/store.go` — Document types (SessionDocument, PromptDocument, Document) that define the MeiliSearch schemas. Hit types must mirror these field names exactly.
- `hooks-store/internal/store/meili.go` — MeiliSearch client patterns (meilisearch-go SDK usage, index access, search construction).
- `docs/meilisearch-query-guide.md` — Full schema reference, query patterns, gotchas.
- `docs/meilisearch-integration-strategy.md` — Architectural context for why this MCP server exists.

## Verification

1. `make build` succeeds
2. `make test` — all unit tests pass (dateparse, format, resolve with mocks)
3. Start MeiliSearch + hooks-mcp, run `claude mcp add --transport stdio hooks-mcp -- ./bin/hooks-mcp`
4. In Claude Code: `/mcp` shows hooks-mcp connected with 8 tools
5. Invoke each tool and verify output matches what raw curl queries produce
6. Compare `query-sessions` output against the session list from today's earlier curl queries
7. Verify `session-summary af7deb64` resolves the prefix and returns detailed output
