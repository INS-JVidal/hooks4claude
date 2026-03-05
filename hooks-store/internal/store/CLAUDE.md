# store — Milvus storage layer for hook event documents

## store.go

```go
type Document struct {
    ID, HookType, Timestamp, SessionID, ToolName string
    TimestampUnix, InputTokens, OutputTokens, CacheReadTokens, CacheCreateTokens int64
    CostUSD float64
    HasClaudeMD, DenseValid bool
    Prompt, FilePath, ErrorMessage, ProjectDir, ProjectName, PermissionMode, Cwd string
    DataFlat, DataJSON string
    DenseEmbedding []float32
    Data map[string]interface{}
}

type PromptDocument struct {
    ID, HookType, Timestamp, SessionID, Prompt, Cwd, ProjectDir, PermissionMode string
    TimestampUnix int64
    PromptLength int
    HasClaudeMD, DenseValid bool
    PromptDense []float32
}

type EventStore interface {
    Index(ctx context.Context, doc Document) error
    Close() error
}
```

## milvus.go

```go
type MilvusStore struct { /* client, embedder, collection names */ }
func NewMilvusStore(milvusURL, milvusToken, eventsCol, promptsCol, sessionsCol, embedURL string) (*MilvusStore, error)
func (s *MilvusStore) Index(ctx context.Context, doc Document) error
func (s *MilvusStore) Close() error
func EmbeddingText(doc Document) string
func DataToJSON(data map[string]interface{}) string
```

MilvusStore implements EventStore. NewMilvusStore creates MilvusClient + Embedder, ensures collections exist with correct schemas (hook_events, hook_prompts, hook_sessions). Thread-safe.

Index() generates embedding via embed-svc (fail-soft: zero vector + dense_valid=false), truncates fields to VarChar limits, inserts into Milvus. Dual-writes UserPromptSubmit events to prompts collection.

Collections: hook_events (FloatVector 384 + scalars), hook_prompts (FloatVector 384 + scalars), hook_sessions (scalar only, dummy FloatVector 2).

## milvus_client.go

Thin HTTP client for Milvus REST API v2. Methods: HasCollection, CreateCollection, Insert, Upsert, Query, Search, HybridSearch. Exponential backoff retry (3 attempts, 1s→2s→4s).

## embedder.go

HTTP client for embed-svc. Circuit breaker: 3 consecutive failures → skip for 30s. Returns nil on circuit open (caller uses zero vector).

## transform.go

HookEventToDocument, DocumentToPromptDocument, extractStringValues, extractTokenMetrics, and field extraction helpers. Unchanged from MeiliSearch version.

## meili.go (build-tagged)

Original MeiliSearch backend, excluded from default build via `//go:build meili`.
