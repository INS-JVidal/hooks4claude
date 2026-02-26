# Quickstart — Running the Full Hook Pipeline

This guide walks you through launching the complete system:
**Claude hooks → hook-client → monitor → sink → hooks-store → MeiliSearch**

## Clone

```bash
git clone --recurse-submodules https://github.com/INS-JVidal/hooks4claude.git
cd hooks4claude
```

If you already cloned without `--recurse-submodules`:

```bash
git submodule update --init
```

## Prerequisites

| Component | Status check |
|-----------|-------------|
| Go 1.25+  | `go version` |
| curl + jq | `which curl jq` |

## Architecture Overview

```
Claude Code session
  │
  ├─ hook events (stdin JSON)
  │
  ▼
hook-client  (binary, runs per-event)
  │
  ├─ POST /hook/<HookType>
  │
  ▼
claude-hooks-monitor  (long-running, port 8080)
  │
  ├─ TUI display + /stats /events /health API
  │
  ├─ fire-and-forget goroutine (if sink enabled)
  │   POST /ingest
  │   ▼
  hooks-store  (long-running, port 9800)
  │
  ├─ transform + index
  │   ▼
  MeiliSearch  (long-running, port 7700)
```

---

## Step 1 — Install & Start MeiliSearch

```bash
cd hooks-store

# Option A: install binary to ~/.local/bin
./scripts/install-meili.sh

# Option B: install as systemd user service (auto-start on login)
./scripts/install-meili.sh --service
```

If you chose Option A, start it manually:

```bash
meilisearch --db-path ~/.local/share/meilisearch/data.ms --http-addr 127.0.0.1:7700
```

Verify it's running:

```bash
curl -s http://localhost:7700/health | jq .
# Expected: {"status":"available"}
```

## Step 2 — Configure MeiliSearch Index

Run once to set up searchable/filterable/sortable attributes:

```bash
cd hooks-store
./scripts/setup-meili-index.sh
```

You should see:

```
Configuring index 'hook-events'...
  Creating index... ok
  Setting searchable attributes... ok
  Setting filterable attributes... ok
  Setting sortable attributes... ok
```

## Step 3 — Build Binaries

```bash
# From the project root
cd claude-hooks-monitor && make build && cd ..
cd hooks-store && make build && cd ..
```

## Step 4 — Start hooks-store (companion)

In a **dedicated terminal**:

```bash
cd hooks-store
./bin/hooks-store
# Or with custom settings:
# ./bin/hooks-store --port 9800 --meili-url http://localhost:7700
```

You should see:

```
Connecting to MeiliSearch at http://localhost:7700...
──────────────────────────────────────────────────────
  hooks-store dev
──────────────────────────────────────────────────────
  MeiliSearch: http://localhost:7700 (index: hook-events)
  Listening:   http://localhost:9800
  Endpoints:   POST /ingest  GET /health  GET /stats
──────────────────────────────────────────────────────
  Waiting for events...
```

Verify:

```bash
make companion-health
# Expected: {"status":"ok","meili":"connected"}
```

## Step 5 — Enable Sink Forwarding in Monitor Config

Edit `~/.config/claude-hooks-monitor/hook_monitor.conf` and add the `[sink]` section:

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
cd claude-hooks-monitor

# Console mode (see events scroll by):
make run

# Or interactive TUI mode:
make run-ui
```

Verify:

```bash
make check
# Expected: Server is running on port 8080
```

## Step 7 — Verify Claude Hooks Are Registered

The hooks are already registered in `~/.claude/settings.json` (all 15 hook types).
They point to `hook-client` which must be in your PATH:

```bash
which hook-client
# Expected: /home/opos/.local/bin/hook-client
```

If not found, either:
- Run `make install` from `claude-hooks-monitor/` to copy binaries to `~/.local/bin/`
- Or ensure `~/.local/bin` is in your PATH: `export PATH="$HOME/.local/bin:$PATH"`

## Step 8 — Test the Pipeline

### Manual test (without Claude):

```bash
# Send a test event directly to the monitor
cd claude-hooks-monitor && make send-test-hook

# Check it arrived at hooks-store
cd ../hooks-store && make companion-stats

# Search for it in MeiliSearch
make meili-search Q="Bash"
```

### Live test (with Claude):

Simply start a new Claude Code session in any project.
The hooks fire automatically — every tool use, prompt submit, session start, etc.
Watch the monitor terminal for live events scrolling by.

```bash
# In a third terminal, check stats accumulating:
cd hooks-store && make companion-stats

# Search all indexed events:
make meili-search Q="Write"

# Filter by hook type:
curl -s 'http://localhost:7700/indexes/hook-events/search' \
  -H 'Content-Type: application/json' \
  -d '{"filter":"hook_type = PreToolUse","sort":["timestamp_unix:desc"],"limit":5}' | jq .
```

---

## Terminal Layout (Recommended)

```
┌──────────────────────┬──────────────────────┐
│                      │                      │
│   MeiliSearch        │   hooks-store        │
│   (port 7700)        │   (port 9800)        │
│                      │                      │
├──────────────────────┼──────────────────────┤
│                      │                      │
│   monitor (TUI)      │   Claude Code        │
│   (port 8080)        │   (your session)     │
│                      │                      │
└──────────────────────┴──────────────────────┘
```

## Quick Reference — All Ports

| Service        | Port | Purpose |
|---------------|------|---------|
| MeiliSearch   | 7700 | Search engine + dashboard |
| Monitor       | 8080 | Hook event receiver + TUI |
| hooks-store   | 9800 | Ingest → MeiliSearch bridge |

## Quick Reference — Health Checks

```bash
curl -s http://localhost:7700/health | jq .     # MeiliSearch
curl -s http://localhost:8080/health | jq .     # Monitor
curl -s http://localhost:9800/health | jq .     # hooks-store
```

## Troubleshooting

**hooks-store fails to start**: MeiliSearch must be running first.

**Events not reaching MeiliSearch**: Check `forward = yes` in the `[sink]` section
of `~/.config/claude-hooks-monitor/hook_monitor.conf`. The monitor must be restarted
after changing this setting.

**hook-client not found**: Ensure `~/.local/bin` is in your PATH.

**Port already in use**: Another instance may be running. Check with:
`lsof -i:8080` / `lsof -i:9800` / `lsof -i:7700`
