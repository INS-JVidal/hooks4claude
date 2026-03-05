# hooks-mcp — MCP Server for Milvus Hook Data

Go binary exposing 11 MCP tools (8 scalar + 3 vector-powered) wrapping Milvus queries on Claude Code hook event data. Connects via stdio transport.

## Build & Test

```bash
make build          # → bin/hooks-mcp
make test           # go test ./...
make install        # cp to ~/.local/bin/hooks-mcp
```

## Configuration (env vars only)

| Env Var | Default | Description |
|---------|---------|-------------|
| `MILVUS_URL` | `http://localhost:19530` | Milvus endpoint |
| `MILVUS_TOKEN` | `""` | Milvus API token |
| `EMBED_SVC_URL` | `http://localhost:8900` | Embedding service URL |
| `EVENTS_COLLECTION` | `hook_events` | Events collection name |
| `PROMPTS_COLLECTION` | `hook_prompts` | Prompts collection name |
| `SESSIONS_COLLECTION` | `hook_sessions` | Sessions collection name |

## Registration

```bash
claude mcp add --transport stdio --scope project hooks-mcp -- hooks-mcp
```

## Tools (11)

### Scalar tools (8)
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

### Vector-powered tools (3, require embed-svc)
| Tool | Description |
|------|-------------|
| `semantic-search` | Find events/prompts by meaning (dense vector similarity) |
| `recall-context` | Hybrid keyword + semantic search with session context grouping |
| `similar-sessions` | Find sessions with similar work patterns via prompt embeddings |

## Architecture

```
cmd/hooks-mcp/main.go  ← env config, Milvus health check, MCP server + stdio
internal/dateparse/     ← "today", "last 3 days" → DateRange (unix + ISO)
internal/format/        ← Table, Tree, BarChart, FormatDuration, FormatCost, etc.
internal/meili/         ← Searcher interface + hit types (shared contract)
internal/milvus/        ← MilvusClient implementing Searcher via REST API v2
internal/tools/         ← 11 MCP tool handlers + RegisterAll wiring
```

## Constraints

- **stdout is the MCP protocol channel.** All logging goes to stderr.
- **Milvus may be down.** Tools return MCP error results (not crash).
- **No imports from hooks-store or monitor.** Independent types, same JSON schema.
