# hooks-store — Milvus companion for Claude Hooks Monitor

**Requires: Milvus running on :19530** (default; configurable via --milvus-url / MILVUS_URL).
**Optional: embed-svc on :8900** for dense vector embeddings (configurable via --embed-url / EMBED_SVC_URL).

Go module: `hooks-store`. Receives hook events via HTTP POST /ingest, transforms, generates embeddings, and indexes them into Milvus for vector + scalar search.

## Build & Test

```bash
make build          # → bin/hooks-store
make test           # go test ./...
make run            # build + run
make send-test-hook # curl a test event
```

## Key Files
- cmd/hooks-store/main.go — Entry point, flag parsing, wiring
- internal/ingest/server.go — HTTP server, IngestEvent callback, validation
- internal/store/milvus.go — Milvus client, collection setup, Index()
- internal/store/milvus_client.go — Thin Milvus REST API v2 HTTP client with retry
- internal/store/embedder.go — embed-svc HTTP client with circuit breaker
- internal/store/transform.go — HookEvent → Document conversion
- internal/tui/model.go — Bubble Tea dashboard (alt screen, live event stats)

## Configuration
- Flags: --port, --milvus-url, --milvus-token, --events-col, --prompts-col, --sessions-col, --embed-url, --uds-sock
- Env: HOOKS_STORE_PORT, MILVUS_URL, MILVUS_TOKEN, MILVUS_EVENTS_COL, MILVUS_PROMPTS_COL, MILVUS_SESSIONS_COL, EMBED_SVC_URL

## Architecture
```
POST /ingest → ingest.Server → store.HookEventToDocument → Embedder.Embed → MilvusStore.Index
                    ↓ (callback)                                                    ↓
              eventCh → tui.Model                                        Milvus REST API v2
```

## Legacy
- internal/store/meili.go — Original MeiliSearch backend (build-tagged `//go:build meili`, excluded from default build)
