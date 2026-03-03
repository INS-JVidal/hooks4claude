# hooks4claude — Hook monitoring and storage system for Claude Code

Three independent Go programs that form a pipeline:
1. **claude-hooks-monitor** — Receives hook events from Claude Code via hook-client, displays in console/TUI, optionally forwards to hooks-store
2. **hooks-store** — Ingests events via HTTP, indexes into MeiliSearch for search and filtering
3. **hooks-mcp** — MCP server exposing 8 read-only tools wrapping MeiliSearch queries on hook data

Each is a separate Go module with its own go.mod, git repo, and binary.

## Repository Structure

This is a **parent git repo** (`INS-JVidal/hooks4claude`) that uses **git submodules** to tie together the two sub-projects:

```
hooks4claude/                    ← parent repo (INS-JVidal/hooks4claude)
├── CLAUDE.md, QUICKSTART.md     ← tracked by parent
├── setup.sh, docs/, plans/      ← tracked by parent
├── claude-hooks-monitor/        ← submodule (INS-JVidal/claude-hooks-monitor, branch: main)
├── hooks-store/                 ← submodule (jvidaldamm3/hooks-store, branch: master)
└── hooks-mcp/                   ← MCP server for MeiliSearch hook queries (local, not yet a submodule)
```

Clone everything: `git clone --recurse-submodules https://github.com/INS-JVidal/hooks4claude.git`

Update submodule pointers after pulling new commits inside a submodule: `git add <submodule> && git commit`

## Pipeline

```
Claude Code                hooks-client               monitor                    hooks-store
───────────                ────────────               ───────                    ───────────
Hook fires ──→ hook-client ──→ POST /hook/<Type> ──→ HookMonitor ──→ HTTPSink ──→ POST /ingest ──→ MeiliSearch
               (stdin JSON,    (loopback only)        ├─ console log                                (search/filter)
                exit 0)                               └─ TUI eventCh                                     ↑
                                                                                                         │
Claude Code ──→ hooks-mcp (MCP stdio) ──→ MeiliSearch queries ──────────────────────────────────────────┘
               (8 read-only tools)
```

hook-client is a **separate binary** because Claude Code spawns it per-event and expects exit 0 immediately.

## Why Two HookEvent Types

Each module defines its own `HookEvent` to avoid a shared dependency; the JSON wire format is the contract.

## Quick Start

```bash
# Monitor (Terminal 1)
cd claude-hooks-monitor && make run-ui

# Store (Terminal 2) — requires MeiliSearch running on :7700
cd hooks-store && make run

# MCP Server — register with Claude Code (requires MeiliSearch)
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

## Configuration Reference

### claude-hooks-monitor

| Source | Keys |
|--------|------|
| Env vars | `PORT`, `HOOK_MONITOR_TOKEN`, `HOOK_MONITOR_URL`, `PORT_FILE`, `HOOK_MONITOR_CONFIG` |
| Config file | `hook_monitor.conf` — `[hooks]` per-type toggles (fail-open: missing = enabled), `[sink]` forwarding URL/token |
| XDG path | `~/.config/claude-hooks-monitor/` |
| CLI flags | `--ui` (TUI mode), `--version` |

### hooks-store

| Source | Keys |
|--------|------|
| CLI flags | `--port`, `--meili-url`, `--meili-key`, `--meili-index`, `--prompts-index`, `--sessions-index` |
| Env vars | `HOOKS_STORE_PORT`, `MEILI_URL`, `MEILI_KEY`, `MEILI_INDEX`, `PROMPTS_INDEX`, `SESSIONS_INDEX` |
| Config file | `hooks-store.conf` (lowest priority) |

### hooks-mcp

| Source | Keys |
|--------|------|
| Env vars | `MEILI_URL` (default localhost:7700), `MEILI_KEY`, `MEILI_INDEX` (hook-events), `PROMPTS_INDEX` (hook-prompts), `SESSIONS_INDEX` (hook-sessions) |
| No CLI flags | stdio MCP servers read env only |

## File Architecture Map

### hooks-store packages

hooks-store/cmd/hooks-store/ — Entry point. Flags, MeiliSearch connect, TUI startup.

hooks-store/internal/hookevt/ — Wire format HookEvent struct. Key types: HookEvent.
hooks-store/internal/ingest/ — HTTP ingest server. Key types: Server, IngestEvent. Key funcs: New, SetOnIngest, ErrCount.
hooks-store/internal/store/ — MeiliSearch storage layer. Key types: Document, EventStore, MeiliStore. Key funcs: NewMeiliStore, HookEventToDocument.
hooks-store/internal/tui/ — Bubble Tea dashboard. Key types: Model, Config. Key funcs: NewModel, Run.

### claude-hooks-monitor packages

claude-hooks-monitor/cmd/monitor/ — Entry point. Console + TUI modes, signal handling.
claude-hooks-monitor/cmd/hook-client/ — Per-event binary. Stdin JSON, loopback-only URL, alpha-only hookType validation.

claude-hooks-monitor/internal/hookevt/ — Shared HookEvent type (same schema as hooks-store).
claude-hooks-monitor/internal/config/ — INI config, hook toggles, sink config. Key types: HookConfig, SinkConfig. Key funcs: ReadConfig, WriteConfig, ReadSinkConfig.
claude-hooks-monitor/internal/monitor/ — Core ring buffer (1000 events). Key types: HookMonitor. Key funcs: NewHookMonitor, AddEvent, CloseChannel.
claude-hooks-monitor/internal/server/ — HTTP handlers + middleware. Key funcs: HandleHook, HandleStats, HandleEvents, AuthMiddleware.
claude-hooks-monitor/internal/sink/ — Event forwarding. Key types: EventSink, HTTPSink. Key funcs: NewHTTPSink, Send.
claude-hooks-monitor/internal/platform/ — OS-specific lock/signals (flock on Unix, LockFileEx on Windows). Key funcs: AcquireLock, ShowRunningInstance.
claude-hooks-monitor/internal/tui/ — Interactive tree UI. 6 files: model, tree, processor, detail, hooks_menu, styles. Key types: Model, Session, EventProcessor. Key funcs: Run, FlattenTree.

### hooks-mcp packages

hooks-mcp/cmd/hooks-mcp/ — Entry point. Env config, MeiliSearch health check, MCP server + stdio.

hooks-mcp/internal/dateparse/ — Date range parsing ("today", "last 3 days" → DateRange). Key types: DateRange. Key funcs: ParseRange.
hooks-mcp/internal/format/ — Pure formatting functions. Key funcs: Table, Tree, BarChart, FormatDuration, FormatCost, FormatTokens, ShortID.
hooks-mcp/internal/meili/ — Typed MeiliSearch client. Key types: Searcher (interface), MeiliClient, SessionHit, PromptHit, EventHit. Key funcs: NewMeiliClient, ResolveSessionPrefix.
hooks-mcp/internal/tools/ — 8 MCP tool handlers. Key funcs: RegisterAll. Tools: query-sessions, query-prompts, session-summary, project-activity, search-events, error-analysis, cost-analysis, tool-usage.

## Querying Hook Data in MeiliSearch

See [docs/meilisearch-query-guide.md](docs/meilisearch-query-guide.md) for schema, query patterns, and recipes. Read it before querying the database.

For the integration strategy (MCP server, slash commands, custom agents) see [docs/meilisearch-integration-strategy.md](docs/meilisearch-integration-strategy.md).

## Active Files (read source before editing, don't rely on summaries)

No files currently under active development.

## CLAUDE.md Maintenance

Every directory has a CLAUDE.md with package summaries. Prefer summaries over re-reading stable source files for context. When you need to edit a file, always read the actual source. After changing a package's exported API, update its CLAUDE.md.
