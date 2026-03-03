# ML Algorithms & Technologies — Quick Reference

*Extracted from ml-suitability-analysis.md and ml-decision-tables.md*

## Algorithms

| Algorithm | Category | What it does | Input → Output |
|-----------|----------|-------------|----------------|
| RAKE | Keyword extraction | Scores phrases by word co-occurrence graph | Text → ranked keyphrases |
| LDA | Topic modeling | Finds k fixed topics via Dirichlet prior | Corpus → topic distributions per doc |
| HDP | Topic modeling | Like LDA but auto-discovers topic count | Corpus → auto-k topic distributions |
| Spectral clustering | Clustering | Builds similarity graph, clusters via eigenvectors of Laplacian | Similarity matrix → cluster labels |
| DBSCAN | Clustering | Density-based, discovers k and outliers automatically | Vectors → cluster labels + outliers |
| Markov chain | Sequence modeling | Transition probabilities between states | Sequences → P(next\|current) matrix |
| Apriori | Association rules | Finds frequent co-occurring item sets | Item sets → support + confidence pairs |
| Z-score | Anomaly detection | Flags values >Nσ from mean | Feature vector → anomaly flags |
| Isolation Forest | Anomaly detection | Tree-based multivariate anomaly scoring | Feature matrix → anomaly scores |
| Cosine similarity | Similarity metric | Angle between two vectors | Two vectors → score [0,1] |
| PageRank / Betweenness | Graph centrality | Identifies hub/bridge nodes | Graph → node importance scores |
| PCA | Dimensionality reduction | Linear projection to principal components | High-dim → low-dim |
| Autoencoder | Learned embedding | Compress → reconstruct; bottleneck = embedding | Vectors → compressed representation |

## Embedding Models

| Model | Params | Dims | Size | Latency (CPU) | Tokenizer | Availability |
|-------|--------|------|------|---------------|-----------|-------------|
| all-MiniLM-L6-v2 | 22M | 384 | ~90MB | ~5ms | WordPiece | ONNX, HuggingFace |
| nomic-embed-text | 137M | 768 | ~274MB | 50-500ms | SentencePiece | Ollama |
| CodeBERT-base | 125M | 768 | ~500MB | ~50ms | BPE | HuggingFace → ONNX only |
| UniXcoder | 125M | 768 | ~500MB | ~50ms | BPE | HuggingFace → ONNX only |

## Inference Runtimes

| Runtime | Latency | Memory | Capabilities | Setup |
|---------|---------|--------|-------------|-------|
| ONNX Runtime | 1-10ms | 90-300MB | Embedding, classification | High |
| Ollama | 50-500ms | 2-8GB | Embedding, generation, classification | Low |

## MeiliSearch Embedder Sources

| Source | Model runs in | Tokenizer by | Privacy |
|--------|-------------|-------------|---------|
| `ollama` | Ollama service | Ollama | Local |
| `huggingFace` | MeiliSearch process | MeiliSearch | Local |
| `openAi` | OpenAI cloud | OpenAI | Cloud |
| `rest` | Your endpoint | Your endpoint | Your choice |
| `userProvided` | Your code | Your code | Local |

## Tokenizer Types

| Tokenizer | Used by | What it does |
|-----------|---------|-------------|
| Charabia | MeiliSearch keyword search | Language-aware word splitting |
| WordPiece | MiniLM, BERT | Subword splitting for transformers |
| SentencePiece | nomic-embed, LLaMA | Byte-pair variant, language-agnostic |
| BPE (tiktoken) | GPT, CodeBERT | Byte-pair encoding for code/text |
