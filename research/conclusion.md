# Research Conclusion: Observability Over Retrieval

**Date:** 2026-02-28

## The Question

Can hooks4claude become a deep memory system where Claude queries its own past?

## The Answer

No. The value is observability for the user, not memory for Claude.

## Why Deep Memory for Claude Doesn't Work

1. **Claude already handles compaction.** Built-in summary + CLAUDE.md re-injection provides Level 0 orientation. A carry-forward hook adds marginal Level 1 detail.

2. **Editing requires exact content.** The 38-call post-compaction burst is mostly Claude re-reading files to get exact line content for editing. No summary replaces that — Claude must Read the file.

3. **Claude won't query memory unprompted.** After compaction, Claude doesn't know what it lost. It won't call an MCP tool for knowledge it doesn't know is missing.

4. **Cross-session retrieval solves a rare problem.** Users restart context by re-explaining their intent, not by asking Claude to recall past sessions.

5. **Curated beats uncurated.** A good CLAUDE.md provides better project orientation than querying 3755 raw events. Manual curation has higher signal-to-noise.

## Where the Real Value Is

hooks4claude makes Claude's behavior **visible to the user**. The data answers questions no other tool can:

| Insight | What the user learns | Action |
|---------|---------------------|--------|
| 73% of reads are redundant re-reads | CLAUDE.md coverage is insufficient for hot files | Write better file summaries in CLAUDE.md |
| 38-call post-compaction bursts | Compaction is expensive; long sessions waste tokens | Break work into shorter, focused sessions |
| 56% of tool calls are info-seeking | Claude spends more time seeking than acting | Improve project documentation to reduce seeking |
| Top re-read files: main.go (81x), monitor.go (68x) | These files lack sufficient CLAUDE.md coverage | Add detailed summaries for these specific files |
| 51 PostToolUseFailure events | Error-prone areas in the workflow | Investigate common failure patterns |

## The Pivot

**From:** hooks4claude as a retrieval-based deep memory for Claude
**To:** hooks4claude as an observability tool that helps users optimize their Claude Code workflow

The most impactful output of the entire research is a single finding: **73% of file reads are redundant.** That tells the user exactly where to invest effort (better CLAUDE.md) to get immediate, measurable improvement — without building any new retrieval infrastructure.

## What This Means for Implementation

- **Drop:** MCP server for Claude-facing retrieval, PreCompact carry-forward hook
- **Keep:** MeiliSearch indexing (the recording is the foundation of observability)
- **Build:** User-facing analysis tools — dashboards, reports, CLAUDE.md optimization suggestions
- **Keep researching:** Session lifecycle patterns, cost analysis, workflow fingerprints — all serve the observability angle
