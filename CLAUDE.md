# hooks4claude — Hook monitoring and storage system for Claude Code

Three independent Go programs that form a pipeline:
1. **hooks-monitor** — Receives hook events from Claude Code via hooks-client, displays in console/TUI, optionally forwards to hooks-store
2. **hooks-store** — Ingests events via HTTP, indexes into Milvus for vector + scalar search
3. **hooks-mcp** — MCP server exposing 11 tools (8 scalar + 3 vector-powered) wrapping Milvus queries on hook data

Plus a Rust embedding sidecar:
4. **embed-svc** — HTTP microservice for 384-dim text embeddings (all-MiniLM-L6-v2 via ONNX Runtime)

Each Go program is a separate module with its own go.mod and binary. All code is inlined in this single repo (no submodules).

## Repository Structure

```
hooks4claude/
├── hooks-monitor/            ← hook event monitor
├── hooks-client/             ← per-event binary + daemon mode
├── hook-shim/               ← Rust shim (low-latency per-event binary)
├── hooks-mcp/               ← MCP server for Milvus hook queries
├── hooks-store/             ← HTTP + UDS ingest → Milvus indexer
├── embed-svc/               ← Rust embedding microservice (ONNX)
├── slash-commands/           ← Claude slash command .md files
├── docker/                  ← Docker Compose for Milvus (etcd + minio + milvus)
│   ├── docker-compose.yml
│   ├── setup.sh
│   └── .env.example
├── meilisearch/             ← Legacy MeiliSearch setup (deprecated)
├── shared/                  ← shared Go packages (config, hookevt, filecache, uds)
├── scripts/                 ← install.sh, setup.sh, hook_monitor.conf
├── plans/, research/        ← planning and research docs
├── CLAUDE.md, QUICKSTART.md
└── Makefile
```

## Pipeline

hooks-store is the **always-on primary receiver**. hooks-monitor is an **optional subscriber** that receives events from hooks-store's pub socket.

### Primary Pipeline
```
Hook fires ──→ hook-shim (Rust) ──UDS──→ hook-client daemon ──UDS──→ hooks-store
               (stdin, exit 0)           (validates, enriches)        ├──→ Milvus (always)
                                                                      ├──→ File cache (always)
                                                                      └──→ UDS pub socket ──→ hooks-monitor (optional)
                                                                                            ──→ future consumers
                                                                            ↕
                                                                      embed-svc (ONNX embeddings)
```

Wire protocol: `[type:1][len:4][json:len]` — see `shared/uds/`. Message types: `0x01` event, `0x02` cache query, `0x03` cache response, `0x04` subscribe.

```
Claude Code ──→ hooks-mcp (MCP stdio) ──→ Milvus queries + embed-svc (for semantic search)
               (11 tools: 8 scalar + 3 vector)
```

hooks-client is a **separate binary** because Claude Code spawns it per-event and expects exit 0 immediately. The Rust hook-shim provides lower startup latency by delegating to the persistent hook-client daemon.

### Key Design Invariants
- **Index-then-broadcast**: pub broadcast only after successful Milvus Index()
- **Async broadcast**: never blocks ingest handlers
- **File cache in hooks-store only**: single authoritative owner
- **Per-subscriber write mutex**: prevents interleaved framed writes

### Transport Configuration
- Shim vs client: `hook-client install-hooks [--shim]` (registers in `settings.json`)
- UDS ingest: env `HOOKS_STORE_SOCK` (hook-client daemon connects here)
- UDS pub: env `HOOKS_STORE_PUB_SOCK` (monitor subscribes here)
- Legacy: `HOOK_MONITOR_SOCK` still supported as fallback

## Why Two HookEvent Types

Each module defines its own `HookEvent` to avoid a shared dependency; the JSON wire format is the contract.

## Quick Start

```bash
# Milvus (requires Docker)
cd docker && docker compose up -d

# Embedding service (Terminal 1) — optional but enables vector search
cd embed-svc && make download-model && make run

# Store (Terminal 2) — always-on primary receiver, requires Milvus on :19530
cd hooks-store && make run

# Monitor (Terminal 3) — optional viewer, subscribes to store's pub socket
HOOKS_STORE_PUB_SOCK=/tmp/hooks-store-pub.sock cd hooks-monitor && make run-ui

# MCP Server — register with Claude Code (requires Milvus)
cd hooks-mcp && make install
claude mcp add --transport stdio --scope project hooks-mcp -- hooks-mcp
```

## HTTP API

| Program | Endpoint | Method | Description |
|---------|----------|--------|-------------|
| monitor | `/hook/<Type>` | POST | Receive a hook event (type in URL path) |
| monitor | `/stats` | GET | Event counts by hook type + dropped count |
| monitor | `/events` | GET | Recent events (ring buffer, ?limit=N) |
| monitor | `/health` | GET | Health check (200 OK) |
| store | `/ingest` | POST | Receive and index a hook event |
| store | `/health` | GET | Health check |
| store | `/stats` | GET | Ingest statistics |
| embed-svc | `/embed` | POST | Generate embeddings: `{"texts": [...]}` → `{"embeddings": [[...], ...]}` |
| embed-svc | `/health` | GET | Readiness check |

## Configuration Reference

### hooks-monitor

| Source | Keys |
|--------|------|
| Env vars | `PORT`, `HOOK_MONITOR_TOKEN`, `HOOK_MONITOR_URL`, `PORT_FILE`, `HOOK_MONITOR_CONFIG`, `HOOK_MONITOR_SOCK`, `HOOKS_STORE_PUB_SOCK` |
| Config file | `hook_monitor.conf` — `[hooks]` per-type toggles |
| XDG path | `~/.config/hooks-monitor/` |
| CLI flags | `--ui` (TUI mode), `--version` |

### hooks-store

| Source | Keys |
|--------|------|
| CLI flags | `--port`, `--milvus-url`, `--milvus-token`, `--events-col`, `--prompts-col`, `--sessions-col`, `--embed-url`, `--uds-sock`, `--pub-sock`, `--no-cache` |
| Env vars | `HOOKS_STORE_PORT`, `HOOKS_STORE_SOCK`, `HOOKS_STORE_PUB_SOCK`, `MILVUS_URL`, `MILVUS_TOKEN`, `MILVUS_EVENTS_COL`, `MILVUS_PROMPTS_COL`, `MILVUS_SESSIONS_COL`, `EMBED_SVC_URL` |

### hooks-mcp

| Source | Keys |
|--------|------|
| Env vars | `MILVUS_URL` (default localhost:19530), `MILVUS_TOKEN`, `EVENTS_COLLECTION` (hook_events), `PROMPTS_COLLECTION` (hook_prompts), `SESSIONS_COLLECTION` (hook_sessions), `EMBED_SVC_URL` (default localhost:8900) |
| No CLI flags | stdio MCP servers read env only |

### embed-svc

| Source | Keys |
|--------|------|
| Env vars | `EMBED_SVC_PORT` (default 8900), `EMBED_MODEL_PATH`, `EMBED_TOKENIZER_PATH` |

### hook-client daemon

| Source | Keys |
|--------|------|
| Env vars | `HOOK_CLIENT_SOCK` (default `/tmp/hook-client.sock`), `HOOK_MONITOR_SOCK`, `HOOKS_STORE_SOCK` |
| Subcommand | `hook-client daemon` |

### hook-shim

| Source | Keys |
|--------|------|
| Env vars | `HOOK_CLIENT_SOCK` (default `/tmp/hook-client.sock`) |

## File Architecture Map

### hooks-store packages

hooks-store/cmd/hooks-store/ — Entry point. Flags, Milvus connect, TUI startup.

hooks-store/internal/hookevt/ — Wire format HookEvent struct. Key types: HookEvent.
hooks-store/internal/ingest/ — HTTP + UDS ingest server. Key types: Server, UDSIngestServer, IngestEvent. Key funcs: New, NewUDS, SetOnIngest, ErrCount.
hooks-store/internal/pubsub/ — UDS pub server for event subscribers. Key types: PubServer. Key funcs: New, Serve, Broadcast, Close.
hooks-store/internal/store/ — Milvus storage layer. Key types: Document, EventStore, MilvusStore, MilvusClient, Embedder. Key funcs: NewMilvusStore, HookEventToDocument, EmbeddingText.
hooks-store/internal/tui/ — Bubble Tea dashboard. Key types: Model, Config. Key funcs: NewModel, Run.

### hooks-monitor packages

hooks-monitor/cmd/monitor/ — Entry point. Console + TUI modes, signal handling.

hooks-monitor/internal/hookevt/ — Shared HookEvent type (same schema as hooks-store).
hooks-monitor/internal/config/ — INI config, hook toggles, sink config. Key types: HookConfig, SinkConfig (Transport, SocketPath fields). Key funcs: ReadConfig, WriteConfig, ReadSinkConfig.
hooks-monitor/internal/monitor/ — Core ring buffer (1000 events). Key types: HookMonitor. Key funcs: NewHookMonitor, AddEvent, CloseChannel.
hooks-monitor/internal/server/ — HTTP handlers + middleware. Key funcs: HandleHook, HandleStats, HandleEvents, AuthMiddleware.
hooks-monitor/internal/subscriber/ — UDS pub subscriber for receiving events from hooks-store. Key types: Subscriber. Key funcs: New, Run.
hooks-monitor/internal/udsserver/ — UDS listener for events + cache queries. Key types: UDSServer. Key funcs: New, Serve.
hooks-monitor/internal/platform/ — OS-specific lock/signals (flock on Unix, LockFileEx on Windows). Key funcs: AcquireLock, ShowRunningInstance.
hooks-monitor/internal/tui/ — Interactive tree UI. 6 files: model, tree, processor, detail, hooks_menu, styles. Key types: Model, Session, EventProcessor. Key funcs: Run, FlattenTree.

### hooks-mcp packages

hooks-mcp/cmd/hooks-mcp/ — Entry point. Env config, Milvus health check, MCP server + stdio.

hooks-mcp/internal/dateparse/ — Date range parsing ("today", "last 3 days" → DateRange). Key types: DateRange. Key funcs: ParseRange.
hooks-mcp/internal/format/ — Pure formatting functions. Key funcs: Table, Tree, BarChart, FormatDuration, FormatCost, FormatTokens, ShortID.
hooks-mcp/internal/meili/ — Searcher interface + hit types (shared contract). Key types: Searcher (interface), SessionHit, PromptHit, EventHit, FacetResult.
hooks-mcp/internal/milvus/ — Milvus REST API v2 client implementing Searcher. Key types: MilvusClient, Embedder. Key funcs: NewMilvusClient, NewEmbedder.
hooks-mcp/internal/tools/ — 11 MCP tool handlers. Key funcs: RegisterAll. Tools: query-sessions, query-prompts, session-summary, project-activity, search-events, error-analysis, cost-analysis, tool-usage, semantic-search, recall-context, similar-sessions.

### shared packages

shared/config/ — INI config, hook toggles, sink config (Transport, SocketPath). Key types: SinkConfig, HookConfig, CacheConfig.
shared/hookevt/ — Wire format HookEvent struct.
shared/filecache/ — Session-scoped file read tracking. Key types: SessionFileCache, CacheQuery.
shared/uds/ — UDS wire protocol. Key consts: MsgEvent, MsgCacheQuery, MsgCacheResponse. Key funcs: WriteMsg, ReadMsg, Listen, Dial, SocketPath.

### embed-svc (Rust)

embed-svc/src/main.rs — Rust binary. Loads all-MiniLM-L6-v2 ONNX model, serves POST /embed + GET /health on port 8900.

### hook-shim (Rust)

hook-shim/src/main.rs — Rust binary (~80 lines). Reads stdin, connects to hook-client daemon UDS, sends framed event or cache query.

### hooks-client

hooks-client/main.go — Per-event handler + `install-hooks` + `daemon` subcommand dispatch.
hooks-client/daemon.go — Daemon mode: UDS listener, validates/enriches events, forwards to monitor or store via UDS.

## Milvus Collections

Three collections in Milvus (replacing 3 MeiliSearch indexes):

| Collection | Vector Fields | Purpose |
|------------|--------------|---------|
| hook_events | dense_embedding (FloatVector 384) | All hook events with semantic search |
| hook_prompts | prompt_dense (FloatVector 384) | User prompts with semantic search |
| hook_sessions | dummy_vector (FloatVector 2) | Session aggregates (scalar only) |

All collections use AUTOINDEX with COSINE metric. Documents without embeddings have `dense_valid = false` and zero vectors.

## Active Files (read source before editing, don't rely on summaries)

No files currently under active development.

## CLAUDE.md Maintenance

Every directory has a CLAUDE.md with package summaries. Prefer summaries over re-reading stable source files for context. When you need to edit a file, always read the actual source. After changing a package's exported API, update its CLAUDE.md.
