# meili — Searcher interface and hit types (shared contract)

Despite the package name, this is now the interface contract used by both the Milvus client and tools.

## Interface

```go
type Searcher interface {
    SearchSessions(ctx, SessionSearchOpts) ([]SessionHit, int64, error)
    SearchPrompts(ctx, PromptSearchOpts)   ([]PromptHit, int64, error)
    SearchEvents(ctx, EventSearchOpts)     ([]EventHit, int64, error)
    FacetEvents(ctx, filter string, facets []string) (*FacetResult, error)
    ResolveSessionPrefix(ctx, prefix string) (string, error)
    SemanticSearchPrompts(ctx, queryText string, opts PromptSearchOpts) ([]PromptHit, error)
    SemanticSearchEvents(ctx, queryText string, opts EventSearchOpts) ([]EventHit, error)
    HybridSearch(ctx, queryText string, opts EventSearchOpts) ([]EventHit, error)
}
```

## Types

- types.go — Searcher interface, SessionSearchOpts, PromptSearchOpts, EventSearchOpts, SessionHit, PromptHit, EventHit, FacetResult, BuildEventFilter (Milvus syntax)
- meiliclient.go — Original MeiliClient (build-tagged `//go:build meili`, excluded from default build)

Dependencies: `internal/dateparse`.
