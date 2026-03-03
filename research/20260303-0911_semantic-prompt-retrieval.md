# Semantic Prompt Retrieval — Multi-Layer Search for Hook Data

**Date:** 2026-03-03
**Context:** How to answer queries like "find the algorithm I was working on two days ago" when the user can't remember the exact terms — combining MeiliSearch, embeddings, keywords, and topic models.

---

## 1. The Problem

```
"find the relevant algorithm I was working two days ago. I forgot its name"
```

No single technique can answer this alone:

| Challenge | Why |
|---|---|
| No keywords to match | The user doesn't know the algorithm name — can't search for it |
| Temporal reference | "two days ago" = needs date resolution |
| Intent is meta | Not asking about code — asking about their own history |
| "relevant" is subjective | Relevant to what? Current session context? |

---

## 2. Solution: Layer by Layer

### Layer 1 — Date Resolution (deterministic)

Resolve "two days ago" to a timestamp range. The hooks-mcp `dateparse` package already does this:

```
"two days ago" → 2026-03-01T00:00:00Z to 2026-03-01T23:59:59Z
```

```bash
# MeiliSearch filter
filter: "hook_type = UserPromptSubmit AND timestamp_unix >= 1740787200 AND timestamp_unix <= 1740873599"
```

This narrows 10,000 events → maybe 50 prompts from that day.

### Layer 2 — Keyword Filtering (if RAKE keywords exist)

If prompts were tagged at ingest, filter further:

```bash
filter: "keywords = algorithm OR keywords = model OR keywords = ML"
```

50 prompts → maybe 8. But if the user said "algorithm" and the actual prompt used "classifier" or "BERTopic", **keywords miss it**.

### Layer 3 — Embedding Similarity (semantic)

This is where embeddings shine. Take the **current prompt** and compare against stored prompt embeddings:

```python
# Current session context (what the user is working on now)
current_context = "ML algorithms, topic modeling, RAKE, keyword extraction"

# Get embedding for current context
current_vector = embed(current_context)  # → [0.12, -0.45, ...]

# Search MeiliSearch with vector similarity (only within date range)
results = index.search("", {
    "vector": current_vector,
    "filter": "timestamp_unix >= 1740787200 AND timestamp_unix <= 1740873599",
    "hybrid": {"semanticRatio": 0.8},
    "limit": 5
})
```

This finds prompts from two days ago that are **semantically close** to what the user is currently doing — even if zero words overlap.

### Layer 4 — Topic Match (if HDP/BERTopic labels exist)

If prompts have topic labels, match the current session's topic:

```
Current session topic:  Topic 5 (ML/data-science)
Two days ago prompts in Topic 5:
  → "explore LDA for document clustering"         ← MATCH
  → "how does BERTopic compare to Top2Vec"         ← MATCH
  → "refactor the ingest server"                   ← different topic, skip
```

---

## 3. Full Pipeline

```
User: "find the relevant algorithm I was working two days ago"
                    |
                    v
        +-- Parse temporal reference -----------------+
        |  "two days ago" → March 1 date range        |
        +---------------------------------------------+
                    |
                    v
        +-- Retrieve candidate prompts ---------------+
        |  MeiliSearch filter by date + hook_type      |
        |  Result: ~50 prompts                         |
        +---------------------------------------------+
                    |
                    v
        +-- Rank by similarity -----------------------+
        |  Option A: Embedding cosine similarity       |
        |            against current context            |
        |  Option B: Topic label match                 |
        |  Option C: Keyword overlap                   |
        |  Best: combine all three (hybrid score)      |
        +---------------------------------------------+
                    |
                    v
        +-- Return top results -----------------------+
        |  "On March 1 you were exploring:             |
        |   - LDA for topic modeling                   |
        |   - BERTopic vs Top2Vec comparison           |
        |   - RAKE keyphrase extraction"               |
        +---------------------------------------------+
```

---

## 4. What Each Layer Contributes

| Layer | What it does | Without it |
|---|---|---|
| **Date parsing** | Narrows to the right day | Searching all history (slow, noisy) |
| **Keywords** | Fast filter if user remembers a term | Still works without, just more candidates |
| **Embeddings** | Finds semantic matches even with zero word overlap | Miss synonyms, related concepts |
| **Topic labels** | Coarse but reliable grouping | Embeddings cover this but less interpretably |

---

## 5. What Would Need to Be Added to hooks-store

| Component | Effort | Impact |
|---|---|---|
| Date parsing | Already done (hooks-mcp `dateparse`) | Baseline — temporal queries work today |
| RAKE keywords at ingest | ~100 LOC Go | Faceting, fast filtering |
| Ollama embeddings at ingest | ~50 LOC Go (HTTP call) | Semantic search — the big win |
| MeiliSearch vector search config | One settings PATCH | Enables hybrid search |
| New MCP tool `find-related-prompts` | ~150 LOC Go | Exposes it all to Claude |

---

## 6. Background: Embedding Models

An embedding model converts input (text, images, code) into a **fixed-size vector of numbers** — a point in high-dimensional space where **similar things are near each other**.

```
"refactor the auth handler"  →  [0.12, -0.45, 0.78, 0.03, ..., -0.22]   (768 dimensions)
"rename the login function"  →  [0.11, -0.42, 0.75, 0.05, ..., -0.20]   (nearby — similar meaning)
"fix the CSS padding"        →  [0.89, 0.33, -0.12, 0.67, ..., 0.44]    (far away — different meaning)
```

### What Makes Them Different from Keyword Search

| | Keyword matching | Embedding similarity |
|---|---|---|
| "auth handler" vs "login function" | No match (different words) | High similarity (same meaning) |
| "bank" (finance) vs "bank" (river) | Same match | Different vectors (context-aware) |
| How it works | Exact/fuzzy string matching | Geometric distance in vector space |

### Popular Embedding Models

| Model | Dimensions | Size | Best For |
|---|---|---|---|
| `all-MiniLM-L6-v2` | 384 | 80 MB | Fast, general-purpose (good starting point) |
| `all-mpnet-base-v2` | 768 | 420 MB | Higher quality, still fast |
| `nomic-embed-text` | 768 | 270 MB | Open-source, runs locally via Ollama |
| `bge-small-en-v1.5` | 384 | 130 MB | Optimized for retrieval |
| OpenAI `text-embedding-3-small` | 1536 | API only | Cloud, pay-per-token |
| Voyage `voyage-code-3` | 1024 | API only | Code-specific embeddings |

---

## 7. Background: Inference Runtimes

An inference runtime is the **engine that runs the model** — it loads the weights, accepts input, and produces output vectors. The model file alone is inert; it needs a runtime to execute.

```
Input text → [Runtime loads model weights] → forward pass through neural network → output vector
```

### Runtime Options

| Runtime | Language | How It Works | Speed | Setup |
|---|---|---|---|---|
| **Ollama** | Any (HTTP API) | Downloads & serves models locally, GPU auto-detect | Fast | `ollama pull nomic-embed-text` |
| **ONNX Runtime** | C/C++/Go/Python | Optimized cross-platform inference, CPU or GPU | Very fast | Load `.onnx` model file |
| **llama.cpp** | C++ (bindings for Go/Python) | Runs GGUF quantized models on CPU | Fast | Compile or use prebuilt |
| **Hugging Face Transformers** | Python | Reference implementation, full flexibility | Moderate | `pip install transformers` |
| **Sentence-Transformers** | Python | High-level wrapper over HF, built for embeddings | Moderate | `pip install sentence-transformers` |
| **candle** | Rust | Lightweight, no Python dependency | Very fast | Rust crate |
| **FastEmbed** | Python | Optimized ONNX-based, minimal dependencies | Fast | `pip install fastembed` |

### For Go (hooks-store stack)

| Option | Integration |
|---|---|
| **Ollama HTTP API** | `POST http://localhost:11434/api/embeddings` — simplest, no Go deps |
| **ONNX Runtime Go bindings** | `github.com/yalue/onnxruntime_go` — embedded, no external process |
| **Call out to Python** | `exec.Command("python", "embed.py")` — pragmatic but clunky |

---

## 8. How to Feed Data to an Embedding Model

### Step 1: Prepare text input

```go
// Your prompt from MeiliSearch
prompt := "refactor the authentication handler to use middleware"
```

### Step 2: Call the runtime

**Ollama (HTTP — easiest for Go):**
```bash
curl http://localhost:11434/api/embeddings \
  -d '{"model": "nomic-embed-text", "prompt": "refactor the auth handler"}'

# Response:
# {"embedding": [0.12, -0.45, 0.78, ...]}   ← 768 floats
```

**Python (sentence-transformers):**
```python
from sentence_transformers import SentenceTransformer

model = SentenceTransformer('all-MiniLM-L6-v2')
vectors = model.encode([
    "refactor the auth handler",
    "rename the login function",
    "fix the CSS padding"
])
# vectors.shape → (3, 384)  — 3 texts, 384 dimensions each
```

### Step 3: Store the vectors

Store alongside the document in MeiliSearch (supports vector search since v1.3):

```json
{
  "id": "evt_abc123",
  "prompt": "refactor the authentication handler",
  "keywords": ["refactor", "authentication handler"],
  "_vectors": {
    "prompt_embedding": [0.12, -0.45, 0.78, ...]
  }
}
```

---

## 9. Embedding Use Cases for Hook Data

| Use Case | How Embeddings Help | Keywords Can't |
|---|---|---|
| **Semantic search** | "find prompts about renaming things" finds "refactor function name" | Misses synonyms |
| **Prompt clustering** | Group similar prompts without predefined categories | Requires manual taxonomy |
| **Duplicate detection** | "fix the bug" ≈ "resolve the issue" → cosine similarity 0.92 | Different words = no match |
| **Anomaly detection** | Prompt vector far from all clusters → unusual request | No notion of "unusual" |
| **Recommendation** | "You also asked about X in similar sessions" | No similarity measure |
| **RAG (retrieval)** | Find relevant past prompts to inform current context | Only exact matches |

### Concrete Example

```
Session A prompt: "add error handling to the API endpoint"
Session B prompt: "implement proper exception management for the REST handler"

Keyword overlap:  0 shared words
Embedding cosine: 0.89 (very similar)
→ These sessions were doing the same kind of work
```

---

## 10. How the Three Approaches Relate

```
RAKE (per-prompt, real-time)       → keywords for faceting/filtering
HDP  (corpus-wide, batch)         → topic discovery, noise-free categorization
Embeddings (per-prompt, real-time) → semantic similarity, clustering, search

Each operates at a different level:
  RAKE        = "what words matter in this prompt"
  HDP         = "what themes exist across all prompts"
  Embeddings  = "where does this prompt sit in meaning-space"
```

---

## 11. Key Insight

The embedding layer is the **critical piece** that makes "I forgot the name" queries work. Without it, you're limited to what the user can remember and type as search terms — which is exactly what they can't do in this scenario.

For the hooks-store pipeline, the most practical addition would be **Ollama + nomic-embed-text** — it runs locally, the HTTP API integrates cleanly with Go, and MeiliSearch can store and search the vectors natively.
