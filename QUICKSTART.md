# Quickstart — Running the Full Hook Pipeline

This guide walks you through launching the complete system:
**Claude hooks → hooks-client → monitor → sink → hooks-store → Milvus**

## Clone

```bash
git clone https://github.com/INS-JVidal/hooks4claude.git
cd hooks4claude
```

## Prerequisites

| Component | Status check |
|-----------|-------------|
| Go 1.25+  | `go version` |
| Docker + Compose | `docker compose version` |
| Rust (for embed-svc) | `rustc --version` |
| curl + jq | `which curl jq` |

## Architecture Overview

```
Claude Code session
  │
  ├─ hook events (stdin JSON)
  │
  ▼
hooks-client  (binary, runs per-event)
  │
  ├─ POST /hook/<HookType>
  │
  ▼
hooks-monitor  (long-running, port 8080)
  │
  ├─ TUI display + /stats /events /health API
  │
  ├─ fire-and-forget goroutine (if sink enabled)
  │   POST /ingest
  │   ▼
  hooks-store  (long-running, port 9800)
  │
  ├─ embed via embed-svc (port 8900) + index
  │   ▼
  Milvus  (port 19530, via Docker)
```

---

## Step 1 — Start Milvus (Docker)

```bash
cd docker
docker compose up -d
```

Wait for healthy status:

```bash
docker compose ps
# All 3 services (etcd, minio, milvus) should show "healthy"
```

Verify Milvus is responding:

```bash
curl -s http://localhost:19530/v2/vectordb/collections/list | jq .
# Expected: {"code":0,"data":[]}
```

Milvus WebUI is available at http://localhost:9091/webui/

### Attu (Milvus GUI)

```bash
docker run --network host -e MILVUS_URL=127.0.0.1:19530 zilliz/attu:v2.6
```

Open http://localhost:3000 — login with username `root`, leave password empty.

Or start it with the full stack: `docker compose -f docker/docker-compose.yml up -d`

## Step 2 — Start Embedding Service (Optional)

The embedding service enables semantic/vector search. Without it, hooks-store falls back to zero vectors (scalar search still works).

```bash
cd embed-svc

# Download the ONNX model (one-time, ~90MB)
make download-model

# Build and run
make run
```

Verify:

```bash
curl -s http://localhost:8900/health | jq .
# Expected: {"status":"ok","model":"all-MiniLM-L6-v2","dimensions":384}
```

## Step 3 — Build Binaries

```bash
# From the project root
cd hooks-monitor && make build && cd ..
cd hooks-store && make build && cd ..
cd hooks-mcp && make build && cd ..
```

## Step 4 — Start hooks-store

In a **dedicated terminal**:

```bash
cd hooks-store
./bin/hooks-store
# Or with custom settings:
# ./bin/hooks-store --milvus-url http://localhost:19530 --embed-url http://localhost:8900
```

You should see:

```
Connecting to Milvus at http://localhost:19530...
```

Verify:

```bash
curl -s http://localhost:9800/health | jq .
```

## Step 5 — Enable Sink Forwarding in Monitor Config

Edit `~/.config/hooks-monitor/hook_monitor.conf` and add the `[sink]` section:

```ini
[hooks]
SessionStart = yes
UserPromptSubmit = yes
...existing hooks...

[sink]
forward = yes
endpoint = http://localhost:9800/ingest
```

**Important**: Set `forward = yes` to enable the pipeline.

## Step 6 — Start the Monitor

In another **dedicated terminal**:

```bash
cd hooks-monitor

# Console mode (see events scroll by):
make run

# Or interactive TUI mode:
make run-ui
```

## Step 7 — Register MCP Server (Optional)

For querying hook data from within Claude Code:

```bash
cd hooks-mcp && make install
claude mcp add --transport stdio --scope project hooks-mcp -- hooks-mcp
```

Set environment variables if using non-default ports:

```bash
export MILVUS_URL=http://localhost:19530
export EMBED_SVC_URL=http://localhost:8900
```

## Step 8 — Test the Pipeline

### Manual test (without Claude):

```bash
# Send a test event directly to the monitor
cd hooks-monitor && make send-test-hook

# Check it arrived at hooks-store
cd ../hooks-store && curl -s http://localhost:9800/stats | jq .
```

### Live test (with Claude):

Simply start a new Claude Code session in any project.
The hooks fire automatically — every tool use, prompt submit, session start, etc.
Watch the monitor terminal for live events scrolling by.

---

## Terminal Layout (Recommended)

```
┌──────────────────────┬──────────────────────┐
│                      │                      │
│   Milvus (Docker)    │   hooks-store        │
│   (port 19530)       │   (port 9800)        │
│                      │                      │
├──────────────────────┼──────────────────────┤
│                      │                      │
│   monitor (TUI)      │   Claude Code        │
│   (port 8080)        │   (your session)     │
│                      │                      │
└──────────────────────┴──────────────────────┘
```

## Quick Reference — All Ports

| Service        | Port  | Purpose |
|---------------|-------|---------|
| Milvus        | 19530 | Vector + scalar database |
| Milvus WebUI  | 9091  | Milvus dashboard |
| Attu          | 3000  | Milvus GUI (root, no password) |
| MinIO Console | 9001  | Object storage dashboard |
| embed-svc     | 8900  | Text embeddings (ONNX) |
| Monitor       | 8080  | Hook event receiver + TUI |
| hooks-store   | 9800  | Ingest → Milvus bridge |

## Quick Reference — Health Checks

```bash
curl -s http://localhost:19530/v2/vectordb/collections/list | jq .  # Milvus
curl -s http://localhost:8900/health | jq .                         # embed-svc
curl -s http://localhost:8080/health | jq .                         # Monitor
curl -s http://localhost:9800/health | jq .                         # hooks-store
```

## Activating BM25 Full-Text Search

Milvus v2.5.6+ supports built-in BM25 functions that auto-generate sparse vectors from text fields. This replaces slow LIKE queries with proper relevance-ranked full-text search and enables native hybrid search (dense + sparse + RRF) in a single Milvus call.

### What changes

- **hook_events**: `data_flat` gets an English analyzer; a new `sparse_embedding` SparseFloatVector field is auto-populated by a BM25 function on insert.
- **hook_prompts**: `prompt` gets the same treatment.
- **hook_sessions**: unchanged.
- **search-events** tool: uses BM25 instead of LIKE for keyword search.
- **recall-context** tool: uses native Milvus `hybrid_search` (dense + sparse + RRF) instead of client-side RRF.

### Migration steps

BM25 functions cannot be added to existing collections — they must be recreated.

**1. Stop hooks-store** if running.

**2. Rebuild hooks-store** (picks up new schema definitions):

```bash
cd hooks-store && make build
```

**3. Recreate collections** with the `--recreate-collections` flag:

```bash
./bin/hooks-store --recreate-collections
```

This drops `hook_events`, `hook_prompts`, and `hook_sessions`, then recreates them with the new schema (analyzer-enabled text fields, sparse vector fields, BM25 functions, and sparse indexes).

> **Warning**: This deletes all existing data in those collections. If you need to preserve data, export it first via Attu or the Milvus backup tool.

**4. Verify in Attu** (http://localhost:3000):

- Open each collection's schema tab
- Confirm `sparse_embedding` field exists (type: SparseFloatVector)
- Confirm `data_flat` (events) / `prompt` (prompts) shows `enable_analyzer: true`
- Check the Functions tab shows `bm25_events` / `bm25_prompts`

**5. Rebuild and reinstall hooks-mcp**:

```bash
cd hooks-mcp && make build && make install
```

**6. Restart hooks-store normally** (without the flag):

```bash
cd hooks-store && ./bin/hooks-store
```

**7. Ingest some events** and verify in Attu that the `sparse_embedding` column is populated (non-empty sparse vectors) for new records.

**8. Test the MCP tools**:

- `search-events` with a query — results should be ranked by BM25 relevance
- `recall-context` — should use native hybrid search (check hooks-mcp stderr for any fallback warnings)

### After migration

The `--recreate-collections` flag is only needed once for the schema migration. Subsequent starts use the normal command without the flag. New events automatically get sparse vectors generated by Milvus — no changes to embed-svc or the ingest pipeline are needed.

---

## Troubleshooting

**hooks-store fails to start**: Milvus must be running first. Run `cd docker && docker compose up -d`.

**Events not reaching Milvus**: Check `forward = yes` in the `[sink]` section
of `~/.config/hooks-monitor/hook_monitor.conf`. The monitor must be restarted
after changing this setting.

**Semantic search returns no results**: Ensure embed-svc is running and events have `dense_valid = true`. Events ingested without embed-svc get zero vectors.

**hook-client not found**: Ensure `~/.local/bin` is in your PATH.

**Port already in use**: Another instance may be running. Check with:
`lsof -i:8080` / `lsof -i:9800` / `lsof -i:19530`

**BM25 search returns errors**: Collections were created before the BM25 migration. Run `hooks-store --recreate-collections` once to recreate them with the new schema (destroys existing data).

**Milvus version too old for BM25**: BM25 built-in functions require Milvus v2.5.6+. Check with `curl -s http://localhost:19530/v2/vectordb/collections/list` — the response includes the server version. Update via `docker compose pull && docker compose up -d` in the `docker/` directory.
