# Phase 4: Blast Radius / File Co-modification

*Discover which files are architecturally coupled from historical access patterns*

## Goal

Build a file co-modification graph from `files_read[]` and `files_written[]` across all sessions. Compute association rules and graph centrality to answer: "When I edit file X, which other files usually need changes too?"

## Why

- Directly actionable: change impact warnings before you start editing
- No ML needed — association rules and graph algorithms are deterministic
- 100-200 sessions with 5-50 files each provides sufficient co-occurrence signal
- Existing `tool-usage` MCP tool shows flat file lists; this adds the relational dimension

## Algorithm

### 1. Build co-occurrence matrix

For each session, record which files appeared together:

```
Session A: files_written = [server.go, server_test.go, config.go]
Session B: files_written = [server.go, server_test.go]
Session C: files_written = [config.go, main.go]

Co-occurrence counts:
  server.go ↔ server_test.go: 2
  server.go ↔ config.go: 1
  server_test.go ↔ config.go: 1
  config.go ↔ main.go: 1
```

### 2. Association rules (support + confidence)

```
Support(A, B) = sessions_with_both / total_sessions
Confidence(A → B) = sessions_with_both / sessions_with_A

Example:
  Confidence(server.go → server_test.go) = 2/2 = 100%
  "Every time you edited server.go, you also edited server_test.go"
```

Threshold: confidence >= 0.6 and support >= 3 sessions (to avoid noise from rare file pairs).

### 3. Graph centrality

Build undirected graph: files as nodes, co-occurrence count as edge weight.

Compute:
- **Degree centrality:** Files connected to most other files → "hub" files (e.g., config.go, CLAUDE.md)
- **Betweenness centrality:** Files that bridge different file clusters → cross-cutting concerns
- **Community detection:** Clusters of files that co-occur together → architectural modules

### 4. Output: blast radius prediction

Given a file about to be edited, look up its top co-modified files:

```
Editing: server.go
Likely also needs changes:
  server_test.go  (confidence: 100%, 5 co-sessions)
  config.go       (confidence: 75%, 3 co-sessions)
  CLAUDE.md       (confidence: 60%, 3 co-sessions)
```

## Implementation

### New MCP tool: `blast-radius`

In `hooks-mcp/internal/tools/`, add a new tool that:
1. Takes a file path as input
2. Queries hook-sessions index for all sessions containing that file in files_read or files_written
3. Builds co-occurrence from those sessions' file lists
4. Returns top co-modified files with confidence scores

### Data source

Query `hook-sessions` index:
```json
{
  "filter": "files_written EXISTS",
  "limit": 1000,
  "attributesToRetrieve": ["session_id", "files_read", "files_written"]
}
```

### Go packages

- `gonum/graph` for centrality computation
- Or implement co-occurrence matrix as `map[string]map[string]int` (simpler, sufficient)

### Alternative: precomputed graph document

Store the full co-modification graph as a document in a new MeiliSearch index (`hook-file-graph`):

```json
{
  "file": "internal/server/server.go",
  "co_modified": [
    {"file": "internal/server/server_test.go", "confidence": 1.0, "sessions": 5},
    {"file": "internal/config/config.go", "confidence": 0.75, "sessions": 3}
  ],
  "centrality": 0.85,
  "community": "server-cluster"
}
```

Recompute periodically (daily or after N new sessions).

## Verification

- [ ] Co-occurrence matrix captures expected patterns (test files pair with source files)
- [ ] Confidence scores are meaningful (>0.6 threshold filters noise)
- [ ] Centrality identifies the "hub" files in the project
- [ ] MCP tool returns useful predictions when given a file path
- [ ] Community detection groups files by actual module boundaries

## Files Modified/Created

- `hooks-mcp/internal/tools/blastradius.go` (new) — MCP tool handler
- `hooks-mcp/internal/tools/register.go` — Register blast-radius tool
- `hooks-mcp/internal/meili/client.go` — Add method to fetch session file lists

## Feeds Into

- **Phase 5 (Session Clustering):** File access breadth as a clustering feature
- **Phase 8 (ONNX):** File features (unique files, max depth, cross-directory ratio) in the fingerprint
- **AST analysis (future):** Tree-sitter can provide ground-truth import dependencies to validate co-modification patterns
