# Carry-Forward Experiment: File Manifest Injection After Compaction

**Date:** 2026-03-01
**Status:** Failed — approach abandoned, code removed

## Hypothesis

Injecting a file manifest (list of recently-touched files with edit/read counts) into `additionalContext` after context compaction would reduce the "re-read penalty" — the burst of file reads Claude performs to regain context lost during compaction.

## Implementation

Built `internal/carryforward/` package in claude-hooks-monitor:

- **Tracker** records every file Read/Edit/Write per session via PostToolUse hooks
- **Snapshot** (triggered by PreCompact) filters files by recency (last 30% of session), formats a manifest, caches it
- **Consume** (triggered by post-compaction SessionStart) returns the manifest one-shot via `additionalContext`
- Edited files always included regardless of recency cutoff
- Cached manifests expire after 5 minutes (TTL) to prevent stale injection

Pipeline: `PostToolUse → RecordFile → PreCompact → Snapshot → SessionStart → Consume → additionalContext`

## Experiment Design

A/B test with two sessions per run:
- **Session A:** carry-forward enabled (manifest injected after compaction)
- **Session B:** carry-forward disabled (no manifest, baseline)
- **Control:** CLAUDE.md present in both sessions (only variable is carry-forward)
- **Isolation:** git worktrees per session (code changes don't affect original repo)
- **Prompt:** 6-phase intensive audit (review → bugs → simplify → docs → harden → consistency)
- **Metric:** post-compaction re-reads within 300s window after each PreCompact event

Script: `hooks-store/scripts/run-carryforward-experiment.sh`

## Results

### Run 1 — mdink (Rust, ~8K lines)

| Metric | CF ON | CF OFF |
|--------|-------|--------|
| Total tool calls | 44 | 91 |
| Exploration calls | 37 | 51 |
| Compactions | 1 | 1 |
| Post-compact re-reads | 7 (15.9%) | 17 (18.7%) |

Manifest wording: "Re-read before continuing:"

### Run 2 — claude-hooks-monitor (Go, ~3K lines)

| Metric | CF ON | CF OFF |
|--------|-------|--------|
| Total tool calls | 133 | 172 |
| Exploration calls | 86 | 105 |
| Compactions | 1 | 1 |
| Post-compact re-reads | 24 (27.9%) | 14 (13.3%) |

Manifest wording: "Re-read before continuing:"

### Run 3 — claude-hooks-monitor, revised wording

| Metric | CF ON | CF OFF |
|--------|-------|--------|
| Total tool calls | 111 | 140 |
| Exploration calls | 73 | 76 |
| Compactions | 1 | 1 |
| Post-compact re-reads | 32 (43.8%) | 14 (18.4%) |

Manifest wording: "Do NOT re-read these files unless you need to edit them. Use this list only as a memory aid."

### Summary Table

| Run | Codebase | Wording | CF ON re-reads | CF OFF re-reads | Winner |
|-----|----------|---------|----------------|-----------------|--------|
| 1 | mdink | Directive ("re-read") | 7 | 17 | CF ON |
| 2 | monitor | Directive ("re-read") | 24 | 14 | CF OFF |
| 3 | monitor | Negative ("do NOT") | 32 | 14 | CF OFF |

## Analysis

### The Shopping List Effect

When Claude receives a list of files after compaction — regardless of framing — it treats the list as implicit work items. Each file mentioned becomes something Claude "should" visit, even when explicitly told not to.

This is consistent with the finding in [20260228-2037_conclusion.md](20260228-2037_conclusion.md): "Claude won't query memory unprompted." The mirror is also true: **Claude will act on memory it didn't ask for.** Unsolicited context creates unsolicited work.

### Why Run 1 Appeared Positive

Run 1 (mdink) showed CF ON with fewer re-reads (7 vs 17), but this was likely noise:
- mdink is a larger codebase (~8K lines Rust vs ~3K lines Go)
- Only 1 compaction per session — insufficient data
- Natural variance in how Claude approaches the 6-phase prompt

The two runs on claude-hooks-monitor (Runs 2-3) were more controlled and both showed CF ON performing worse — with a stable CF OFF baseline of exactly 14 re-reads.

### The Streisand Effect

Changing the wording from "Re-read before continuing" to "Do NOT re-read" made things worse (24 → 32 post-compact re-reads). Telling Claude about files it shouldn't re-read made it think about those files more.

Three framing strategies all failed:
1. **Directive** ("Re-read before continuing") — Claude obeys and re-reads everything listed
2. **Informational** ("Files you were editing") — Claude treats the list as a todo
3. **Negative** ("Do NOT re-read") — Streisand effect, even more re-reads

### CF OFF Baseline is Already Efficient

Without any carry-forward intervention, Claude's natural post-compaction behavior is remarkably consistent:
- 14 re-reads on claude-hooks-monitor (both Run 2 and Run 3)
- 17 re-reads on mdink (Run 1)

Claude's built-in compaction summary + CLAUDE.md re-injection already provides enough orientation. The post-compaction reads are Claude getting exact file content for editing — something no summary can replace.

## Conclusion

**File manifest carry-forward does not reduce post-compaction re-reads. It increases them.**

The mechanism is fundamentally flawed: listing files in post-compaction context gives Claude a "shopping list" it feels compelled to visit. The problem isn't the wording or framing — it's the act of presenting a file list itself.

This reinforces the project's pivot toward **observability for users** rather than **memory for Claude**. The 73% re-read finding from [20260228-1913_information-seeking-analysis.md](20260228-1913_information-seeking-analysis.md) tells users where their CLAUDE.md coverage is failing — that's the actionable insight, not trying to inject context that Claude will misinterpret.

## Code Disposition

- Carry-forward code removed from claude-hooks-monitor via `git reset --hard` to commit `43bd191`
- Force-pushed to origin/main
- Experiment script preserved in `hooks-store/scripts/run-carryforward-experiment.sh`
- All monitoring infrastructure remains useful independently

## Bugs and Fixes During Experimentation

| Bug | Fix |
|-----|-----|
| Bash `${VAR:-default}` fails with parentheses in multi-line strings | Use heredoc (`cat <<'EOF'`) |
| hook-client assumed per-repo, copied to worktrees | Recognized as global binary (`~/.local/bin`), preflight check only |
| Wrong session detected (concurrent sessions from other projects) | Added `verify_session_cwd()` to match session cwd against target repo |
| COMPACT_WINDOW too short (120s) — missed late re-reads | Increased default to 300s across all scripts |
| Hardcoded "with/without CLAUDE.md" labels in analysis script | Made configurable via LABEL_A/LABEL_B env vars |
