# ML Decision Tables — Visual Reference

*Dense summary tables for narrowing down approaches*

---

## Table 1: All Techniques at a Glance

| Technique | Type | Input | Output | Dependencies | Data Ready? |
|-----------|------|-------|--------|-------------|-------------|
| RAKE | Keyword extraction | Single prompt text | Ranked keyphrases | None (pure algorithm) | Yes (1+ prompts) |
| HDP | Topic discovery | All prompts corpus | Auto-discovered topics + distributions | Go/Python lib | Borderline (233) |
| LDA | Topic discovery | All prompts corpus | Fixed-k topics + distributions | Go/Python lib | Borderline (233) |
| MiniLM (ONNX) | Neural embedding | Text | 384-dim dense vector | ONNX runtime | Yes (pre-trained) |
| nomic-embed (Ollama) | Neural embedding | Text | 768-dim dense vector | Ollama service | Yes (pre-trained) |
| CodeBERT (ONNX) | Code embedding | Code diff text | 768-dim code vector | ONNX runtime | Yes (pre-trained) |
| K-means | Clustering | Numerical vectors | Cluster labels | None (~30 lines Go) | Yes (200 sessions) |
| Markov chain | Sequence model | Tool name sequences | Transition probabilities | None (map of maps) | Yes (20K transitions) |
| Association rules | Co-occurrence | File lists per session | Co-modification pairs | None | Yes (200 sessions) |
| Z-scores | Anomaly detection | Session feature vectors | Anomaly flags | None | Yes (200 sessions) |
| Isolation Forest | Anomaly detection | Session feature vectors | Anomaly scores | sklearn → ONNX | Needs 1000+ sessions |
| Autoencoder | Learned embedding | Session feature vectors | Compressed representation | PyTorch → ONNX | Needs 1000+ sessions |
| Random Forest | Classification | Mixed feature vectors | Session type + confidence | sklearn → ONNX | Needs labels from K-means |

---

## Table 2: What Each Technique Operates On

| Technique | hook-events | hook-prompts | hook-sessions | Cross-index |
|-----------|:-----------:|:------------:|:-------------:|:-----------:|
| RAKE | | prompt text | | |
| HDP/LDA | | all prompts | | |
| MiniLM/nomic | | prompt text | prompt_preview | |
| CodeBERT | Edit diffs | | | |
| K-means | | | tool counts, cost, duration | |
| Markov chain | PreToolUse sequences | | | |
| Association rules | | | files_read, files_written | |
| Z-scores | | | all numerical features | |
| Multi-signal ONNX | | prompts | numerical features | Yes |

---

## Table 3: MeiliSearch Vector Database — What It Handles vs What You Build

| Capability | MeiliSearch built-in | You build |
|-----------|:-------------------:|:---------:|
| Text embedding at index time | Ollama/HF embedder | |
| Vector storage (HNSW) | Built-in | |
| Hybrid search (keyword + semantic) | semanticRatio param | |
| Tokenization for embedding models | Delegated to embedder | Only if userProvided |
| Prompt similarity search | Hybrid search on hook-prompts | |
| Session similarity search | Hybrid search on hook-sessions | |
| Error message clustering | Hybrid search filtered to failures | |
| Multi-signal fusion (text + numbers) | | ONNX pipeline |
| Code diff embeddings | | CodeBERT ONNX |
| Keyword extraction | | RAKE |
| Topic modeling | | LDA/HDP |
| File co-modification graph | | Association rules |
| Anomaly detection | | Z-scores |

---

## Table 4: Embedder Source Comparison

| | Ollama | HuggingFace | OpenAI | REST | userProvided |
|---|---|---|---|---|---|
| Where model runs | Ollama service | Inside MeiliSearch | Cloud API | Your endpoint | Your code |
| Tokenizer handled by | Ollama | MeiliSearch | OpenAI | Your endpoint | Your code |
| Latency per doc | 50-500ms | 10-50ms | 100-300ms | Varies | You control |
| Setup effort | Low | Low | Medium (API key) | Medium | High |
| Privacy | Local | Local | Cloud | Your choice | Local |
| Async/Sync | Async (MeiliSearch) | Async (MeiliSearch) | Async (MeiliSearch) | Async (MeiliSearch) | Sync (you ingest) |
| Cost | Free (CPU/GPU) | Free (CPU) | $0.02/1M tokens | Your infra | Your infra |
| Best for | Prototyping | Small datasets (<10K) | Best quality | Custom models | ONNX pipeline |

---

## Table 5: RAKE vs LDA vs Neural Embeddings

| Dimension | RAKE | LDA/HDP | Neural (MiniLM/nomic) |
|-----------|------|---------|----------------------|
| What it finds | Important phrases in one doc | Latent themes across corpus | Semantic meaning |
| Output per prompt | `["auth handler", "middleware"]` | `[0.6, 0.2, 0.1, 0.05, 0.05]` | `[0.12, -0.34, ...]` (384 dims) |
| Captures "fix auth" ≈ "repair login"? | No | Partially (same topic) | Yes |
| Interpretable? | Fully | Mostly | No |
| Dependencies | Zero | Library | ONNX or Ollama |
| Min data | 1 prompt | ~100 prompts (233 borderline) | 1 prompt (pre-trained) |
| Retraining needed? | No | Yes (as corpus grows) | No |
| Dims as embedding | N/A | 5-15 | 384-768 |
| MeiliSearch integration | Filterable field | Filterable field | _vectors + hybrid search |
| Use as ONNX input feature? | Keyword category counts (5 dims) | Topic vector (8 dims) | Raw embedding (384 dims) |

---

## Table 6: ONNX-Specific Advantages

| Scenario | What ONNX does that others can't | Latency | Model |
|----------|----------------------------------|---------|-------|
| Multi-signal fusion | Combine prompt embedding + tool stats + file features + cost → one prediction | ~6ms | MiniLM + classifier |
| Sync embedding at ingest | Embed prompt before document is stored (not async background) | ~5ms | MiniLM |
| Code diff embedding | CodeBERT not available in Ollama — only HF→ONNX | ~50ms | CodeBERT |
| Real-time duplicate detect | Cosine similarity at ingest time — async is too late | ~5ms | MiniLM |
| Prompt drift detection | Compare new prompt to session centroid in real-time | ~5ms | MiniLM |

---

## Table 7: Feature Vectors for Session Fingerprinting

| Feature group | Dims | Source | Available now? |
|--------------|------|--------|---------------|
| Tool distribution | 8 | hook-sessions tool counts | Yes |
| Temporal | 5 | Computed from event timestamps | Yes (needs extraction) |
| File access | 4 | hook-sessions files_read/written | Yes |
| Cost trajectory | 3 | hook-sessions cost/tokens | Yes |
| RAKE keyword categories | 5 | Phase 2 output | After Phase 2 |
| LDA topic vector | 8 | Phase 3 output | After Phase 3 |
| MiniLM prompt embedding | 384 | ONNX or Ollama | After Phase 1 or 8A |
| **Total (lightweight)** | **33** | Phases 2-3 | **Better for 200 sessions** |
| **Total (full)** | **404** | All phases | **Better for 1000+ sessions** |

---

## Table 8: What Each Approach Answers

| Question | Best technique |
|----------|---------------|
| "Find prompts similar to X" | Neural embeddings (hybrid search) |
| "What keywords define this session?" | RAKE |
| "What type of work was this session?" | LDA topics or K-means clustering |
| "What usually follows a Read?" | Markov chains |
| "Is this session unusual?" | Z-scores |
| "If I edit X, what else needs changing?" | File co-modification (association rules) |
| "What's the semantic change pattern?" | CodeBERT embeddings (ONNX) |
| "Predict session type from mixed signals" | Multi-signal ONNX classifier |
| "Group similar errors without regex" | Neural embeddings on error_message |
| "Find sessions like this one" | Embed prompt_preview (hybrid search) |

---

## Table 9: Layered Pipeline — Stop Anywhere

| Layer | What you add | Cumulative capability | External deps |
|-------|-------------|----------------------|---------------|
| 0 | Raw data | Keyword search on data_flat | None (current state) |
| 1 | RAKE keywords | Filterable keyphrases per prompt | None |
| 2 | LDA topics | Session type labels, 8-dim topic embeddings | Go/Python lib |
| 3 | MeiliSearch Ollama | Hybrid search (keyword + semantic) | Docker (Ollama) |
| 4 | K-means + Z-scores | Session clustering + anomaly flags | None |
| 5 | Markov + co-modification | Workflow patterns + blast radius | None |
| 6 | ONNX MiniLM | Sync embedding, real-time duplicate detect | Rust binary |
| 7 | ONNX classifier | Multi-signal session prediction | Rust + Python training |
| 8 | ONNX CodeBERT | Semantic code change analysis | Rust + large model |

---

## Table 10: Risk / Reward Matrix

| Technique | Reward if it works | Risk of failure | Effort | Net value |
|-----------|-------------------|-----------------|--------|-----------|
| MeiliSearch hybrid search | High — semantic search on prompts | Low — config only, no code | 1 day | Very high |
| RAKE keywords | Medium — better filtering | Very low — deterministic | 2-3 days | High |
| File co-modification | High — blast radius warnings | Low — straightforward math | 3-5 days | High |
| K-means clustering | Medium — session type labels | Low — well-understood algo | 2-3 days | Medium-high |
| LDA topics | Medium — interpretable themes | Medium — small corpus, unstable topics | 3-5 days | Medium |
| Z-score anomaly | Medium — unusual session flags | Low — simple statistics | 1-2 days | Medium |
| Markov chains | Low-medium — workflow patterns | Low — simple math | 2-3 days | Medium |
| ONNX MiniLM sync embed | Medium — real-time duplicate detect | Medium — Rust setup overhead | 1-2 weeks | Medium |
| ONNX multi-signal classifier | High — fused prediction | High — needs training data + labels | 2-3 weeks | Uncertain |
| ONNX CodeBERT | High if it works — unique capability | High — noisy short diffs, large model | 2-3 weeks | Speculative |

---

## Table 11: Tokenizer Responsibility Map

| Embedder path | Who tokenizes? | Tokenizer type | You touch it? |
|--------------|---------------|----------------|:------------:|
| MeiliSearch + Ollama | Ollama | SentencePiece (nomic) | No |
| MeiliSearch + HuggingFace | MeiliSearch | Model's tokenizer from HF Hub | No |
| MeiliSearch + OpenAI | OpenAI | tiktoken (BPE) | No |
| MeiliSearch + REST | Your endpoint | Whatever your model needs | Yes |
| MeiliSearch + userProvided | Your code | Whatever your model needs | Yes |
| ONNX in Rust (MiniLM) | Rust `tokenizers` crate | WordPiece | Yes |
| ONNX in Rust (CodeBERT) | Rust `tokenizers` crate | BPE | Yes |
| MeiliSearch keyword search | Charabia (built-in) | Language-aware word split | No |
