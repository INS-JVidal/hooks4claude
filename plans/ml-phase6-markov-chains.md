# Phase 6: Markov Chain Workflow Patterns

*Model tool usage sequences to discover workflow patterns and detect anomalies*

## Goal

Build transition probability matrices from tool usage sequences across sessions. Expose workflow patterns via a new MCP tool: "What usually follows a Read?" "Is this session's tool flow unusual?"

## Why

- Existing `tool-usage` shows counts; Markov adds sequential information
- ~20K transitions across 200 sessions is statistically robust for order 1-2 chains
- Trivial to implement in Go — a map of maps
- Enables anomaly scoring: "this session's tool transitions deviate from historical patterns"

## Algorithm

### Order-1 Markov Chain

Build transition matrix P(next_tool | current_tool):

```
From\To    Read  Edit  Bash  Grep  Glob  Write  Agent  Other
Read       0.45  0.15  0.10  0.10  0.08  0.05   0.02   0.05
Edit       0.20  0.30  0.25  0.05  0.02  0.10   0.03   0.05
Bash       0.25  0.10  0.30  0.15  0.05  0.03   0.07   0.05
...
```

### Order-2 (bigram context)

P(next_tool | current_tool, previous_tool):
- Captures patterns like Read→Read→Edit (reading before editing)
- 10x10x10 = 1000 cells, but most are zero — use sparse map
- Needs more data to be reliable; start with order-1

### Session anomaly score

For a session's tool sequence [t1, t2, t3, ...]:
```
log_likelihood = sum(log(P(t_{i+1} | t_i))) for all i
perplexity = exp(-log_likelihood / N)
```

High perplexity = unusual tool flow. Compare against historical distribution.

## Implementation

### Data extraction

Query hook-events for PreToolUse events ordered by timestamp within each session:

```json
{
  "filter": "hook_type = PreToolUse AND session_id = 'abc-123'",
  "sort": ["timestamp_unix:asc"],
  "limit": 1000,
  "attributesToRetrieve": ["tool_name", "timestamp_unix"]
}
```

### Go data structures

```go
type MarkovChain struct {
    Transitions map[string]map[string]int    // counts
    Totals      map[string]int               // row totals for normalization
}

func (mc *MarkovChain) Add(from, to string)
func (mc *MarkovChain) Probability(from, to string) float64
func (mc *MarkovChain) TopTransitions(from string, n int) []Transition
func (mc *MarkovChain) Perplexity(sequence []string) float64
```

### New MCP tool: `workflow-patterns`

Two modes:

**Mode 1: Show transition matrix**
```
Tool Transition Probabilities:
  After Read:   Read (45%), Edit (15%), Bash (10%), Grep (10%)
  After Edit:   Edit (30%), Bash (25%), Read (20%), Write (10%)
  After Bash:   Bash (30%), Read (25%), Grep (15%), Edit (10%)
```

**Mode 2: Score a session's workflow**
```
Session abc-123 workflow analysis:
  Perplexity: 3.2 (typical range: 2.5-4.0)
  Unusual transitions:
    Bash → Agent (observed 5x, expected <1x)
    Glob → Write (observed 3x, never seen before)
```

## Verification

- [ ] Transition matrix sums to ~1.0 per row
- [ ] Top transitions match intuition (Read→Read is common, Read→Agent is rare)
- [ ] Perplexity scores distinguish normal vs unusual sessions
- [ ] MCP tool output is readable and useful

## Files Modified/Created

- `hooks-mcp/internal/tools/workflow.go` (new) — MCP tool handler
- `hooks-mcp/internal/tools/register.go` — Register workflow-patterns tool
- `hooks-mcp/internal/markov/` (new package) — MarkovChain implementation

## Feeds Into

- **Phase 7 (Anomaly Detection):** Perplexity as an additional anomaly signal
- **Phase 8 (ONNX):** Markov features (top transition probabilities for session's most-used tool) in the fingerprint
