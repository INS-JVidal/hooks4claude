# Phase 8: ONNX Rust Integration

*Multi-signal fusion, sync embedding, and code-aware analysis*

## Goal

Build a Rust binary (`hooks-ml`) using ONNX runtime that combines text embeddings with numerical features for capabilities that MeiliSearch/Ollama/pure-Go cannot replicate.

## Why ONNX in Rust

- Sub-10ms inference per session (vs 500ms+ for Ollama)
- Single binary deployment, no runtime services
- Can fuse text + numerical + structural features in one pass
- CodeBERT/UniXcoder models exist only as HF checkpoints — ONNX is the only local inference path
- Deterministic output

## Prerequisites

Phases 1-7 inform what features and labels are available:
- Phase 1: Ollama proves embedding value
- Phase 2: RAKE keywords available as features
- Phase 3: LDA topic vectors available as features
- Phase 5: Session type labels available as training targets
- Phase 7: Anomaly thresholds as baseline comparison

## Three Sub-phases

### 8A: Sync Prompt Embedding (MiniLM in ONNX)

**What:** Replace or complement MeiliSearch's async Ollama embedder with synchronous ONNX embedding at ingest time.

**Rust crates:**
- `ort` — ONNX runtime bindings
- `tokenizers` — HuggingFace tokenizer library (handles WordPiece for MiniLM)

**Model:** `all-MiniLM-L6-v2` (~90MB ONNX, 384-dim output, ~5ms inference)

**Architecture:**
```
hooks-store (Go)
  → on UserPromptSubmit event
  → pipes prompt text to hooks-ml via stdin
  → hooks-ml tokenizes (WordPiece) → embeds (MiniLM ONNX) → writes 384 floats to stdout
  → hooks-store reads vector, stores in Document._vectors
```

**What this enables over MeiliSearch async:**
- Immediate vector availability (no background processing delay)
- Real-time duplicate detection: cosine similarity against last N prompts
- Streaming drift detection: is this prompt far from the session centroid?

**Effort:** 1-2 weeks (Rust setup, model download pipeline, stdin/stdout protocol)

### 8B: Multi-Signal Session Fingerprinting

**What:** Combine text embedding with numerical features → trained classifier → session prediction.

**Feature vector:**

With MiniLM:
```
tool_dist(8) + temporal(5) + files(4) + cost(3) + MiniLM(384) = 404 dims
```

With LDA (lighter, trains better at 200 sessions):
```
tool_dist(8) + temporal(5) + files(4) + cost(3) + LDA(8) + RAKE(5) = 33 dims
```

**Training workflow (Python):**
1. Export session features + labels from MeiliSearch
2. Train classifier: Random Forest (sklearn) or small MLP (PyTorch)
3. Export to ONNX: `skl2onnx` or `torch.onnx.export`
4. Deploy ONNX model in the Rust binary alongside MiniLM

**Stacked inference:**
```
hooks-ml receives: {prompt_text, tool_counts, temporal_features, file_features, cost_features}
  → Model 1: MiniLM tokenize + embed prompt → 384-dim vector (~5ms)
  → Concatenate: 384 + 20 numerical = 404-dim feature vector
  → Model 2: Classifier → {session_type, confidence, anomaly_score} (<1ms)
  → Total: ~6ms
```

**Why only ONNX can do this:**
- MeiliSearch embeds text but can't fuse with numerical features
- Ollama consumes text but can't accept a 404-dim tensor
- Pure Go does K-means on numbers but can't incorporate prompt semantics

**Effort:** 1-2 weeks (after 8A is working; mostly Python training + ONNX export)

### 8C: CodeBERT for Edit Diff Embeddings (speculative)

**What:** Embed code diffs from Edit/Write events using a code-aware model.

**Model:** CodeBERT-base (~500MB ONNX, 768-dim output, ~50ms inference)

**Why ONNX is the only option:**
- CodeBERT and UniXcoder are HuggingFace research models
- Not available in Ollama's model library
- HF → ONNX export is the only local inference path

**Preprocessing:**
1. Extract `old_string` + `new_string` from Edit PostToolUse events
2. Optionally: read surrounding context from filesystem
3. Feed through CodeBERT ONNX → 768-dim code embedding

**What it enables:**
- Semantic code change similarity (same kind of change in different files)
- Change type clustering (refactor vs bugfix vs feature without keyword rules)
- Enriched blast radius (similar changes → similar follow-up patterns)

**Data challenge:** Edit events contain only changed lines, not full context. Short diffs may produce noisy embeddings. Needs evaluation.

**Effort:** 1-2 weeks (model export, evaluation of embedding quality on real diffs)

## Rust Binary Design

```
hooks-ml/
├── Cargo.toml
├── src/
│   ├── main.rs          — CLI entry point (subcommands: embed, classify, code-embed)
│   ├── embed.rs         — MiniLM embedding (tokenize + ONNX inference)
│   ├── classify.rs      — Multi-signal classifier (feature concat + ONNX inference)
│   ├── code_embed.rs    — CodeBERT embedding for code diffs
│   └── protocol.rs      — stdin/stdout JSON protocol for Go interop
├── models/              — ONNX model files (downloaded at build/install time)
│   ├── minilm-l6-v2.onnx
│   ├── session-classifier.onnx
│   └── codebert-base.onnx (optional)
└── tokenizers/          — HuggingFace tokenizer configs
    ├── minilm-tokenizer.json
    └── codebert-tokenizer.json
```

**Protocol between Go and Rust:**

```json
// Request (stdin, one JSON line)
{"command": "embed", "text": "fix the race condition in server.go"}

// Response (stdout, one JSON line)
{"vector": [0.12, -0.34, 0.56, ...], "dims": 384, "latency_ms": 5}
```

```json
// Request
{"command": "classify", "features": {"tool_dist": [...], "temporal": [...], "prompt": "..."}}

// Response
{"session_type": "Debugging", "confidence": 0.82, "anomaly_score": 1.3}
```

## Model Management

- Models downloaded on first run or via `hooks-ml download` subcommand
- Store in `~/.local/share/hooks-ml/models/`
- Version pinned in Cargo.toml or config file
- MiniLM: download from HuggingFace Hub ONNX export
- Session classifier: generated by Python training script, stored alongside

## Verification

### 8A
- [ ] hooks-ml embed produces 384-dim vectors matching Ollama's output (cosine sim > 0.95)
- [ ] Latency < 10ms per embedding
- [ ] stdin/stdout protocol works from Go subprocess call

### 8B
- [ ] Classifier accuracy > 80% on held-out sessions (cross-validation)
- [ ] Session type labels match Phase 5 K-means labels
- [ ] End-to-end latency < 10ms (embed + classify)

### 8C
- [ ] CodeBERT produces meaningful embeddings for Go code diffs
- [ ] Similar code changes have high cosine similarity (manual evaluation)
- [ ] Different change types cluster separately

## Files Created

- `hooks-ml/` (new Rust crate) — ONNX inference binary
- `scripts/train_classifier.py` (new) — Training script for session classifier
- `hooks-store/internal/store/transform.go` — Call hooks-ml for sync embedding (8A)
- `hooks-mcp/internal/tools/register.go` — New tools using hooks-ml predictions

## Dependencies

- Rust toolchain (rustup)
- `ort` crate (ONNX runtime, ~50MB shared library)
- `tokenizers` crate (HuggingFace tokenizers)
- Python + sklearn/PyTorch for training scripts only (not runtime)
