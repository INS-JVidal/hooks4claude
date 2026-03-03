# Use Cases for Hook Monitoring & Data Storage

Living document of ideas for what the captured hook event data can be used for.

## Current Data Available

10,459+ events across 63 sessions, including:
- Every tool call (Read, Write, Edit, Bash, Grep, Glob, etc.)
- User prompts (what was asked)
- Claude's final responses (last_message on Stop events)
- Session lifecycle (startup, resume, compact, clear)
- File paths touched, working directories, project dirs
- Token usage and cost per session
- Transcript paths linking related sessions
- Model identifiers

---

## 1. Logbook — Queryable Work History

Query the database to retrieve exact details of past work, recent or old.

**Examples:**
- "What did I work on last Tuesday?"
- "When did I last modify the authentication module?"
- "Show me all prompts related to the database migration"
- "What was Claude's response when we discussed the API redesign?"

**Data used:** prompts, last_message, timestamps, session_id, tool calls

---

## 2. Session Replay / Timeline

Reconstruct what happened in a session step by step: what was asked, what tools were used, what files were read and written, and what Claude said at the end.

**Examples:**
- "Walk me through session X from start to finish"
- "What files were touched during yesterday's refactoring session?"
- "Show the sequence of tool calls in the last session"

**Data used:** session_id, hook_type, tool_name, file_path, timestamps, transcript_path

---

## 3. Cost & Usage Analytics

Track spending and token consumption across sessions, projects, and time periods.

**Examples:**
- "How much have I spent this week?"
- "Which project consumes the most tokens?"
- "What's the average cost per session?"
- "Show cost trend over the last month"

**Data used:** cost_usd, input_tokens, output_tokens, cache_read_tokens, project_dir, timestamps

---

## 4. Development Pattern Analysis

Understand how you use Claude Code — which tools are used most, what types of tasks you do, how sessions evolve.

**Examples:**
- "What percentage of tool calls are file reads vs writes?"
- "How often do I hit permission requests?"
- "What's my average session length?"
- "Do I tend to work in short bursts or long sessions?"

**Data used:** hook_type, tool_name, session_id, timestamps, source (compact frequency)

---

## 5. Project Activity Dashboard

Per-project view of all Claude Code activity — when work happened, what was done, which files are most active.

**Examples:**
- "Show all activity on hooks4claude this week"
- "Which files in this project have been edited the most?"
- "How many sessions have worked on this project?"

**Data used:** project_dir, cwd, file_path, session_id, timestamps

---

## 6. Error & Failure Tracking

Monitor what goes wrong — tool failures, permission denials, patterns of errors.

**Examples:**
- "Show all errors from the last 3 days"
- "Which tools fail most often?"
- "Are there recurring permission issues?"

**Data used:** error_message, hook_type (PostToolUseFailure), tool_name

---

## 7. Context Compaction Insights

Track when and how often context compaction happens, which helps understand session complexity and context window pressure.

**Examples:**
- "How many compactions happened this week?"
- "Which projects trigger the most compactions?"
- "What's the average number of events before compaction?"

**Data used:** source="compact" on SessionStart events, session_id, timestamps

---

## 8. Cross-Session Knowledge Recovery

When a session runs out of context and continues in a new one, the transcript_path links them. Query across session boundaries to recover lost context.

**Examples:**
- "What was being discussed before this session's compaction?"
- "Show me the full history of this conversation across continuations"
- "Find the session where we first implemented feature X"

**Data used:** transcript_path, source, session_id, prompts, last_message

---

## 9. File Impact Analysis

Track which files are read and modified most frequently, helping identify hotspots, coupling, and areas that need attention.

**Examples:**
- "Which files have been edited in the last 10 sessions?"
- "What files are always read together?" (coupling detection)
- "Show the most frequently touched files this month"

**Data used:** file_path, tool_name (Read/Write/Edit), session_id

---

## 10. Prompt Patterns & Improvement

Analyze your own prompting habits — what works, what leads to long sessions, what types of requests are most common.

**Examples:**
- "What are my most common types of requests?"
- "Which prompts led to the longest sessions?"
- "Show prompts that resulted in errors"

**Data used:** prompt (from hook-prompts index), session_id, error_message, token counts

---

## 11. Team Awareness (Multi-Instance)

Since the database captures events from all Claude Code instances, it can show what's happening across concurrent sessions.

**Examples:**
- "Are there other sessions working on this project right now?"
- "What was changed in this project while I was away?"
- "Show all sessions active in the last hour"

**Data used:** session_id, project_dir, timestamps, cwd

---

## 12. Audit Trail

Maintain a verifiable record of all AI-assisted code changes for compliance, security review, or personal accountability.

**Examples:**
- "Show all file writes made by Claude in production code this week"
- "List all Bash commands executed in the last 24 hours"
- "What changes were made to security-sensitive files?"

**Data used:** tool_name, file_path, hook_type, timestamps, data (command details)

---

## Implementation Priority

| Use Case | Complexity | Value | Status |
|----------|-----------|-------|--------|
| 1. Logbook | Medium | High | Data foundation done (Phase 1) |
| 2. Session Replay | Medium | High | Data available |
| 3. Cost Analytics | Low | Medium | Data available |
| 4. Pattern Analysis | Low | Medium | Data available |
| 5. Project Dashboard | Medium | High | Data available |
| 6. Error Tracking | Low | Medium | Data available |
| 7. Compaction Insights | Low | Low | Data available |
| 8. Cross-Session Recovery | High | High | transcript_path extracted |
| 9. File Impact | Low | Medium | file_path extracted |
| 10. Prompt Patterns | Low | Medium | Dedicated prompts index exists |
| 11. Team Awareness | Medium | Medium | Multi-instance works already |
| 12. Audit Trail | Low | High | Data available |

All use cases can be queried against the existing MeiliSearch data today. The missing piece is a query interface (hooks-log CLI or similar) to make these accessible without writing raw curl commands.
