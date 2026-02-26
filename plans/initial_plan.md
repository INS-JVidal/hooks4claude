# Project Specification: Claude Code Hooks Monitor

**Version:** 2.2
**Date:** February 2026
**Target:** Claude Code CLI Implementation
**Language:** Go (Server), Python (Hooks)

---

## 1. Executive Summary

### 1.1 Project Overview
Build a real-time monitoring and logging system for Claude Code CLI hooks that helps developers and students understand how Claude Code's hook system works. The system consists of:
- A Go-based REST API server that receives and logs hook events
- Python hook scripts that intercept Claude Code hooks and send data to the server
- Complete documentation and testing infrastructure

### 1.2 Learning Objectives
- Understand Claude Code's hook event types (14 documented events; this project monitors 14)
- Visualize hook execution flow in real-time
- Learn REST API design patterns
- Study inter-process communication
- Explore event-driven architectures
- Understand hook stdout semantics (decisions, blocking, context injection)

### 1.3 Key Deliverables
1. Go REST API server with colorized console logging
2. Universal Python hook script compatible with all hook types
3. Claude Code configuration for all 14 hooks
4. Testing infrastructure (test script, Makefile)
5. Comprehensive documentation (README with quickstart, EXAMPLES, ARCHITECTURE)

---

## 2. Technical Requirements

### 2.1 System Requirements
- **Go:** Version 1.21 or higher
- **Python:** Version 3.11 or higher
- **uv:** Python package manager (for hook scripts)
- **Operating System:** macOS, Linux, or Windows with WSL
- **Claude Code CLI:** Latest version installed

### 2.2 Dependencies

#### Go Dependencies
```go
require github.com/fatih/color v1.16.0
```

#### Python Dependencies (via uv)
```python
# dependencies in script header:
# requires-python = ">=3.11"
# dependencies = ["requests"]
```

### 2.3 Network Requirements
- HTTP server on localhost (default port 8080)
- No external network access required
- All communication via localhost HTTP

---

## 3. Architecture Specifications

### 3.1 System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Claude Code CLI                            │
│  Hook Event Fires → Executes Python Script                     │
│  Provides JSON on stdin with common + event-specific fields    │
└─────────────────────┬───────────────────────────────────────────┘
                      │ stdin (JSON)
                      ↓
┌─────────────────────────────────────────────────────────────────┐
│            hook_monitor.py (Python Script)                      │
│  1. Read hook_event_name from stdin JSON                       │
│  2. Check hook_monitor.conf — is this hook enabled?            │
│     • If disabled → exit 0 immediately (skip)                  │
│     • If enabled  → continue                                   │
│  3. Add metadata (_monitor field: timestamp, project_dir)      │
│  4. HTTP POST to server using hook_event_name from payload     │
│  5. Exit 0 (non-blocking, no stdout output)                    │
└─────────────────────┬───────────────────────────────────────────┘
                      │ HTTP POST application/json (only if enabled)
                      ↓
┌─────────────────────────────────────────────────────────────────┐
│              Go REST API Server                                 │
│  Endpoints:                                                     │
│  • POST /hook/{HookType} - Receive hook events                 │
│  • GET  /stats           - Get statistics                      │
│  • GET  /events          - Get event history (last 100)        │
│  • GET  /health          - Health check                        │
│                                                                 │
│  Storage: In-memory ring buffer (max 1000 events)              │
│  Output: Colorized console logs                                │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  hooks/hook_monitor.conf (Toggle File)                          │
│  SessionStart = yes       PreCompact = yes                     │
│  PreToolUse = yes         ConfigChange = yes                   │
│  PostToolUse = yes        ...                                  │
│  (edit to "no" to disable any hook — takes effect immediately) │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 Data Flow

1. **Hook Trigger:** Claude Code fires a hook (e.g., PreToolUse)
2. **Script Execution:** Python script runs with JSON on stdin
3. **Data Read:** Script reads JSON from stdin (contains `hook_event_name`)
4. **Config Check:** Script reads `hooks/hook_monitor.conf` to check if this hook is enabled
   - If **disabled** → exit 0 immediately, no network call, no overhead
   - If **enabled** → continue to step 5
5. **Enrichment:** Adds `_monitor` object with timestamp and project dir
6. **HTTP POST:** Sends enriched JSON to `/hook/{hook_event_name}` endpoint
7. **Server Processing:**
   - Receives POST request
   - Creates HookEvent object
   - Stores in ring buffer (max 1000 events)
   - Logs to console with colors
   - Returns 200 OK
8. **Non-blocking Exit:** Script exits 0 with no stdout, Claude continues normally

### 3.3 Hook Types to Monitor

All 14 Claude Code hook event types:

| # | Event | Description | Has Matcher? |
|---|-------|-------------|:---:|
| 1 | **SessionStart** | Fires once when session begins | No |
| 2 | **UserPromptSubmit** | Fires when user submits a prompt | No |
| 3 | **PreToolUse** | Fires before any tool execution | Yes |
| 4 | **PermissionRequest** | Fires when Claude needs permission | No |
| 5 | **PostToolUse** | Fires after tool completes successfully | Yes |
| 6 | **PostToolUseFailure** | Fires after tool execution fails | Yes |
| 7 | **Notification** | Fires on system notifications | No |
| 8 | **SubagentStart** | Fires when a subagent is launched | No |
| 9 | **SubagentStop** | Fires when subagent completes | No |
| 10 | **Stop** | Fires when Claude stops responding | No |
| 11 | **TeammateIdle** | Fires when a teammate agent is idle | No |
| 12 | **TaskCompleted** | Fires when an assigned task completes | No |
| 13 | **ConfigChange** | Fires when configuration changes | No |
| 14 | **PreCompact** | Fires before context compaction | No |
| — | **SessionEnd** | Fires when session terminates (not configurable) | No |

> **Note:** `SessionEnd` is a special event. It fires on session termination but may not always be configurable in settings.json depending on the Claude Code version. The plan includes it for completeness.

### 3.4 Common Stdin Fields (All Events)

Every hook event receives these common fields via stdin JSON:

```json
{
  "session_id": "abc123-def456",
  "transcript_path": "/home/user/.claude/projects/.../transcript.jsonl",
  "cwd": "/home/user/my-project",
  "permission_mode": "default",
  "hook_event_name": "PreToolUse"
}
```

The `hook_event_name` field is the canonical way to identify which hook is firing. Event-specific fields are merged alongside these common fields.

### 3.5 Hook Stdout Semantics (Educational Reference)

This project is a **passive monitor** — it produces no stdout and always exits 0. However, for educational purposes, students should understand that hooks can actively influence Claude's behavior:

| Mechanism | Effect |
|-----------|--------|
| **Exit code 0** | Success. Any JSON on stdout is processed for decisions. |
| **Exit code 2** | Blocking error. Stdout ignored. Stderr fed to Claude as error. Blocks the associated action. |
| **Other exit codes** | Non-blocking error. Stderr shown in verbose mode only. |
| **stdout `{"decision": "block", "reason": "..."}` ** | Blocks the action (PreToolUse denies tool, Stop forces continue). |
| **stdout `{"continue": false, "stopReason": "..."}` ** | Immediately stops Claude entirely. |
| **PreToolUse stdout `updatedInput`** | Modifies the tool's input before execution. |
| **SessionStart stdout** | Added to Claude's context as additional information. |

---

## 4. Component Specifications

### 4.1 Go Server (main.go)

#### 4.1.1 Data Structures

```go
const maxEvents = 1000

type HookEvent struct {
    HookType  string                 `json:"hook_type"`
    Timestamp time.Time              `json:"timestamp"`
    Data      map[string]interface{} `json:"data"`
}

type HookMonitor struct {
    events []HookEvent
    mu     sync.RWMutex
    stats  map[string]int
}
```

#### 4.1.2 Core Methods

**NewHookMonitor() *HookMonitor**
- Returns initialized HookMonitor
- Empty events slice
- Empty stats map

**AddEvent(event HookEvent)**
- Thread-safe event addition (use mu.Lock())
- Append to events slice
- If len(events) > maxEvents, trim oldest entries
- Increment stats counter for hook type
- Call logEvent() for console output

**logEvent(event HookEvent)**
- Print separator line (80 "═" characters)
- Print hook type with appropriate color
- Print timestamp (format: "15:04:05.000")
- Pretty-print JSON data with 2-space indent
- Print separator line

**GetStats() map[string]int**
- Thread-safe read (use mu.RLock())
- Return copy of stats map

**GetEvents(limit int) []HookEvent**
- Thread-safe read
- Return last N events
- If limit <= 0 or > len(events), return all
- Return slice from end

#### 4.1.3 HTTP Handlers

**handleHook(hookType string) http.HandlerFunc**
- Accept only POST requests
- Read request body
- Parse JSON into map[string]interface{}
- Create HookEvent with current timestamp
- Call AddEvent()
- Return JSON: {"status": "ok", "hook": hookType}

**handleStats(w http.ResponseWriter, r *http.Request)**
- Return JSON with stats map and total_hooks count
- Example: {"stats": {...}, "total_hooks": 17}

**handleEvents(w http.ResponseWriter, r *http.Request)**
- Get last 100 events (configurable via ?limit=N query param)
- Return JSON: {"events": [...], "count": N}

**handleHealth(w http.ResponseWriter, r *http.Request)**
- Return JSON: {"status": "healthy", "time": ISO8601}

#### 4.1.4 Color Mapping

Use `github.com/fatih/color` package:

| Hook Type | Color |
|-----------|-------|
| SessionStart | Green, Bold |
| SessionEnd | Red, Bold |
| PreToolUse | Yellow, Bold |
| PostToolUse | Cyan, Bold |
| PostToolUseFailure | HiRed, Bold |
| UserPromptSubmit | Magenta, Bold |
| Notification | Blue, Bold |
| PermissionRequest | White, Bold |
| Stop | Red |
| SubagentStart | HiCyan |
| SubagentStop | HiCyan, Bold |
| TeammateIdle | HiBlue |
| TaskCompleted | HiGreen |
| ConfigChange | HiYellow |
| PreCompact | HiMagenta |

#### 4.1.5 Server Initialization

```go
func main() {
    monitor := NewHookMonitor()

    // Print banner with color
    // Register all 14 hook endpoints + SessionEnd
    // Register utility endpoints (/stats, /events, /health)
    // Get PORT from env (default: "8080")
    // Start server: http.ListenAndServe(":"+port, nil)
}
```

---

### 4.2 Python Hook Script (hooks/hook_monitor.py)

#### 4.2.1 File Header (uv single-file script)

```python
#!/usr/bin/env -S uv run --quiet --script
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "requests",
# ]
# ///
```

#### 4.2.2 Configuration

```python
MONITOR_URL = os.getenv("HOOK_MONITOR_URL", "http://localhost:8080")
CONFIG_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "hook_monitor.conf")

def _safe_int(value: str, default: int) -> int:
    """Parse int from string, returning default on any failure."""
    try:
        return int(value)
    except (ValueError, TypeError):
        return default

TIMEOUT = _safe_int(os.getenv("HOOK_TIMEOUT", "5"), 5)
```

> **Note:** `HOOK_TYPE` env var is not needed. The hook type is read from the `hook_event_name` field in the stdin JSON payload, which Claude Code provides automatically.

#### 4.2.3 Core Functions

**is_hook_enabled(hook_name: str) -> bool**
- Create `configparser.ConfigParser()` instance
- **Critical:** Set `parser.optionxform = str` to preserve case (default lowercases keys, which would break `SessionStart` → `sessionstart` mismatch)
- Read `CONFIG_PATH` using `parser.read()` (returns empty list on missing file — no exception)
- Look up `hook_name` in section `[hooks]`; if value is `yes` → return True, if `no` → return False
- If key is missing → return True (default: enabled)
- If config file is missing or unreadable → return True (fail-open: monitor everything)
- Wrap entire function body in try/except → return True (must never raise)

**read_stdin() -> Dict[str, Any]**
- Read all data from sys.stdin
- Parse as JSON
- Return dict
- On error: return {"error": str(e), "raw_input": data}

**send_to_monitor(hook_type: str, data: Dict[str, Any]) -> bool**
- Construct URL: f"{MONITOR_URL}/hook/{hook_type}"
- POST request with JSON body
- Timeout: TIMEOUT seconds
- Headers: {"Content-Type": "application/json"}
- Return True on success (status 200)
- On ConnectionError: return False silently (server not running)
- On other errors: print warning to stderr, return False

**main()**
- Read stdin → hook_data
- Extract hook_type from hook_data["hook_event_name"] (fallback: "Unknown")
- **Check config:** if not is_hook_enabled(hook_type) → exit 0 immediately (skip)
- Add `hook_data["_monitor"]` dict with enrichment metadata:
  ```python
  hook_data["_monitor"] = {
    "timestamp": datetime.now().isoformat(),
    "project_dir": hook_data.get("cwd", ""),
    "plugin_root": os.getenv("CLAUDE_PLUGIN_ROOT", ""),
    "is_remote": os.getenv("CLAUDE_CODE_REMOTE", "") == "true"
  }
  ```
- Send to monitor
- **Produce no stdout** (stdout carries semantic meaning for hooks)
- Always exit 0

#### 4.2.4 Error Handling
- **Connection errors:** Fail silently (monitor might not be running)
- **JSON parse errors:** Include error in data sent to server
- **Timeout errors:** Fail silently
- **Always exit 0:** Critical — never block Claude Code operation
- **Never write to stdout:** Stdout is reserved for hook decisions; only write to stderr for debugging

---

### 4.3 Claude Code Configuration (.claude/settings.json)

#### 4.3.1 Complete Hook Configuration

The hooks configuration must coexist with any existing `permissions` block in `.claude/settings.json`. The following shows only the `hooks` key:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "PostToolUseFailure": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "Notification": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "PermissionRequest": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "SubagentStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "SubagentStop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "TeammateIdle": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "TaskCompleted": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "ConfigChange": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/hooks/hook_monitor.py"
          }
        ]
      }
    ]
  }
}
```

#### 4.3.2 Key Design Decisions

- **No HOOK_TYPE env var needed** — the script reads `hook_event_name` from stdin JSON
- **Identical command for every hook** — simplifies configuration; the payload self-identifies
- **matcher: "*"** for PreToolUse/PostToolUse/PostToolUseFailure — captures all tools
- **$CLAUDE_PROJECT_DIR** — references project root (handles spaces via quotes)
- **type: "command"** — executes shell command (not LLM prompt)
- **Always quote paths** — handles directories with spaces
- **Coexistence** — the `hooks` key sits alongside `permissions` in settings.json

---

### 4.4 Go Module Configuration (go.mod)

```
module claude-hooks-monitor

go 1.21

require github.com/fatih/color v1.16.0

require (
    github.com/mattn/go-colorable v0.1.13 // indirect
    github.com/mattn/go-isatty v0.0.20 // indirect
    golang.org/x/sys v0.14.0 // indirect
)
```

---

### 4.5 Hook Toggle Configuration (hooks/hook_monitor.conf)

#### 4.5.1 Purpose

A simple INI-style config file that controls which hooks are monitored. Every hook defaults to `yes` (enabled). Users can set any hook to `no` to disable it. Changes take effect immediately — the Python script reads this file on every invocation (hooks are short-lived processes, so there's no caching issue).

#### 4.5.2 File Format

```ini
[hooks]
# Toggle individual hooks on/off.
# Set to "yes" to monitor, "no" to skip.
# Missing entries default to "yes" (monitor everything).
# Changes take effect immediately — no restart needed.

SessionStart = yes
UserPromptSubmit = yes
PreToolUse = yes
PostToolUse = yes
PostToolUseFailure = yes
PermissionRequest = yes
Notification = yes
SubagentStart = yes
SubagentStop = yes
Stop = yes
TeammateIdle = yes
TaskCompleted = yes
ConfigChange = yes
PreCompact = yes
SessionEnd = yes
```

#### 4.5.3 Design Decisions

- **INI format** — Python's `configparser` (stdlib) reads it natively, no extra dependency
- **Fail-open** — if the file is missing or unreadable, all hooks are treated as enabled
- **Missing keys default to yes** — adding new hooks in the future doesn't require config changes
- **Lives in `hooks/` directory** — co-located with the script that reads it
- **Human-editable** — plain text, no JSON/YAML quoting issues, supports comments
- **No restart needed** — read fresh on every hook invocation

#### 4.5.4 Usage Examples

```bash
# Disable noisy PreToolUse/PostToolUse to focus on session lifecycle:
sed -i 's/^PreToolUse = yes/PreToolUse = no/' hooks/hook_monitor.conf
sed -i 's/^PostToolUse = yes/PostToolUse = no/' hooks/hook_monitor.conf

# Re-enable:
sed -i 's/^PreToolUse = no/PreToolUse = yes/' hooks/hook_monitor.conf

# Or just edit the file directly in any text editor
```

---

## 5. Testing Infrastructure

### 5.1 Test Script (test-hooks.sh)

#### 5.1.1 Requirements
- Bash script
- Check if server is running before tests
- Simulate all 14 hook types (+ SessionEnd)
- Use realistic test data matching actual Claude Code stdin schemas
- Colorized output (Green check, Red cross, Yellow arrow)
- Sleep 0.5s between events for visibility

#### 5.1.2 Test Data Specifications

All test payloads include the **common fields** that Claude Code provides to every hook:

```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "<EventType>"
}
```

Event-specific fields are merged alongside these common fields.

**SessionStart:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "SessionStart"
}
```

**UserPromptSubmit:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "UserPromptSubmit",
  "prompt": "Create a Python script that calculates fibonacci numbers"
}
```

**PreToolUse (Write):**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "PreToolUse",
  "tool_name": "Write",
  "tool_input": {
    "file_path": "fibonacci.py",
    "content": "def fib(n):\n    if n <= 1:\n        return n\n    return fib(n-1) + fib(n-2)"
  }
}
```

**PostToolUse (Write):**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "PostToolUse",
  "tool_name": "Write",
  "tool_input": {
    "file_path": "fibonacci.py"
  },
  "tool_response": {
    "success": true,
    "bytes_written": 128
  }
}
```

**PostToolUseFailure (Bash):**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "PostToolUseFailure",
  "tool_name": "Bash",
  "tool_input": {
    "command": "python nonexistent.py"
  },
  "tool_response": {
    "exit_code": 1,
    "stdout": "",
    "stderr": "python: can't open file 'nonexistent.py': [Errno 2] No such file or directory"
  }
}
```

**PreToolUse (Bash):**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "tool_input": {
    "command": "python fibonacci.py"
  }
}
```

**PostToolUse (Bash):**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "PostToolUse",
  "tool_name": "Bash",
  "tool_input": {
    "command": "python fibonacci.py"
  },
  "tool_response": {
    "exit_code": 0,
    "stdout": "0 1 1 2 3 5 8 13 21 34",
    "stderr": ""
  }
}
```

**Notification:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "Notification",
  "notification_type": "permission_prompt",
  "message": "Claude needs your permission to use Bash",
  "tool_name": "Bash"
}
```

**PermissionRequest:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "PermissionRequest",
  "tool_name": "Bash",
  "tool_input": {
    "command": "rm -rf test.txt"
  }
}
```

**Stop:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "Stop",
  "stop_hook_active": true,
  "last_assistant_message": "I've completed the fibonacci implementation."
}
```

**SubagentStart:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "SubagentStart",
  "agent_id": "linter-agent-001",
  "agent_type": "code-reviewer"
}
```

**SubagentStop:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "SubagentStop",
  "agent_id": "linter-agent-001",
  "agent_type": "code-reviewer",
  "agent_transcript_path": "/home/test/.claude/projects/demo/agents/linter.jsonl",
  "last_assistant_message": "Code quality check passed.",
  "stop_hook_active": false
}
```

**TeammateIdle:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "TeammateIdle"
}
```

**TaskCompleted:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "TaskCompleted"
}
```

**ConfigChange:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "ConfigChange"
}
```

**PreCompact:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "PreCompact"
}
```

**SessionEnd:**
```json
{
  "session_id": "test-session-001",
  "transcript_path": "/home/test/.claude/projects/demo/transcript.jsonl",
  "cwd": "/home/test/projects/demo",
  "permission_mode": "default",
  "hook_event_name": "SessionEnd",
  "reason": "user_exit"
}
```

#### 5.1.3 Test Flow

**Phase 1: Direct server test (curl → server)**
1. Check server health (curl localhost:8080/health)
2. If not running, exit with error message
3. Send each hook type via curl POST in logical order (session lifecycle)
4. Print progress for each (arrow Testing X... check Success)
5. Print summary at end (N/N events sent)
6. Show commands to check stats and events

**Phase 2: End-to-end test (stdin → Python script → server)**
7. Pipe a test JSON payload through `hook_monitor.py` to verify the full pipeline:
   ```bash
   echo '{"hook_event_name":"PreToolUse","session_id":"e2e-test","cwd":"/tmp","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"echo test"}}' | ./hooks/hook_monitor.py
   ```
8. Verify event appears in server (`curl localhost:8080/events?limit=1`)
9. Verify script produced no stdout (pipe through `wc -c`, expect 0)

**Phase 3: Config toggle test**
10. Temporarily set `PreToolUse = no` in `hook_monitor.conf`
11. Record current stats count for PreToolUse
12. Pipe a PreToolUse payload through `hook_monitor.py`
13. Verify stats count did NOT increase (hook was skipped)
14. Restore `PreToolUse = yes` in conf

---

### 5.2 Makefile

#### 5.2.1 Required Targets

```makefile
help          # Show all targets with descriptions
deps          # Install Go and check Python dependencies
build         # Build Go binary to bin/monitor
run           # Run server with go run main.go
run-background # Run in background, output to monitor.log
test          # Run test-hooks.sh
test-api      # Test API endpoints with curl | jq
send-test-hook # Send single test event
clean         # Remove bin/ and logs
install       # Install binary to ~/bin
check         # Check if server is running
stats         # curl localhost:8080/stats | jq
show-config   # Display current hook_monitor.conf toggle state
reset-config  # Reset hook_monitor.conf to all-enabled defaults
```

#### 5.2.2 Implementation Notes
- Use `.PHONY` for all targets
- Add helpful echo messages with colors
- Check for required tools (uv, jq, etc.)
- Make hook scripts executable in deps target
- Default port: 8080 (configurable via PORT env var)

---

## 6. Documentation Requirements

### 6.1 README.md

**Sections Required:**
1. **Title and Description** — What the project does
2. **Features** — Bullet list of capabilities
3. **Prerequisites** — Required software
4. **Quick Start** — 3-step setup (absorbs the former QUICKSTART.md)
5. **API Endpoints** — Table of all endpoints
6. **Hook Types** — List and explanation of all 14
7. **Hook Stdout Semantics** — How hooks can influence Claude (educational)
8. **Usage Examples** — How to use with Claude Code
9. **Testing** — How to test without Claude
10. **Configuration** — Environment variables and hook_monitor.conf toggle
11. **Troubleshooting** — Common issues
12. **Learning Guide** — Exercises for students
13. **Project Structure** — File tree
14. **Next Steps** — Extension ideas
15. **Resources** — Links to docs
16. **License** — MIT

**Length:** 400-600 lines
**Style:** Technical but accessible

---

### 6.2 EXAMPLES.md

**Sections Required:**
1. **Server Startup** — Banner output
2. **Example for each hook type** — All 14 hooks + SessionEnd
3. **API Response Examples** — /stats and /events
4. **Color Legend** — Explain color coding
5. **Full Session Example** — Complete workflow

**Length:** 200-300 lines
**Style:** Show, don't tell — mostly code blocks

---

### 6.3 ARCHITECTURE.md

**Sections Required:**
1. **System Overview** — ASCII diagram
2. **Data Flow** — Step-by-step with diagrams
3. **Component Details** — Each component explained
4. **Environment Variables** — Complete table
5. **Hook Lifecycle** — Diagrams showing flow
6. **Hook Stdout Semantics** — Decision/blocking model explained
7. **Common Stdin Fields** — Schema reference
8. **Security Notes** — Important warnings
9. **Extension Points** — How to customize
10. **Performance** — Overhead and throughput

**Length:** 300-400 lines
**Style:** Technical deep-dive, diagrams, tables

---

## 7. Project Structure

### 7.1 Directory Layout

```
claude-hooks-monitor/
├── .claude/
│   └── settings.json          # Hook config + permissions (coexist)
├── hooks/
│   ├── hook_monitor.py        # Universal Python hook script
│   └── hook_monitor.conf      # Toggle file: enable/disable individual hooks
├── bin/                       # Build output (gitignored)
├── .gitignore                 # Git ignore patterns
├── go.mod                     # Go module definition
├── go.sum                     # Go dependencies (generated)
├── main.go                    # Go server implementation
├── Makefile                   # Build automation
├── test-hooks.sh              # Test script
├── README.md                  # Main documentation (includes quickstart)
├── EXAMPLES.md                # Output examples
└── ARCHITECTURE.md            # Architecture docs
```

### 7.2 File Permissions
- `hooks/hook_monitor.py` — Executable (755)
- `hooks/hook_monitor.conf` — Regular (644), user-editable
- `test-hooks.sh` — Executable (755)
- All other files — Regular (644)

### 7.3 .gitignore Contents

```gitignore
# Binaries
bin/
*.exe
*.dll
*.so
*.dylib

# Go
*.test
*.out
go.work

# Logs
*.log
monitor.log

# Python
__pycache__/
*.py[cod]
*$py.class
.Python
*.so
.venv/
venv/
ENV/

# IDE
.vscode/
.idea/
*.swp
*.swo
*~

# OS
.DS_Store
Thumbs.db

# Local env
.env
.env.local
```

---

## 8. Implementation Guidelines

### 8.1 Code Style

#### Go Code Style
- Use `gofmt` formatting
- Follow standard Go project layout
- Clear variable names (no single letters except i, j in loops)
- Add comments for exported functions
- Error handling: check every error
- Use `defer` for cleanup

#### Python Code Style
- Follow PEP 8
- Type hints for function signatures
- Docstrings for all functions
- Use f-strings for formatting
- Clear variable names

### 8.2 Error Handling

#### Go Server
- Log errors to stderr
- Return appropriate HTTP status codes
- Never panic in handlers
- Graceful degradation

#### Python Hooks
- **Critical:** Always exit 0
- **Critical:** Never write to stdout (reserved for hook decisions)
- Silent failure on connection errors
- Log only to stderr (stdout reserved for hook output)
- Include error details in data sent to server

### 8.3 Thread Safety

#### Go Server
- Use `sync.RWMutex` for shared data
- Lock before writes (mu.Lock())
- RLock for reads (mu.RLock())
- Always defer Unlock()
- No data races

### 8.4 Performance

- Server should handle 1000+ req/sec
- No unnecessary allocations in hot paths
- HTTP timeouts: 5 seconds default
- Event history bounded to 1000 events (ring buffer)

---

## 9. Testing Specifications

### 9.1 Manual Testing Checklist

- [ ] Server starts without errors
- [ ] All 14 endpoints registered (+ SessionEnd)
- [ ] Health endpoint returns 200
- [ ] test-hooks.sh runs successfully
- [ ] All hook types appear in console
- [ ] Colors display correctly
- [ ] Stats endpoint returns correct counts
- [ ] Events endpoint returns last events
- [ ] Python script executable
- [ ] uv dependencies install correctly
- [ ] Runs with Claude Code
- [ ] Non-blocking (Claude operates normally)
- [ ] Handles server down gracefully
- [ ] Python script produces no stdout
- [ ] hook_monitor.conf present with all hooks set to yes
- [ ] Disabling a hook in conf prevents it from reaching the server
- [ ] Missing conf file doesn't break the script (fail-open)

### 9.2 API Testing

Test each endpoint:

```bash
# Health
curl http://localhost:8080/health
# Expected: {"status":"healthy","time":"..."}

# Stats (after test-hooks.sh)
curl http://localhost:8080/stats
# Expected: {"stats":{...},"total_hooks":N}

# Events
curl http://localhost:8080/events
# Expected: {"events":[...],"count":N}

# Events with limit
curl http://localhost:8080/events?limit=5
# Expected: {"events":[last 5],"count":5}

# Hook (manual)
curl -X POST http://localhost:8080/hook/PreToolUse \
  -H "Content-Type: application/json" \
  -d '{"hook_event_name":"PreToolUse","tool_name":"test","session_id":"manual"}'
# Expected: {"status":"ok","hook":"PreToolUse"}
```

### 9.3 Integration Testing

With Claude Code:
1. Start monitor server
2. Run Claude Code from project directory
3. Execute various commands
4. Verify hooks appear in monitor
5. Check stats increase correctly
6. Verify non-blocking behavior

---

## 10. Deployment Specifications

### 10.1 Installation

```bash
# Method 1: Direct run
go run main.go

# Method 2: Build and install
make build
make install
~/bin/claude-hooks-monitor

# Method 3: Using Makefile
make run
```

### 10.2 Configuration

Environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| PORT | 8080 | Server port |
| HOOK_MONITOR_URL | http://localhost:8080 | Server URL (for hooks) |
| HOOK_TIMEOUT | 5 | HTTP timeout in seconds |

Usage:
```bash
# Custom port
PORT=9000 go run main.go

# Custom URL for hooks
HOOK_MONITOR_URL=http://localhost:9000 claude
```

### 10.3 Usage with Claude Code

```bash
# Copy settings to project (merge hooks key with existing settings)
# Or run Claude from this directory
cd claude-hooks-monitor
claude
```

---

## 11. Success Criteria

### 11.1 Functional Requirements
- All 14 hook types captured (+ SessionEnd)
- Real-time console logging with colors
- REST API with 4+ endpoints
- Non-blocking operation
- Thread-safe event storage with bounded memory
- Standalone testing capability

### 11.2 Educational Requirements
- Clear documentation for students
- Examples for all hook types
- Hook stdout semantics explained
- Learning exercises included
- Architecture clearly explained
- Easy setup (< 5 minutes)

### 11.3 Technical Requirements
- Go 1.21+ compatible
- Python 3.11+ with uv
- Cross-platform (macOS/Linux/WSL)
- No external dependencies beyond listed
- Clean, maintainable code
- Event buffer bounded (max 1000)

### 11.4 Documentation Requirements
- README (400-600 lines, includes quickstart)
- EXAMPLES (200-300 lines)
- ARCHITECTURE (300-400 lines)
- All commands documented
- All APIs documented

---

## 12. Extension Points

### 12.1 Optional Enhancements

These are NOT required but could be added:

1. **Persistence:** Save events to SQLite/PostgreSQL
2. **Web UI:** Real-time dashboard with WebSocket
3. **Filtering:** Query parameters for /events endpoint
4. **Export:** CSV/JSON export functionality
5. **Analytics:** Aggregate statistics and charts
6. **Webhooks:** Send events to external services
7. **Auth:** Basic authentication for API
8. **Docker:** Containerization support
9. **Metrics:** Prometheus/Grafana integration
10. **Multi-project:** Support multiple Claude projects

---

## 13. Validation Checklist

Before considering the project complete:

### 13.1 Code Quality
- [ ] All Go code formatted with gofmt
- [ ] All Python code follows PEP 8
- [ ] No compilation errors
- [ ] No runtime errors
- [ ] All imports used
- [ ] No TODO comments in production code

### 13.2 Functionality
- [ ] Server starts successfully
- [ ] All endpoints work
- [ ] All 14 hook types captured
- [ ] Colors display correctly
- [ ] test-hooks.sh passes
- [ ] Works with real Claude Code
- [ ] Non-blocking verified
- [ ] Python produces no stdout
- [ ] Event buffer stays bounded
- [ ] Config toggle works (disable a hook, verify it's skipped)

### 13.3 Documentation
- [ ] All markdown files present (3 files)
- [ ] No broken links
- [ ] Code examples tested
- [ ] Commands verified
- [ ] Formatting consistent

### 13.4 Deliverables
- [ ] All files in correct locations
- [ ] Permissions set correctly
- [ ] .gitignore includes all patterns
- [ ] Makefile targets all work
- [ ] Dependencies installable
- [ ] Project builds cleanly

---

## 14. Notes for Implementation

### 14.1 Critical Decisions

1. **Single Python Script:** One universal script — reads `hook_event_name` from stdin JSON instead of requiring per-hook env vars
2. **Config Toggle File:** INI-format `hook_monitor.conf` lets users enable/disable individual hooks without editing settings.json; fail-open design (missing file = all enabled)
3. **In-Memory Ring Buffer:** Bounded to 1000 events for memory safety; simplicity for learning
4. **Color Coding:** Different color per hook type for visual scanning
5. **Non-Blocking:** Hooks MUST exit 0 always and produce no stdout
6. **uv Single-File:** Modern Python approach, self-contained
7. **Thread Safety:** Go's sync.RWMutex for concurrent access
8. **Identical Command:** Same command string for every hook — the payload self-identifies

### 14.2 Common Pitfalls to Avoid

- Hook scripts that write to stdout (can trigger unintended decisions/blocking)
- Hook scripts that exit non-zero (exit 2 blocks actions!)
- Missing quotes around $CLAUDE_PROJECT_DIR
- Not checking if server is running
- Race conditions in Go server
- Forgetting to make scripts executable
- Missing matcher for PreToolUse/PostToolUse/PostToolUseFailure
- Not handling server down gracefully
- Unbounded event storage (always cap the buffer)
- Config file with wrong section name (must be `[hooks]`)
- Config parser raising on missing file (must fail-open)
- `configparser` lowercasing keys by default (`SessionStart` → `sessionstart`) — must set `optionxform = str`
- `HOOK_TIMEOUT` env var with non-integer value crashing the script — must use safe parse

### 14.3 Debug Tips

- Check hook permissions with `ls -la hooks/`
- Verify server running: `curl localhost:8080/health`
- Check Claude sees hooks: `/hooks` command in Claude Code
- Monitor server output in separate terminal
- Test Python script standalone: `echo '{"hook_event_name":"Test"}' | ./hooks/hook_monitor.py`
- Verify no stdout: `echo '{"hook_event_name":"Test"}' | ./hooks/hook_monitor.py | wc -c` (should be 0)
- Check config: `cat hooks/hook_monitor.conf` — verify the hook you expect is set to `yes`
- Test disabled hook: set a hook to `no` in conf, fire it, verify no event reaches the server

---

## 15. Reference Information

### 15.1 Hook Event Examples

See section 5.1.2 for complete test data examples with accurate schemas.

### 15.2 API Response Formats

**Success Response:**
```json
{
  "status": "ok",
  "hook": "PreToolUse"
}
```

**Stats Response:**
```json
{
  "stats": {
    "SessionStart": 1,
    "PreToolUse": 5,
    "PostToolUse": 5,
    "PostToolUseFailure": 1
  },
  "total_hooks": 12
}
```

**Events Response:**
```json
{
  "events": [
    {
      "hook_type": "PreToolUse",
      "timestamp": "2026-02-20T14:23:45.123Z",
      "data": {
        "session_id": "abc123",
        "hook_event_name": "PreToolUse",
        "tool_name": "Write",
        "tool_input": {"file_path": "test.py", "content": "..."}
      }
    }
  ],
  "count": 1
}
```

### 15.3 Environment Variables Summary

Complete reference in section 10.2.

### 15.4 External Resources

- Claude Code Hooks Docs: https://docs.claude.com/en/docs/claude-code/hooks
- uv Documentation: https://docs.astral.sh/uv/
- Go fatih/color: https://github.com/fatih/color
- Python requests: https://requests.readthedocs.io/

---

## 16. Acceptance Criteria

The project is complete when:

1. All files in correct directory structure
2. Server compiles and runs without errors
3. All 14 hook types configured and working (+ SessionEnd)
4. Test script runs successfully
5. All 3 documentation files present and complete
6. Makefile has all required targets
7. Colors display correctly in terminal
8. API endpoints return correct data
9. Works with real Claude Code CLI
10. Non-blocking operation verified
11. No stdout output from hook scripts
12. Event buffer stays bounded at 1000
13. hook_monitor.conf present with all hooks enabled by default
14. Disabling a hook in conf prevents events from reaching server
15. Missing/corrupt conf file doesn't break the script (fail-open)

---

## Appendix: Changelog from v1.0

| Change | Reason |
|--------|--------|
| Removed `Setup` hook | Does not exist in Claude Code |
| Added 6 missing hooks (PostToolUseFailure, SubagentStart, TeammateIdle, TaskCompleted, ConfigChange, PreCompact) | Match actual Claude Code documentation |
| Updated hook count from 10 to 14 (+SessionEnd) | Accuracy |
| Fixed test data field names (`tool_response` not `tool_output`, `content` not `file_text`) | Match actual stdin schemas |
| Added common stdin fields to all test payloads | Realism |
| Removed HOOK_TYPE env var; use `hook_event_name` from stdin | Simpler, canonical approach |
| Identical command string for all hooks | Simplifies settings.json |
| Added ring buffer (max 1000 events) | Prevent unbounded memory growth |
| Added hook stdout semantics section | Critical educational content |
| Consolidated QUICKSTART into README | Reduce doc overlap |
| Removed PROJECT_SUMMARY | Redundant with README |
| Added "no stdout" as critical requirement for hook script | stdout carries semantic meaning in hooks |
| Added ?limit query param to /events endpoint | Usability |
| Fixed Notification test data (`notification_type` not `type`) | Match actual schema |
| Fixed PermissionRequest test data (`tool_name`/`tool_input` not `tool`/`reason`) | Match actual schema |
| Fixed Stop/SubagentStop test data to match real fields | Match actual schema |
| Added PostToolUseFailure to matcher-required hooks | Accuracy |
| Added `hooks/hook_monitor.conf` toggle file (Section 4.5) | Per-hook enable/disable without editing settings.json |
| Added `is_hook_enabled()` to Python script | Config check before network call |
| Added fail-open design for config | Missing file = all hooks enabled |
| Added `show-config` and `reset-config` Makefile targets | Config management convenience |
| Added config-related test checklist items | Verify toggle behavior |
| Fixed: `configparser.optionxform = str` to preserve case | BUG: default lowercasing breaks PascalCase hook names |
| Fixed: safe int parse for `HOOK_TIMEOUT` env var | BUG: non-integer value would crash before try/except |
| Fixed: PostToolUse (Bash) test data missing `tool_input` | BUG: incomplete test payload |
| Added end-to-end test phase (stdin → Python → server) | BUG: test script only tested server, never the Python script |
| Added config toggle test phase to test script | BUG: config feature was never tested |
| Made `_monitor` key name explicit in main() spec | Ambiguity between `_metadata` (v1) and `_monitor` (v2) |
| Added config to acceptance criteria (items 13-15) | Missing from acceptance |
| Added config to README sections list | Missing from docs spec |

---

**END OF SPECIFICATION v2.2**

This document contains all information necessary to reproduce the Claude Code Hooks Monitor project from scratch using Claude Code CLI or any other development tool.
