# hooks4claude — Architecture & Communication Flow

## Components

| Component              | Type            | Description                                      |
|------------------------|-----------------|--------------------------------------------------|
| **Claude Code**        | Host process    | Fires hook events on tool calls, notifications   |
| **hook-client**        | CLI binary      | Spawned per-event by Claude Code, exits 0 fast   |
| **claude-hooks-monitor** | HTTP server   | Receives events, displays TUI, forwards to store |
| **hooks-store**        | HTTP server     | Ingests events, indexes into MeiliSearch         |
| **MeiliSearch**        | Search engine   | Stores and indexes all hook event data           |
| **hooks-mcp**          | MCP server      | Read-only query tools over stdio for Claude Code |

## Communication Flow

```
                         HOOK EVENT PATH (write)
                         ======================

  ┌────────────┐    spawn + stdin JSON     ┌─────────────┐
  │            │ ─────────────────────────► │             │
  │ Claude Code│                           │ hook-client  │
  │            │ ◄── exit 0 (immediate) ── │ (per-event)  │
  └────────────┘                           └──────┬──────┘
                                                  │
                                    POST /hook/<Type>
                                    (loopback only)
                                                  │
                                                  ▼
                                    ┌─────────────────────────┐
                                    │  claude-hooks-monitor    │
                                    │                         │
                                    │  ┌───────────────────┐  │
                                    │  │ HookMonitor        │  │
                                    │  │ (ring buffer 1000) │  │
                                    │  └──────┬────────────┘  │
                                    │         │               │
                                    │    ┌────┴────┐          │
                                    │    │         │          │
                                    │    ▼         ▼          │
                                    │  Console   TUI          │
                                    │  log       eventCh      │
                                    │    │                    │
                                    │    ▼                    │
                                    │  HTTPSink               │
                                    └────┬────────────────────┘
                                         │
                                   POST /ingest
                                         │
                                         ▼
                                    ┌──────────────┐        ┌──────────────┐
                                    │              │ index  │              │
                                    │ hooks-store  │ ─────► │  MeiliSearch │
                                    │              │        │  :7700       │
                                    └──────────────┘        └──────┬───────┘
                                                                   │
                                                                   │
                         QUERY PATH (read-only)                    │
                         ======================                    │
                                                                   │
  ┌────────────┐    stdio (MCP protocol)   ┌──────────────┐        │
  │            │ ◄────────────────────────► │              │ query  │
  │ Claude Code│                           │  hooks-mcp   │ ◄─────┘
  │            │                           │  (8 tools)   │
  └────────────┘                           └──────────────┘


                         HTTP API SUMMARY
                         ================

  hook-client ──POST──► monitor:/hook/<Type>
  monitor     ──POST──► hooks-store:/ingest
  hooks-mcp   ──HTTP──► MeiliSearch:/indexes/hook-events/search
                        MeiliSearch:/indexes/hook-prompts/search
                        MeiliSearch:/indexes/hook-sessions/search

  monitor also exposes:
    GET /stats     — event counts by type
    GET /events    — recent events from ring buffer
    GET /health    — health check

  hooks-store also exposes:
    GET /stats     — ingest statistics
    GET /health    — health check
```

## Key Design Decisions

- **hook-client is a separate binary** — Claude Code spawns it per-event and expects exit 0 immediately; a long-running process would block the hook.
- **Two separate HookEvent types** — monitor and store each define their own struct to avoid a shared dependency; the JSON wire format is the contract.
- **hooks-mcp uses stdio** — MCP servers communicate over stdin/stdout, no HTTP port needed.
- **Loopback only** — hook-client only posts to localhost (the monitor).
