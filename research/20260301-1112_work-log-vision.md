# hooks4claude as a Work Log System

**Date:** 2026-03-01
**Status:** Vision / next direction

## Core Idea

hooks4claude is already a centralized, multi-session, multi-project work log. Every Claude Code instance — regardless of terminal or project — fires global hooks through the same pipeline. MeiliSearch captures and indexes everything.

## What Already Works

The infrastructure captures a complete tool-level timeline across all projects:

- **Multi-session capture**: Every Claude Code CLI instance fires the same hooks
- **Multi-project**: `data.cwd` identifies which project each event belongs to
- **Session grouping**: `session_id` groups events into logical work chunks
- **Full tool trace**: every Read, Edit, Write, Bash, Glob, Grep call is recorded
- **User prompts**: `UserPromptSubmit` captures what the user asked
- **Searchable**: MeiliSearch provides instant filtering by project, time, tool, session

### What this gives you today

| Capability | How |
|------------|-----|
| "What did Claude do on project X?" | Filter by `data.cwd` |
| "What files were edited today?" | Filter by `hook_type=PostToolUse`, `tool_name=Edit`, time range |
| "How many sessions this week?" | Facet on `session_id` with time filter |
| "What prompts did I give?" | Filter `hook_type=UserPromptSubmit` |
| "Show me the compaction events" | Filter `hook_type=PreCompact` |
| "Which session touched file Y?" | Search for file path in event data |

## What's Missing

The gap between "tool call log" and "work transcription":

### 1. No Assistant Responses
Claude Code hooks don't expose Claude's output text. We capture what Claude *did* (tool calls) but not what Claude *said* (explanations, analysis, decisions). The narrative is missing.

### 2. No Semantic Summaries
Events are tool-level: "Read file X", "Edit file Y". There's no higher-level record of "Implemented authentication module" or "Fixed race condition in worker pool." The *what was accomplished* layer doesn't exist.

### 3. No Historical Dashboard
The TUI shows live events as they happen. There's no way to browse historical sessions, see a timeline of work across projects, or get a weekly summary of activity.

### 4. No Cross-Session Linking
When a session continues from a previous one (context restoration), the two sessions share no explicit link. A multi-day feature implementation spans multiple sessions with no thread connecting them.

### 5. No Project-Level Aggregation
Each event is atomic. There's no rollup showing "project X: 5 sessions, 342 tool calls, 23 files modified this week."

## Possible Directions

### A. Presentation Layer (low effort, high value)
Build a dashboard/CLI that queries MeiliSearch and presents historical data:
- Session timeline view (when, which project, how many events)
- Project activity summary (files touched, tools used, session count)
- Weekly/daily work reports generated from event data
- Search across all sessions ("when did I last work on authentication?")

### B. Enrichment Layer (medium effort)
Post-process events to add semantic meaning:
- Cluster tool calls into "tasks" (consecutive edits to related files = one task)
- Extract file-level summaries from edit patterns (file X: 5 edits over 3 sessions)
- Detect session themes from prompts + file patterns
- Link continued sessions via overlapping file access patterns

### C. Extended Capture (depends on Claude Code hook evolution)
If new hook types become available:
- Assistant response text (the missing narrative)
- Conversation-level summaries (task completion signals)
- Plan mode outputs (architectural decisions)

## Architecture Consideration

The current pipeline is already right:

```
Claude Code → hook-client → monitor → hooks-store → MeiliSearch
                                                        ↓
                                              [new] dashboard / CLI
                                              [new] enrichment worker
```

New features build *on top of* MeiliSearch, not inside the capture pipeline. The capture layer is stable and complete for what hooks expose.

## Relationship to Failed Experiments

The carry-forward experiment ([20260301-1102_carry-forward-experiment-results.md](20260301-1102_carry-forward-experiment-results.md)) tried to use captured data to improve Claude's behavior. That failed — Claude misinterprets injected context.

This vision is different: the data serves the **user**, not Claude. The user reviews their work history, tracks progress, and understands patterns. This aligns with the project's core finding: **hooks4claude's value is observability for humans, not memory for machines.**
