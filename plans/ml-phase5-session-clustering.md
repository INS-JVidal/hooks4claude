# Phase 5: Session Clustering

*Classify sessions by work type using K-means on feature vectors*

## Goal

Cluster sessions into types (exploration, implementation, debugging, testing, refactoring) using numerical features from hook-sessions. Store cluster labels as a new `session_type` field.

## Why

- Answers: "Am I spending more time debugging than building?"
- Enables trend analysis: session type distribution over time
- K-means on 8-33 dimensions with 200 points is statistically stable
- Interpretable clusters that map to recognizable work patterns

## Feature Vector Options

### Minimal (tool distributions only, 8 dims)

```
[%Read, %Edit, %Bash, %Grep, %Glob, %Write, %Agent, %Other]
```

Normalize tool counts to proportions. Already available in hook-sessions.

### Enhanced with topics (33 dims, requires Phases 2-3)

```
tool_dist(8) + temporal(5) + files(4) + cost(3) + LDA(8) + RAKE(5) = 33 dims
```

Where:
- temporal: duration_s, mean_inter_event_gap, stddev_gap, burst_count, longest_gap
- files: unique_files, max_depth, cross_dir_ratio, test_file_ratio
- cost: tokens_per_prompt, cost_per_event, cache_hit_ratio
- LDA: topic distribution from Phase 3
- RAKE: count of keywords in each category (debug-related, feature-related, etc.)

### Recommendation

Start with the 8-dim tool distribution. It's available now and produces meaningful clusters. Add dimensions from Phases 2-3 later to refine.

## Expected Clusters

| Cluster | Signature | Description |
|---------|-----------|-------------|
| Exploration | High %Read, high %Grep, low %Edit | Reading code, searching patterns |
| Implementation | Balanced Read/Edit, moderate Bash | Writing new code |
| Debugging | High %Bash, moderate %Read, low %Edit | Running commands, investigating |
| Refactoring | High %Edit, moderate %Read, low %Bash | Changing existing code |
| Delegation | High %Agent | Multi-agent workflows |

## Algorithm

### K-means (k=3-5)

```go
// Pseudocode
sessions := fetchAllSessions()
vectors := extractFeatureVectors(sessions)
normalize(vectors) // Z-score normalization per feature

centroids := kmeansInit(vectors, k=5)
for i := 0; i < maxIter; i++ {
    assignments := assignToClusters(vectors, centroids)
    centroids = recomputeCentroids(vectors, assignments)
}
labels := assignToClusters(vectors, centroids)
```

K-means is ~30 lines of Go. No external library needed.

### Choosing k

- Start with k=5 (matches expected cluster types)
- Use silhouette score to validate: compute for k=3,4,5,6, pick the k with highest score
- Alternatively: run DBSCAN to discover natural cluster count, then use K-means with that k

### Cluster labeling

After clustering, inspect centroids to name clusters:
- Centroid with highest %Read → "Exploration"
- Centroid with highest %Edit → "Refactoring"
- etc.

Or use LDA topic labels from Phase 3 if available.

## Implementation

### New MCP tool: `session-types`

Shows cluster distribution and per-session labels:

```
Session Types (last 30 days):
  Exploration:     12 sessions (35%)
  Implementation:  10 sessions (29%)
  Debugging:        7 sessions (20%)
  Refactoring:      4 sessions (12%)
  Delegation:       1 session  (3%)
```

### Storage

Add to SessionDocument in `store.go`:

```go
SessionType      string    `json:"session_type,omitempty"`
SessionTypeConf  float64   `json:"session_type_confidence,omitempty"`
```

Recompute labels when the model is retrained (after significant new data).

### Batch recompute script

Clustering should be rerun periodically (centroids shift as data grows):
1. Fetch all session feature vectors from MeiliSearch
2. Run K-means
3. Update session documents with new labels via MeiliSearch API

## Verification

- [ ] Clusters are separable (silhouette score > 0.3)
- [ ] Cluster labels match human intuition (manually review 10 sessions per cluster)
- [ ] Feature normalization doesn't distort the data (z-scores reasonable)
- [ ] MCP tool displays meaningful session type distribution
- [ ] Filtering by `session_type = "Debugging"` returns debugging-heavy sessions

## Files Modified/Created

- `hooks-mcp/internal/tools/sessiontypes.go` (new) — MCP tool handler
- `hooks-mcp/internal/tools/register.go` — Register session-types tool
- `hooks-store/internal/store/store.go` — Add session_type field to SessionDocument
- `hooks-store/internal/store/meili.go` — Add session_type to filterable/searchable
- `scripts/cluster_sessions.go` or `.py` (new) — Batch recompute script

## Feeds Into

- **Phase 7 (Anomaly Detection):** Session type as context for anomaly thresholds (a "Debugging" session with high Bash is normal, an "Implementation" session with high Bash is unusual)
- **Phase 8 (ONNX):** Session type labels become training targets for the multi-signal classifier
