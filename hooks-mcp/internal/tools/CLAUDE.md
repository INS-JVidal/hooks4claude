# tools — MCP tool handlers

## Registration

```go
func RegisterAll(server *mcp.Server, searcher meili.Searcher)
```

Registers all 11 tools on the MCP server.

## Tools

| File | Tool | Required Params | Description |
|------|------|-----------------|-------------|
| sessions.go | query-sessions | — | List sessions (project, date, model filters) |
| prompts.go | query-prompts | — | Chronological prompts grouped by session |
| summary.go | session-summary | session_id | Detailed session overview (multi-collection) |
| activity.go | project-activity | project_name | Activity tree by day → submodule → session |
| events.go | search-events | query | Full-text search across all events |
| errors.go | error-analysis | — | PostToolUseFailure analysis |
| costs.go | cost-analysis | — | Cost/token usage analysis |
| toolstats.go | tool-usage | — | Tool distribution + file access patterns |
| semantic.go | semantic-search | query | Dense vector search for similar events/prompts |
| recall.go | recall-context | query | Hybrid search with session context grouping |
| similar.go | similar-sessions | session_id | Find sessions with similar prompt embeddings |

## Pattern

Each tool follows: typed input struct → parse dates → resolve session prefix → search → format → return `CallToolResult` with text content. Errors from Milvus are returned as `IsError: true` results (not Go errors), keeping the MCP connection alive.

Helpers in `helpers.go`: `textResult(text)`, `errResult(msg)`.

Dependencies: `internal/meili`, `internal/milvus`, `internal/dateparse`, `internal/format`, `go-sdk/mcp`.
