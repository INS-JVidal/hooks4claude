# ML Enhancement Phases — Overview

*Derived from research/20260302-2240_ml-suitability-analysis.md (March 2026)*

## Phase Map

Each phase is independent and produces value on its own. Later phases build on earlier ones but none are required.

| Phase | Name | Dependencies | Effort | Tech |
|-------|------|-------------|--------|------|
| 1 | [Ollama + MeiliSearch Vector Search](ml-phase1-vector-search.md) | Docker | 1-3 days | MeiliSearch embedder API, docker-compose |
| 2 | [RAKE Keyword Extraction](ml-phase2-rake-keywords.md) | None | 2-3 days | Pure Go in hooks-store |
| 3 | [HDP/LDA Topic Modeling](ml-phase3-topic-modeling.md) | Phase 2 (optional) | 3-5 days | Go or Python batch |
| 4 | [Blast Radius / File Co-modification](ml-phase4-blast-radius.md) | None | 3-5 days | Pure Go, gonum/graph |
| 5 | [Session Clustering](ml-phase5-session-clustering.md) | Phases 2-3 enhance it | 2-3 days | Pure Go |
| 6 | [Markov Chain Workflow Patterns](ml-phase6-markov-chains.md) | None | 2-3 days | Pure Go |
| 7 | [Z-score Anomaly Detection](ml-phase7-anomaly-detection.md) | None | 1-2 days | Pure Go |
| 8 | [ONNX Rust Integration](ml-phase8-onnx-rust.md) | Phases 1-5 inform features | 3-6 weeks | Rust, ort crate |

## Guiding Principles

- **Progressive enrichment:** Each layer adds value independently. Stop at any layer.
- **Data scale awareness:** ~200 sessions, ~10-20K events, ~233 prompts. Most supervised ML is premature. Focus on unsupervised methods and pre-trained models.
- **Compaction-safe:** Each phase file is self-contained. Read only the phase you're working on.
- **Tech stack fit:** Go is primary. Rust only for ONNX (Phase 8). Python only for model training scripts.

## Key Constraint

At 100-200 sessions, heuristics beat most ML. The exceptions: pre-trained embeddings (Phase 1), topic modeling (Phase 3), and file co-modification patterns (Phase 4). See research/20260302-2240_ml-suitability-analysis.md for full analysis.
