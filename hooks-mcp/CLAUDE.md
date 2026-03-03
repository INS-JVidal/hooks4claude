# hooks-mcp — MCP Server for MeiliSearch Hook Data

Go binary exposing 8 read-only MCP tools wrapping MeiliSearch queries on Claude Code hook event data. Connects via stdio transport.

## Build & Test

```bash
make build          # → bin/hooks-mcp
make test           # go test ./...
make install        # cp to ~/.local/bin/hooks-mcp
```

## Configuration (env vars only)

| Env Var | Default | Description |
|---------|---------|-------------|
| `MEILI_URL` | `http://localhost:7700` | MeiliSearch endpoint |
| `MEILI_KEY` | `""` | MeiliSearch API key |
| `MEILI_INDEX` | `hook-events` | Events index name |
| `PROMPTS_INDEX` | `hook-prompts` | Prompts index name |
| `SESSIONS_INDEX` | `hook-sessions` | Sessions index name |

## Registration

```bash
claude mcp add --transport stdio --scope project hooks-mcp -- hooks-mcp
```

## Tools (8)

| Tool | Description |
|------|-------------|
| `query-sessions` | List sessions filtered by project, date, model |
| `query-prompts` | Get user prompts chronologically, grouped by session |
| `session-summary` | Detailed overview of one session |
| `project-activity` | Activity tree grouped by day and submodule |
| `search-events` | Full-text search across all hook events |
| `error-analysis` | Analyze PostToolUseFailure events |
| `cost-analysis` | Cost and token usage analysis |
| `tool-usage` | Tool distribution and file access patterns |

## Architecture

```
cmd/hooks-mcp/main.go  ← env config, meili health check, MCP server + stdio
internal/dateparse/     ← "today", "last 3 days" → DateRange (unix + ISO)
internal/format/        ← Table, Tree, BarChart, FormatDuration, FormatCost, etc.
internal/meili/         ← Searcher interface + MeiliClient (typed search wrappers)
internal/tools/         ← 8 MCP tool handlers + RegisterAll wiring
```

## Constraints

- **stdout is the MCP protocol channel.** All logging goes to stderr.
- **MeiliSearch may be down.** Tools return MCP error results (not crash).
- **No imports from hooks-store or monitor.** Independent types, same JSON schema.
