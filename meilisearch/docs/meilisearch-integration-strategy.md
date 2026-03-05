# MeiliSearch Integration Strategy for Claude Code

How Claude Code should interact with the hooks4claude MeiliSearch database,
and the extension points available to improve the experience.

## Problem: Raw curl + jq from Bash

The current approach uses `curl` commands piped through `jq` inside the Bash
tool. This works but has consistent friction points observed across sessions:

| Problem | Impact |
|---------|--------|
| **Sandbox blocks localhost** | Every `curl` needs `dangerouslyDisableSandbox: true` — extra permission prompts, easy to forget |
| **Filter syntax errors** | MeiliSearch string filters need escaped quotes (`cwd = \"/path/...\"`). Easy to get wrong, wastes a turn |
| **Session ID prefix mismatch** | Short IDs (e.g., `af7deb64`) can't be used in filters — requires a 2-step lookup to resolve the full UUID |
| **Boilerplate repetition** | Same `curl -s 'http://localhost:7700/indexes/...' -H 'Content-Type: application/json'` in every call |
| **jq complexity** | Non-trivial analysis requires 50-100 line jq programs that are fragile and hard to debug |
| **No schema awareness** | Must read `meilisearch-query-guide.md` every session to recall which fields are filterable vs searchable |
| **Timestamp arithmetic** | "Last 3 days" requires manual `date -d` conversion to Unix timestamps |

## Claude Code Extension Points

All available mechanisms for extending Claude Code, evaluated for MeiliSearch integration:

| Extension Point | Location | Description | Fit |
|----------------|----------|-------------|-----|
| **MCP Server** | `.mcp.json` or `claude mcp add` | External tool server exposing structured tools | Best — replaces curl entirely |
| **Slash command** | `.claude/commands/*.md` | Prompt template invoked with `/<name>` | Good — encodes schema context |
| **Custom agent** | `.claude/agents/*.md` | Specialized sub-agent with scoped instructions | Good — complex multi-query analysis |
| **Rules** | `.claude/rules/*.md` | Always-loaded instructions | Partial — can encode conventions |
| **Skills** | Installable plugins | Prompt + tool bundles | Possible but heavy for this use case |
| **Hooks** | `.claude/settings.json` | Shell scripts triggered by Claude Code events | Not relevant for querying |
| **Shell scripts** | `hooks-store/scripts/` | Standalone analysis scripts | Current approach — works outside Claude Code |
| **CLAUDE.md** | Project root and subdirs | Project instructions loaded into context | Already references query guide |

## Recommended Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Layer 3: Custom Agent (.claude/agents/data-analyst.md) │
│  → Complex multi-query analysis tasks                   │
├─────────────────────────────────────────────────────────┤
│  Layer 2: Slash Command (.claude/commands/query-hooks)  │
│  → Schema context + canned query patterns               │
├─────────────────────────────────────────────────────────┤
│  Layer 1: MeiliSearch MCP Server                        │
│  → Replaces all curl commands, no sandbox issues        │
├─────────────────────────────────────────────────────────┤
│  .mcp.json (committed) → team-shareable config          │
└─────────────────────────────────────────────────────────┘
```

Each layer is independently useful and incrementally adoptable.

### Layer 1: MeiliSearch MCP Server

Install the official MeiliSearch MCP server. This eliminates curl boilerplate
and sandbox issues in one step.

**Setup:**

```bash
# Install and configure (project-scoped, shareable via .mcp.json)
claude mcp add --transport stdio --scope project meilisearch -- uvx -n meilisearch-mcp

# Or add to .mcp.json directly:
```

```json
{
  "mcpServers": {
    "meilisearch": {
      "command": "uvx",
      "args": ["-n", "meilisearch-mcp"],
      "env": {
        "MEILI_HTTP_ADDR": "http://localhost:7700"
      }
    }
  }
}
```

**Tools exposed:** `search`, `get-documents`, `add-documents`, `create-index`,
`list-indexes`, `get-index-metrics`, `get-settings`, `update-settings`,
`health-check`, `get-version`, `get-stats`, `get-system-info`, `get-task`,
`get-tasks`, and more.

**What it solves:**
- No sandbox issues (MCP server runs outside the sandbox)
- No curl boilerplate
- Structured tool interface instead of string-interpolated shell commands
- Persistent connection to MeiliSearch

**What it doesn't solve:**
- No awareness of our schema (hook_type, session_id, indexes)
- Still need to know filter syntax and field names
- No domain helpers (e.g., "last 3 days" still requires timestamp math)

**Source:** [github.com/meilisearch/meilisearch-mcp](https://github.com/meilisearch/meilisearch-mcp)

### Layer 2: Slash Command

A `.claude/commands/query-hooks.md` file that encodes our schema knowledge.
When invoked as `/query-hooks <question>`, it provides Claude with the full
schema context so it can construct correct queries without re-reading docs.

**What it should contain:**
- Index names and their purpose (`hook-events`, `hook-prompts`, `hook-sessions`)
- Field reference table (filterable, sortable, searchable)
- Common query patterns (session overview, prompt timeline, file analysis)
- Conventions (use `hook-prompts` for prompts, `PostToolUse` for confirmed actions)
- Timestamp helpers (how to derive Unix timestamps from date strings)

**What it solves:**
- Schema awareness without reading the full query guide
- Consistent query patterns across sessions
- Reduces errors from forgetting field names or filter syntax

### Layer 3: Custom Agent

A `.claude/agents/data-analyst.md` file defining a sub-agent specialized for
MeiliSearch data analysis. Spawned via the Agent tool for complex tasks.

**When to use:**
- Multi-query analysis (activity trees, session comparisons)
- Tasks that require classification, aggregation, or visualization
- Parallel data extraction while the main conversation continues

**What it should contain:**
- Full schema reference
- Instructions to resolve session ID prefixes before filtering
- Common analysis patterns (classify by submodule, timeline visualization)
- Output formatting conventions

## Alternative: Custom MCP Server

For teams doing frequent analysis, a custom MCP server wrapping MeiliSearch
with domain-specific tools could replace Layers 1-2:

| Tool | Parameters | What it does |
|------|-----------|-------------|
| `query-sessions` | project, date_range, submodule | Pre-filtered session list |
| `query-prompts` | project, session_id_prefix, days | Prompts with auto-resolved IDs |
| `session-summary` | session_id_prefix | Formatted overview of one session |
| `project-activity` | project, date_range | Activity tree visualization |

This is more work but provides the tightest integration. Consider this if
the layered approach proves insufficient.

## Key Configuration Files

| File | Purpose |
|------|---------|
| `.mcp.json` | MCP server config — committed to git, auto-configures for all users |
| `.claude/commands/query-hooks.md` | Schema-aware slash command |
| `.claude/agents/data-analyst.md` | Analysis sub-agent definition |
| `docs/meilisearch-query-guide.md` | Full schema reference (existing) |
| `docs/meilisearch-integration-strategy.md` | This document |

## Implementation Order

1. **Layer 1** — Install MeiliSearch MCP server, add `.mcp.json` (~5 min)
2. **Layer 2** — Write `/query-hooks` slash command (~15 min)
3. **Layer 3** — Write data-analyst agent (~15 min)
4. **Optional** — Custom MCP server if Layers 1-3 prove insufficient
