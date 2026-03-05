# milvus — Milvus REST API v2 client implementing Searcher

## client.go

```go
type MilvusClient struct { /* baseURL, token, httpClient, collection names, embedder */ }
func NewMilvusClient(baseURL, token, eventsCol, promptsCol, sessionsCol string, embedder *Embedder) *MilvusClient
```

Implements meili.Searcher:
- SearchSessions: query hook_sessions, client-side sort
- SearchPrompts: query hook_prompts with optional LIKE filter on prompt
- SearchEvents: query hook_events with optional LIKE filter on data_flat
- FacetEvents: fetch up to 10000 events, aggregate field values client-side
- ResolveSessionPrefix: LIKE query on session_id
- SemanticSearchPrompts: embed query → vector search on prompt_dense
- SemanticSearchEvents: embed query → vector search on dense_embedding
- HybridSearch: falls back to semantic search (RRF TODO)

Milvus filter syntax: `==`, `&&`, `||`, `like`. Client-side sorting (Milvus query API doesn't support ORDER BY).

## embedder.go

Simple HTTP client for embed-svc. `Embed(ctx, text) ([]float32, error)`.

Dependencies: `internal/meili` (for Searcher interface and types).
