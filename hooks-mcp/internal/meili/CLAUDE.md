# meili — Typed MeiliSearch client for hook data queries

## Interface

```go
type Searcher interface {
    SearchSessions(ctx, SessionSearchOpts) ([]SessionHit, int64, error)
    SearchPrompts(ctx, PromptSearchOpts)   ([]PromptHit, int64, error)
    SearchEvents(ctx, EventSearchOpts)     ([]EventHit, int64, error)
    FacetEvents(ctx, filter string, facets []string) (*FacetResult, error)
    ResolveSessionPrefix(ctx, prefix string) (string, error)
}
```

## Implementation

`MeiliClient` wraps `meilisearch-go` SDK. Created via `NewMeiliClient(client, eventsIdx, promptsIdx, sessionsIdx)`.

Hit types (`SessionHit`, `PromptHit`, `EventHit`) mirror the MeiliSearch document schemas from hooks-store but are independent types — no import from hooks-store.

**ResolveSessionPrefix**: if 36 chars return as-is; otherwise search sessions index, confirm prefix match client-side (MeiliSearch is fuzzy), error on 0 or 2+ matches.

Filter builders (`buildSessionFilter`, `buildPromptFilter`, `buildEventFilter`) handle quoting and timestamp ranges. Sessions filter on `started_at` (ISO string); events/prompts filter on `timestamp_unix` (int64).

`BuildEventFilter` is exported for tools that need custom filter construction (e.g., tool-usage uses facets with a pre-built filter).

Dependencies: `meilisearch-go`, `internal/dateparse`.
