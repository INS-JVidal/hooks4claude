# hooks4claude — Hook monitoring and storage system for Claude Code

Two independent Go programs that form a pipeline:
1. **claude-hooks-monitor** — Receives hook events from Claude Code via hook-client, displays in console/TUI, optionally forwards to hooks-store
2. **hooks-store** — Ingests events via HTTP, indexes into MeiliSearch for search and filtering

Each is a separate Go module with its own go.mod, git repo, and binary.

## Pipeline

```
Claude Code                hooks-client               monitor                    hooks-store
───────────                ────────────               ───────                    ───────────
Hook fires ──→ hook-client ──→ POST /hook/<Type> ──→ HookMonitor ──→ HTTPSink ──→ POST /ingest ──→ MeiliSearch
               (stdin JSON,    (loopback only)        ├─ console log                                (search/filter)
                exit 0)                               └─ TUI eventCh
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
| CLI flags | `--port`, `--meili-url`, `--meili-key`, `--meili-index` |
| Env vars | `HOOKS_STORE_PORT`, `MEILI_URL`, `MEILI_KEY`, `MEILI_INDEX` |
| Config file | `hooks-store.conf` (lowest priority) |

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

## Active Files (read source before editing, don't rely on summaries)

No files currently under active development.

## CLAUDE.md Maintenance

Every directory has a CLAUDE.md with package summaries. Prefer summaries over re-reading stable source files for context. When you need to edit a file, always read the actual source. After changing a package's exported API, update its CLAUDE.md.
