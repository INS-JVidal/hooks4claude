# ML on Hook Event Data: Suitability Analysis

*Research document — March 2026*

## Context

hooks4claude captures rich operational telemetry from Claude Code sessions: ~10-20K events across ~100-200 sessions in 3 MeiliSearch indexes (`hook-events`, `hook-prompts`, `hook-sessions`). The current 8 MCP tools provide deterministic, rule-based analysis (cost aggregation, tool distribution, error listing). This document evaluates where ML adds genuine value beyond these heuristics, given the small dataset, local single-developer deployment, and Go/Rust tech stack.

### Data Available

| Index | Documents | Key fields |
|-------|-----------|------------|
| `hook-events` | ~10-20K | hook_type, session_id, tool_name, timestamp_unix, file_path, error_message, prompt, cost_usd, input/output_tokens, data_flat |
| `hook-prompts` | ~233 | prompt, prompt_length, session_id, timestamp_unix |
| `hook-sessions` | ~100-200 | duration_s, total_events, total_prompts, compaction_count, read/edit/bash/grep/glob/write/agent_count, total_cost_usd, files_read[], files_written[], prompt_preview |

---

## Honest Assessment: ML vs Heuristics at This Scale

| ML clearly wins | ML marginally wins | Simple heuristics win |
|-----------------|--------------------|-----------------------|
| Prompt embeddings (semantic search) | Session clustering (vs manual rules) | Error prediction (87 examples too few) |
| Code change embeddings (CodeBERT) | Markov chains (vs frequency counts) | Anomaly detection (z-scores sufficient) |
| Multi-signal session fingerprinting | | |

**The uncomfortable truth:** at 100-200 sessions and 10-20K events, most ML approaches are either premature or beaten by simple statistics. The one clear exception is **prompt embeddings** -- pre-trained models need zero training data and fill a genuine capability gap (semantic search).

---

## Evaluated Approaches

### Tier 1: Start Now (high value, data-ready)

#### 1. Prompt Embeddings & Semantic Search

**Rating:** Best fit for the data

- **What:** Embed 233 user prompts with a small model (all-MiniLM-L6-v2 or nomic-embed-text), enable semantic similarity search
- **Why ML wins:** MeiliSearch keyword search on `data_flat` cannot answer "find sessions where I worked on something like authentication" -- embeddings can
- **Data sufficiency:** Pre-trained models need zero training examples. 233 prompts is plenty for similarity computations
- **Inference:** ONNX MiniLM = ~5ms/prompt, ~90MB model. Ollama nomic-embed-text = ~50-500ms/prompt, ~1-4GB
- **Storage:** 233 x 384 floats = ~350KB. MeiliSearch v1.36 has built-in vector search support
- **Gain:** Semantic prompt search, session similarity, prompt deduplication, recurring-theme discovery
- **Cons:** Adds ONNX or Ollama dependency; vector storage needs solving (MeiliSearch vectors or external)
- **Path:** Ollama for prototyping -> ONNX in Rust/Go for production if latency matters

#### 2. Blast Radius / File Co-modification

**Rating:** High practical value, no ML needed

- **What:** Build co-occurrence matrix from `files_read[]`/`files_written[]` across sessions. Association rules + graph centrality
- **Why it works now:** 100-200 sessions x 5-50 files each = sufficient co-occurrence signal for ~50-200 unique files
- **Not really ML:** Association rules (Apriori) and graph algorithms (centrality, community detection) -- deterministic but powerful
- **Gain:** "When you edit `server.go`, you almost always also edit `server_test.go` and `config.go`" -- change impact warnings
- **Cons:** Single-developer patterns reflect workflow habits, not necessarily code dependencies. Use tree-sitter for ground-truth imports
- **Path:** Pure Go in hooks-mcp. `gonum/graph` for centrality. New `blast-radius` MCP tool

#### 3. Session Clustering (K-means on tool distributions)

**Rating:** High value, simple implementation

- **What:** Normalize tool counts to proportions [%Read, %Edit, %Bash, %Grep, %Glob, %Write, %Agent, %Other], cluster with k=3-5
- **Expected clusters:** Heavy-reading (exploration), edit-heavy (implementation), bash-heavy (debugging/testing), agent-heavy (delegation)
- **Data sufficiency:** 100-200 points in 8 dimensions -- K-means is stable here. DBSCAN also viable
- **Enhance with prompts:** Extract keywords from `PromptPreview` ("fix", "add", "refactor", "test") as categorical features
- **Gain:** New analytical dimension: "Am I spending more time debugging than building?" Trend analysis over time
- **Cons:** "Right" number of clusters is subjective; some sessions are mixed-type
- **Path:** Pure Go in hooks-mcp (K-means is ~30 lines). Store `session_type` label back in hook-sessions

### Tier 2: Moderate Value (useful but heuristics compete)

#### 4. Markov Chains on Tool Sequences

**Rating:** Moderate, educational

- **What:** Build transition probability matrix P(tool_j | tool_i) from PreToolUse event sequences
- **Data sufficiency:** ~20K transitions across 100-200 sessions, vocabulary of ~10 tools -> statistically robust for order 1-2
- **Gain over current:** `tool-usage` shows counts; Markov adds *sequential* patterns. "What usually follows a burst of Reads?" Anomaly scores for unusual workflows
- **Cons:** Vocabulary so small that manual inspection of transition counts is nearly as informative. Low ceiling -- captures only local patterns
- **Path:** Pure Go in hooks-mcp. A map-of-maps. New `workflow-patterns` MCP tool

#### 5. Z-score Anomaly Detection

**Rating:** Low effort, immediately useful

- **What:** Compute mean/stddev per session feature (cost, duration, tool counts, error count). Flag sessions >2sigma from historical norm
- **Data sufficiency:** 100-200 sessions with 14 numerical features -- z-scores are reliable
- **Why not Isolation Forest:** With 200 points in 14 dimensions, the marginal gain over per-feature z-scores doesn't justify the complexity. Revisit at 1000+ sessions
- **Gain:** Extends `cost-analysis` with multi-dimensional anomaly flags. "This session is unusual: normal cost but 10x the error rate"
- **Path:** Go in hooks-mcp. Compute stats from session index, add anomaly flags to `session-summary`

### Tier 3: Wait for More Data (premature at current scale)

#### 6. Error Prediction

**Rating:** Insufficient data

- **Problem:** Predict PostToolUseFailure from tool_name + tool_input patterns
- **Why wait:** 87 positive examples is critically insufficient for any supervised classifier. Even logistic regression needs hundreds
- **Current best approach:** Rule-based risk flags (commands containing `rm -rf`, `sudo`, paths outside project)
- **Data needed:** 500+ errors for useful ML. Consider rule-based pre-execution warnings now

#### 7. Neural Sequence Modeling (RNN/Transformer)

**Rating:** Premature

- **Problem:** Learn long-range tool usage dependencies for session outcome prediction
- **Why not:** Vocabulary of 10, sequences of 50-200, 100-200 training sequences -> instant overfitting. The grammar is too shallow for neural models to outperform Markov chains
- **Data needed:** 1000-5000 sessions minimum. Would need enriched vocabulary (tool + file_path_hash + success/fail) to give the model anything to learn

---

## AST Tree Analysis

Two relevant applications for structural analysis of hook event data:

### A. Bash Command AST

Parse Bash tool_input commands structurally (not just string splitting). A Bash AST reveals:
- Pipeline depth (`cmd1 | cmd2 | cmd3`)
- Redirections and their targets
- Variable interpolation (potential injection vectors)
- Subshell nesting

**Feasibility:** Go has `mvdan.cc/sh/v3` -- a full POSIX-compatible shell parser. Can parse Bash commands into AST nodes. This enables precise command classification that regex cannot match.

**Example gain:** Detect `git push --force` even when buried inside `if/then` or `$(...)` subshells.

### B. Code AST via Tree-sitter

Parse code in Edit/Write tool_input using tree-sitter. Enables:
- Symbol-level change tracking ("session modified function `HandleHook` in `server.go`")
- Blast radius via call-graph analysis (who calls the modified function?)
- Code complexity metrics on modified code

**Feasibility:** Tree-sitter has Go bindings (`smacker/go-tree-sitter`). Processing cost: 10-50ms per file. Requires language grammars for Go (primary language in this project).

**Limitation:** Requires file system access to parse full files (Edit events only contain the changed lines). The hooks-store transform would need to read the actual file -- which is local and available.

---

## MeiliSearch as a Vector Database

### Current State (v1.36.0)

MeiliSearch v1.36 has **built-in vector search** with native embedder integration. This eliminates the need for a separate vector store.

### Five Embedder Sources

MeiliSearch can generate embeddings automatically at indexing time:

| Source | How it works | Tokenizer handling | Best for |
|--------|-------------|-------------------|----------|
| **`ollama`** | MeiliSearch calls Ollama's `/api/embeddings` directly | Ollama handles tokenization internally (model-specific BPE/SentencePiece) | Local, privacy-preserving, Docker stack |
| **`huggingFace`** | MeiliSearch downloads and runs the model locally (in-process) | MeiliSearch loads the model's tokenizer from HuggingFace Hub | Zero external dependencies, best for <10K docs |
| **`openAi`** | API call to OpenAI servers | OpenAI handles tokenization (tiktoken/BPE) | Highest quality, requires API key + internet |
| **`rest`** | MeiliSearch calls any REST endpoint you specify | Your endpoint handles tokenization | Custom ONNX servers, local inference services |
| **`userProvided`** | You supply pre-computed vectors in a `_vectors` field | You handle everything yourself | Full control, ONNX-in-Rust pipeline |

### The Tokenizer Question

**A tokenizer is always needed -- the question is who runs it.**

There are two separate tokenization systems:

#### 1. MeiliSearch's Keyword Tokenizer (Charabia)

- Used for **keyword/full-text search** on fields like `data_flat`, `prompt`, `error_message`
- Language-aware: auto-detects and handles CJK, Hebrew, Thai, etc.
- Splits on whitespace, punctuation, camelCase boundaries
- **Completely independent of ML** -- this is for the traditional search index
- Already running on the data

#### 2. ML Model Tokenizer (BPE/WordPiece/SentencePiece)

- Used to convert text -> token IDs before feeding to an embedding model
- Model-specific: MiniLM uses WordPiece, nomic-embed uses SentencePiece, GPT models use BPE (tiktoken)
- **Who runs it depends on the embedder source:**
  - `ollama`: Ollama runs the tokenizer internally
  - `huggingFace`: MeiliSearch downloads and runs the tokenizer from the HuggingFace model card
  - `openAi`: OpenAI runs it server-side
  - `rest`: Your service runs it
  - `userProvided`: You run it yourself (e.g., in Rust with `tokenizers` crate)

**Key insight:** If you use MeiliSearch's `ollama` or `huggingFace` source, **you never touch a tokenizer**. MeiliSearch handles the full pipeline: document -> tokenize -> embed -> store vector -> hybrid search.

### How It Works End-to-End

```
1. Configure embedder:
   PATCH /indexes/hook-prompts/settings
   {
     "embedders": {
       "prompt-semantic": {
         "source": "ollama",
         "url": "http://ollama:11434/api/embeddings",
         "model": "nomic-embed-text",
         "documentTemplate": "A user prompt to Claude Code: {{doc.prompt}}"
       }
     }
   }

2. MeiliSearch auto-embeds:
   - Every document in hook-prompts gets a vector generated automatically
   - MeiliSearch calls Ollama with the rendered documentTemplate
   - Ollama tokenizes (SentencePiece) -> embeds -> returns vector
   - MeiliSearch stores the vector in its internal HNSW index

3. Hybrid search at query time:
   POST /indexes/hook-prompts/search
   {
     "q": "fix the race condition",
     "hybrid": {
       "embedder": "prompt-semantic",
       "semanticRatio": 0.5
     }
   }
   - semanticRatio=0.0 -> pure keyword (traditional MeiliSearch)
   - semanticRatio=1.0 -> pure vector (semantic only)
   - semanticRatio=0.5 -> hybrid (blend both rankings)
```

### What This Means for Each ML Approach

| Approach | MeiliSearch vector contribution |
|----------|-------------------------------|
| **Prompt embeddings** | Eliminates the need for a separate vector store. Configure Ollama embedder on `hook-prompts`, get hybrid search for free |
| **Session similarity** | Embed `prompt_preview` on `hook-sessions` index. Similar sessions = nearest neighbors in vector space |
| **Error clustering** | Embed `error_message` on `hook-events` filtered to PostToolUseFailure. Semantic grouping without manual regex |
| **Blast radius** | No benefit -- file co-modification is graph analysis, not text similarity |
| **Session clustering** | Marginal -- could embed session descriptions, but K-means on numerical features is more appropriate |
| **Anomaly detection** | No benefit -- numerical features, not text |

### The `documentTemplate` Field (Liquid Syntax)

Controls what text gets embedded. MeiliSearch uses Liquid templating:

```
# For hook-prompts: embed the user's prompt
"A user prompt asking Claude to: {{doc.prompt}}"

# For hook-events: embed tool context
"A {{doc.hook_type}} event for tool {{doc.tool_name}} on file {{doc.file_path}}"

# For hook-sessions: embed session summary
"A coding session in {{doc.project_name}} that {{doc.prompt_preview}}"
```

The template injects semantic context around the raw field values, improving embedding quality. Think of it as a "prompt for the embedder."

### Architecture with Ollama in Docker

Add Ollama to the existing docker-compose.yml:

```yaml
services:
  ollama:
    image: ollama/ollama
    volumes:
      - ollama-models:/root/.ollama
    # Pull model on first run: docker exec ollama ollama pull nomic-embed-text

  meilisearch:
    image: getmeili/meilisearch:v1.36.0
    environment:
      MEILI_OLLAMA_URL: "http://ollama:11434"
    # ... existing config ...
```

MeiliSearch calls Ollama over Docker's internal network. No exposed ports needed. The entire pipeline is local and private.

### `userProvided` Path (for ONNX-in-Rust)

If you build a Rust embedding binary with ONNX runtime:

```
1. Configure: source: "userProvided", dimensions: 384
2. Your Rust binary tokenizes + embeds each prompt
3. Add vectors to documents: {"_vectors": {"my-embedder": [0.1, -0.3, ...]}}
4. MeiliSearch stores vectors, enables hybrid search
5. At search time: your binary embeds the query, pass vector to MeiliSearch
```

This gives maximum control but requires managing tokenization yourself (Rust `tokenizers` crate from HuggingFace).

### Phased Approach

| Phase | Embedder | Tokenizer | Effort |
|-------|----------|-----------|--------|
| **1. Prototype** | `ollama` source in MeiliSearch | Ollama handles it | Add Ollama to docker-compose, configure embedder via API. ~1 day |
| **2. Evaluate** | Same, measure quality | Same | Test hybrid search on real prompts, tune semanticRatio |
| **3. Optimize** | `huggingFace` (in-process) or `userProvided` (ONNX Rust) | MeiliSearch or your code | Only if Ollama latency/quality is insufficient |

---

## ONNX in Rust vs Ollama: Decision Framework

| Dimension | ONNX in Rust | Ollama |
|-----------|-------------|--------|
| Setup | High (Rust toolchain, model files) | Low (`ollama pull model`) |
| Latency | 1-10ms per embedding | 50-500ms per embedding |
| Memory | 90-300MB per model | 2-8GB per model |
| Capabilities | Embedding, classification only | Embedding, classification, generation, summarization |
| Maintenance | Medium (Rust binary versioning) | Low (auto-updates) |
| Real-time viable? | Yes | No |
| Best for | Production embedding pipeline | Prototyping, text analysis, batch enrichment |

**Recommendation:** Start with Ollama (fast iteration, zero model management). Move to ONNX in Rust only if: (a) latency is unacceptable, or (b) you want zero-dependency single-binary deployment.

**Where in the pipeline:**
- **hook-client (real-time):** Neither -- hook-client must exit 0 immediately. No inference here
- **hooks-store (ingest-time):** ONNX viable (~5ms overhead). Ollama viable for batch background processing
- **hooks-mcp (query-time):** Both viable. ONNX for interactive queries, Ollama for heavier analysis

---

## Classical NLP Enrichment: RAKE + HDP/LDA

### Four-Layer Text Analysis Stack

Each layer adds capability but also complexity. You can stop at any layer:

```
Layer 0: Raw prompt text (233 prompts in hook-prompts index)
  |
Layer 1: RAKE keyword extraction (per-prompt, zero dependencies)
  -> ["ml algorithms", "data stored", "pros and cons"]
  -> Store as filterable `keywords` field in MeiliSearch
  |
Layer 2: HDP/LDA topic modeling (corpus-wide, lightweight)
  -> Input: RAKE keywords (cleaner than raw text)
  -> Output: [0.6 exploration, 0.3 analysis, 0.1 debugging] per prompt
  -> Store as `topic_vector` on hook-prompts and hook-sessions
  |
Layer 3: Neural embeddings via MeiliSearch Ollama or ONNX
  -> 384-dim dense vector for deep semantic similarity
  |
Layer 4: ONNX multi-signal fusion (numerical + text features -> prediction)
```

### RAKE Keywords -- Immediate Enrichment (no ML)

- Extracts multi-word keyphrases per prompt (pure algorithm, no training)
- New filterable/searchable field in MeiliSearch: `keywords: ["authentication handler", "middleware"]`
- Fills the gap between `data_flat` (noisy) and neural embeddings (heavy infrastructure)
- Preprocesses input for LDA (cleaner vocabulary -> better topics)

### HDP/LDA Topics -- Lightweight Session Classification

- Topic distributions ARE embeddings: `[0.6, 0.2, 0.1, 0.05, 0.03, 0.02]` = 6-dim per prompt
- Auto-discovers session types from text (exploration, debugging, feature work, refactoring)
- 233 prompts is borderline but workable with RAKE preprocessing to reduce vocabulary noise
- Compact alternative to MiniLM in the ONNX fingerprint:
  - With LDA: `tool_dist(8) + temporal(5) + files(4) + cost(3) + LDA(8) + RAKE(5) = 33 dims`
  - With MiniLM: `tool_dist(8) + temporal(5) + files(4) + cost(3) + MiniLM(384) = 404 dims`
  - 33 dims trains better than 404 dims when you only have 200 sessions (curse of dimensionality)

### When to Use Which

| | RAKE | HDP/LDA | Neural Embeddings |
|---|---|---|---|
| Captures semantics? | No -- surface keywords | Partially -- same-topic clustering | Yes -- deep similarity |
| Interpretable? | Fully | Mostly (topic word lists) | No (384-dim black box) |
| Dependencies | Zero | Go library or Python batch | ONNX or Ollama |
| Data sufficiency | 1 prompt | 233 borderline, 500+ solid | 1 prompt (pre-trained) |
| Best for | Search enrichment | Session classification | Semantic similarity |

---

## Where ONNX Specifically Shines

### The Core ONNX Advantage

ONNX's unique properties -- sub-10ms latency, single-binary deployment, no runtime services, deterministic output -- create capabilities that Ollama/MeiliSearch/pure-Go cannot replicate. The sweet spot is **combining multiple signal types (text + numerical + structural) in one fast inference pass**.

### Best ONNX Scenario: Multi-Signal Session Fingerprinting

The one place ONNX enables a capability nothing else in the stack can replicate:

**Preprocessing pipeline (Rust or Go):**
1. Tool distribution vector (8 dims): `[%Read, %Edit, %Bash, %Grep, %Glob, %Write, %Agent, %Other]`
2. Temporal features (5 dims): duration, mean inter-event gap, stddev, burst count, longest gap
3. File features (4 dims): unique files, max depth, cross-directory ratio, test-file ratio
4. Cost trajectory (3 dims): tokens/prompt, cost/event, cache-hit ratio
5. Prompt embedding (384 dims): first prompt via MiniLM ONNX model

Total: 404-dim feature vector per session

**Stacked ONNX models:**
- Model 1: MiniLM (pre-trained, 22M params, 90MB) -> text to 384-dim vector in ~5ms
- Model 2: Classifier (sklearn/PyTorch trained, exported to ONNX) -> 404-dim to session type + confidence in <1ms
- Total: ~6ms per session. Single binary. No services.

**Why only ONNX can do this:**
- MeiliSearch embeds text fields but can't combine text embeddings with numerical features
- Ollama consumes text but can't accept a 404-dim feature tensor
- Pure Go does K-means on numbers but can't incorporate prompt semantics
- ONNX stacks text embedding + mixed-feature classifier in one pass

**LDA-based alternative (lighter):**
Replace MiniLM's 384 dims with LDA's 8-dim topic vector + RAKE features:
- `tool_dist(8) + temporal(5) + files(4) + cost(3) + LDA(8) + RAKE(5) = 33 dims`
- Trains better with 200 sessions (curse of dimensionality)
- No external model dependency for the text features
- ONNX still valuable for the trained classifier on top

### Other ONNX Scenarios

#### Inline Prompt Embedding at Ingest

**Rating:** Ready now

MeiliSearch's Ollama embedder works *asynchronously* -- document is indexed first, embeddings generated in background. ONNX makes embeddings **synchronous** at document creation time (~5ms).

**What this unlocks that async can't:**
- **Real-time duplicate detection:** Cosine similarity against last N prompts at ingest time -> "you asked the same thing 3 prompts ago"
- **Streaming prompt drift detection:** Is this prompt's embedding far from the session's centroid? -> "this session is shifting topic"
- **Immediate `_vectors` population:** No background delay -- hybrid search works on the document the instant it's indexed

**Architecture:** `hooks-store (Go) -> hooks-embed (Rust binary via stdin/stdout) -> MiniLM tokenizer + ONNX -> 384-dim vector -> Document._vectors`

Or: `onnxruntime_go` directly in hooks-store (less mature but single-language stack).

#### Learned Session Embeddings via Autoencoder

**Rating:** Needs 1000+ sessions

**The idea:** Instead of hand-crafting which session features matter, learn a compressed representation.

**Input:** 20-dim numerical feature vector (tool counts, cost, duration, etc.)
**ONNX model:** Small autoencoder (20 -> 8 -> 20). The 8-dim bottleneck IS the session embedding.
- Train unsupervised on all historical sessions
- Reconstruction error = anomaly score (no labels needed)
- Bottleneck representation = session similarity in 8-dim space

**Honest assessment at current scale:** With 200 sessions and 20 features, this autoencoder is essentially learning PCA. `gonum` PCA gives nearly identical results without ONNX. Gets interesting at 1000+ sessions where nonlinear patterns emerge that PCA can't capture.

**Why wait matters:** The autoencoder's value grows with data diversity. At 200 sessions from one developer, the "normal" space is narrow. At 1000+ sessions (or multi-developer data), the model discovers meaningful nonlinear structure -- session archetypes that aren't separable by linear combinations of features.

#### Code Change Embeddings via CodeBERT

**Rating:** Speculative but unique

**The bold idea:** Embed the *diff* content from Edit/Write tool events using a code-aware model.

**Why ONNX is the only option here:**
- CodeBERT (125M params, ~500MB) and UniXcoder are research models trained specifically on code
- They understand function structure, variable naming conventions, and diff semantics
- These models exist **only** as HuggingFace checkpoints -- not in Ollama's model library
- The only way to run them locally at reasonable speed is HF -> ONNX export

**What code embeddings unlock:**
- **Semantic change similarity:** "These two edits in different files made the same kind of change" (even if different functions/variables)
- **Change type clustering:** Automatic grouping into refactor, bugfix, feature addition, style fix -- without keyword rules
- **Enriched blast radius:** "This change is semantically similar to a previous change that triggered 5 follow-up edits in 3 other files"

**The data challenge:** Edit events contain only the changed lines (`old_string` -> `new_string`), not the full function context. Embeddings of short diffs may be noisy. Mitigation: concatenate nearby context from the file (hooks-store runs locally, file system is accessible).

**Preprocessing pipeline:**
1. Extract Edit tool_input from PostToolUse events (old_string + new_string)
2. Optionally: read surrounding context from file system at ingest time
3. Feed through CodeBERT ONNX model -> 768-dim code embedding
4. Store in `_vectors` on hook-events, filterable to Edit events only

---

## Exploration Sequence

| Phase | What | Effort | Tech |
|-------|------|--------|------|
| 1 | Add Ollama to docker-compose + configure MeiliSearch `ollama` embedder on `hook-prompts` | 1 day | Docker, MeiliSearch API |
| 2 | Evaluate hybrid search quality on real prompts, tune semanticRatio | 2-3 days | MeiliSearch queries |
| 3 | Add embedder to `hook-sessions` (prompt_preview) for session similarity | 1 day | MeiliSearch API |
| 4 | Blast radius co-modification matrix | 3-5 days | Pure Go, `gonum/graph` |
| 5 | Session clustering (K-means on tool distributions) | 2-3 days | Pure Go |
| 6 | Markov chain workflow patterns | 2-3 days | Pure Go |
| 7 | Z-score anomaly detection | 1-2 days | Pure Go |
| 8 | ONNX Rust: MiniLM sync embedding in hooks-store ingest pipeline | 1-2 weeks | Rust, `ort` + `tokenizers` crates |
| 9 | ONNX Rust: Multi-signal session fingerprinting (MiniLM + classifier stack) | 1-2 weeks | Rust, `ort` + sklearn/PyTorch export |
| 10 | ONNX Rust: CodeBERT for Edit diff embeddings (speculative) | 1-2 weeks | Rust, `ort`, HF model export |

---

## Validation Steps

This is a research document, not an implementation plan. To validate findings:
- Query MeiliSearch to confirm actual event counts and field availability
- Run `ollama pull nomic-embed-text` and test embedding quality on real prompts
- Compute file co-occurrence from hook-sessions to verify blast radius signal strength
- Build a toy Markov chain from one session's tool sequence to verify transition patterns
- Test RAKE keyword extraction on a sample of real prompts to assess quality

---

## Key Files for Future Implementation

- `hooks-store/internal/store/transform.go` -- Where enrichment fields (keywords, embeddings) would be added
- `hooks-store/internal/store/store.go` -- Document/SessionDocument types needing new fields
- `hooks-store/internal/store/meili.go` -- Session aggregation + MeiliSearch index config (vector search, embedder setup)
- `hooks-mcp/internal/tools/register.go` -- New MCP tool registration
- `hooks-mcp/internal/meili/client.go` -- Searcher interface extension for vector/hybrid queries
- `docker-compose.yml` -- Add Ollama service for embedding generation
