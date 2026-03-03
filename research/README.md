# Research: From Deep Memory to Observability

This folder documents the research thread that started with "can hooks4claude
become a deep memory system for Claude?" and concluded with "the real value
is observability for the user."

## Reading Order

1. **[information-seeking-analysis.md](information-seeking-analysis.md)**
   Data analysis of 3755 captured events. 56% info-seeking, 73% redundant
   re-reads, post-compaction bursts of 38+ calls. The empirical foundation.

2. **[deep-memory-research.md](deep-memory-research.md)**
   Tree-sitter, Spectral Clustering, Obsidian-style linking — three techniques
   explored for symbol-level, pattern, and associative memory.

3. **[compaction-carry-forward.md](compaction-carry-forward.md)**
   Strategy for PreCompact hook injection. Two-layer architecture proposed.

4. **[mcp-tool-design.md](mcp-tool-design.md)**
   MCP tool design for retrieval. 4 tools proposed, honestly assessed —
   Claude won't call them unprompted.

5. **[progressive-disclosure-memory.md](progressive-disclosure-memory.md)**
   Context+'s progressive disclosure applied to time. Session lifecycle
   discovery: SessionStart fires after compaction, same session_id survives
   restarts.

6. **[conclusion.md](conclusion.md)**
   Research conclusion: deep memory for Claude doesn't work. The value is
   observability for the user. The 73% re-read finding alone justifies
   hooks4claude — it tells users where their CLAUDE.md coverage is failing.

7. **[compaction-function-carry-forward.md](compaction-function-carry-forward.md)** *(HIGH POTENTIAL)*
   Use tree-sitter + hook traces to carry actual function bodies across
   compaction — not summaries, real code. Reframes tree-sitter from grand
   memory system to surgical compaction recovery tool. Directly targets the
   73% re-read problem.

8. **[carry-forward-experiment-results.md](carry-forward-experiment-results.md)** *(FAILED)*
   Carry-forward file manifest injection — 3 A/B experiments showed injecting
   file lists after compaction increases re-reads (14 baseline vs 24-32 with
   manifest). The "shopping list effect": Claude visits every file mentioned
   regardless of framing. Approach abandoned, code removed.

9. **[work-log-vision.md](work-log-vision.md)** *(CURRENT DIRECTION)*
   hooks4claude as a centralized work log. What already works (multi-session,
   multi-project capture), what's missing (assistant responses, semantic
   summaries, dashboard, cross-session linking), and three possible directions.

10. **[logbook-features.md](logbook-features.md)** *(FEATURE SPEC)*
    13 features across 3 layers (data, query, presentation) to turn hooks4claude
    into a project logbook. 5 implementation phases. Builds on untapped fields
    discovery.

11. **[work-log-challenges.md](work-log-challenges.md)** *(ACTIONABLE)*
    Feasibility research for 4 work-log challenges. Key discovery: `last_assistant_message`
    from Stop hook already captures Claude's responses. `transcript_path` enables
    cross-session linking. Challenges 1+2 are tightly coupled and lowest effort.

## Key Findings

- 73% of file reads are redundant — the single strongest finding
- Claude's built-in compaction summary + CLAUDE.md already provides Level 0 orientation
- Claude won't query memory tools unprompted (doesn't know what it lost)
- Curated knowledge (CLAUDE.md) beats uncurated event history
- **Pivot: hooks4claude's value is observability for users, not memory for Claude**
- Actionable: re-read hotspots reveal where CLAUDE.md needs better coverage
- **Carry-forward manifests increase re-reads** — file lists become "shopping lists"
- Negative instructions backfire (Streisand effect: "do NOT re-read" → more re-reads)
