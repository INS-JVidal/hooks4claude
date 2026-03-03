# Phase 7: Z-score Anomaly Detection

*Flag unusual sessions using statistical thresholds on historical data*

## Goal

Compute per-feature z-scores for each session against historical baselines. Flag sessions that deviate >2 standard deviations on any dimension. Add anomaly flags to session-summary output.

## Why

- Lowest effort of all phases (1-2 days)
- Z-scores are trivially implementable in Go — no external dependencies
- Catches multi-dimensional anomalies: "normal cost but 10x the error rate"
- Extends the existing `cost-analysis` MCP tool with richer context
- At 200 sessions, z-scores are as effective as Isolation Forest (revisit at 1000+)

## Features to Monitor

From SessionDocument (14 numerical features):

| Feature | What it catches |
|---------|----------------|
| duration_s | Unusually long/short sessions |
| total_events | Sessions with abnormal event volume |
| total_prompts | Sessions with unusual interaction count |
| compaction_count | Sessions hitting context limits |
| read_count | Excessive file reading (orientation loops) |
| edit_count | Unusually heavy editing |
| bash_count | Command-heavy sessions |
| grep_count, glob_count | Search-heavy sessions |
| write_count | File creation spikes |
| agent_count | Delegation-heavy sessions |
| total_cost_usd | Cost spikes |
| input_tokens | Token budget anomalies |
| output_tokens | Response verbosity anomalies |

## Algorithm

### 1. Compute historical statistics

```go
type FeatureStats struct {
    Mean   float64
    StdDev float64
    Min    float64
    Max    float64
    N      int
}

// Compute from all sessions in hook-sessions index
stats := computeStats(allSessions)
```

### 2. Score each session

```go
type AnomalyReport struct {
    SessionID    string
    OverallScore float64          // max absolute z-score across features
    Flags        []AnomalyFlag    // features that exceeded threshold
}

type AnomalyFlag struct {
    Feature  string
    Value    float64
    ZScore   float64
    Severity string  // "warning" (2-3 sigma) or "alert" (>3 sigma)
}
```

### 3. Threshold

- |z| > 2.0: warning (roughly 5% of sessions)
- |z| > 3.0: alert (roughly 0.3% of sessions)
- With 200 sessions: expect ~10 warnings and ~1 alert per feature

## Implementation

### Extend session-summary MCP tool

Add an anomaly section to the existing `session-summary` output:

```
Session abc-123 — Anomaly Analysis:
  Overall: NORMAL (max z-score: 1.3)

Session def-456 — Anomaly Analysis:
  Overall: WARNING (max z-score: 2.8)
  Flags:
    bash_count:    47 (z=2.8, mean=15, stddev=11) ⚠ WARNING
    total_cost_usd: $3.20 (z=2.1, mean=$0.85, stddev=$1.10) ⚠ WARNING
```

### New MCP tool: `anomaly-scan`

Scan all recent sessions and rank by anomaly score:

```
Anomaly Scan (last 30 days):
  ALERT   session abc-123  cost=$12.50 (z=4.1)  bash=89 (z=3.5)
  WARNING session def-456  duration=45min (z=2.8)
  WARNING session ghi-789  edits=120 (z=2.3)
  NORMAL  27 other sessions
```

### Where to compute

**Option A: Query-time (recommended)**
Compute stats from all sessions, score the requested session. ~10ms for 200 sessions. No storage needed.

**Option B: Precomputed**
Store `anomaly_score` and `anomaly_flags` on SessionDocument. Requires periodic recomputation as baselines shift.

Recommend Option A for simplicity — recompute on demand.

## Context-Aware Thresholds (enhancement)

If Phase 5 (Session Clustering) is complete, use per-cluster baselines:
- A "Debugging" session with 47 Bash calls may be normal for debugging but anomalous for exploration
- Compute separate stats per session_type, score against the matching cluster

## Verification

- [ ] Z-scores are correctly computed (spot-check against manual calculation)
- [ ] Known unusual sessions (from memory: high-cost experiments) are flagged
- [ ] Normal sessions are NOT flagged (false positive rate < 10%)
- [ ] anomaly-scan ranks sessions by severity correctly
- [ ] session-summary anomaly section displays cleanly

## Files Modified/Created

- `hooks-mcp/internal/tools/anomaly.go` (new) — anomaly-scan MCP tool
- `hooks-mcp/internal/tools/session.go` — Extend session-summary with anomaly section
- `hooks-mcp/internal/tools/register.go` — Register anomaly-scan tool
- `hooks-mcp/internal/stats/` (new package) — FeatureStats computation and z-score functions

## Feeds Into

- **Phase 8 (ONNX):** Anomaly score as an additional feature in the multi-signal fingerprint. Or: replace z-scores with ONNX Isolation Forest at 1000+ sessions.
