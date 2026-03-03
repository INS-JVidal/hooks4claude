# ML Topic Modeling & Keyword Extraction for Hook Data

**Date:** 2026-03-02
**Context:** Exploring how ML algorithms can extract insights from hooks4claude prompt data stored in MeiliSearch.

---

## 1. ML Algorithms for Data Science — Overview

| Category | Algorithm | Use Case |
|---|---|---|
| **Regression** | Linear Regression | Continuous prediction |
| **Regression** | Ridge / Lasso | Regularized regression |
| **Regression** | Polynomial Regression | Non-linear relationships |
| **Regression** | Elastic Net | Mixed regularization |
| **Classification** | Logistic Regression | Binary/multi-class |
| **Classification** | K-Nearest Neighbors (KNN) | Instance-based classification |
| **Classification** | Support Vector Machine (SVM) | High-dimensional separation |
| **Classification** | Naive Bayes | Text/probabilistic classification |
| **Tree-Based** | Decision Tree | Interpretable splits |
| **Tree-Based** | Random Forest | Ensemble bagging |
| **Tree-Based** | Gradient Boosting (GBM) | Sequential boosting |
| **Tree-Based** | XGBoost / LightGBM / CatBoost | Optimized boosting |
| **Clustering** | K-Means | Partition-based grouping |
| **Clustering** | DBSCAN | Density-based grouping |
| **Clustering** | Hierarchical Clustering | Dendrogram-based |
| **Clustering** | Gaussian Mixture Models (GMM) | Probabilistic clustering |
| **Dimensionality Reduction** | PCA | Linear projection |
| **Dimensionality Reduction** | t-SNE | Non-linear visualization |
| **Dimensionality Reduction** | UMAP | Fast manifold embedding |
| **Dimensionality Reduction** | LDA | Topic modeling / projection |
| **Time Series** | ARIMA / SARIMA | Autoregressive forecasting |
| **Time Series** | Prophet | Trend + seasonality |
| **Time Series** | LSTM (RNN) | Sequence modeling |
| **Neural Networks** | MLP (Feedforward) | General-purpose deep learning |
| **Neural Networks** | CNN | Image / spatial data |
| **Neural Networks** | Transformer | NLP / sequence tasks |
| **Neural Networks** | Autoencoder | Anomaly detection / compression |
| **Association** | Apriori | Market basket analysis |
| **Association** | FP-Growth | Frequent pattern mining |

---

## 2. LDA & Related Topic/Representation Learning Algorithms

### Algorithm Landscape

| Category | Algorithm | Key Idea |
|---|---|---|
| **Classical Topic Models** | Latent Dirichlet Allocation (LDA) | Bayesian generative model — documents as mixtures of topics, topics as mixtures of words |
| **Classical Topic Models** | Latent Semantic Analysis (LSA/LSI) | SVD on term-document matrix to find latent semantic dimensions |
| **Classical Topic Models** | Non-Negative Matrix Factorization (NMF) | Factorizes term-document matrix with non-negative constraints — more interpretable topics |
| **Classical Topic Models** | Probabilistic LSA (pLSA) | Probabilistic version of LSA — LDA's predecessor, lacks proper prior |
| **Extended Topic Models** | Hierarchical Dirichlet Process (HDP) | Like LDA but automatically discovers the number of topics |
| **Extended Topic Models** | Correlated Topic Model (CTM) | Captures correlations between topics (LDA assumes independence) |
| **Extended Topic Models** | Dynamic Topic Model (DTM) | Topics evolve over time — great for temporal corpora |
| **Extended Topic Models** | Author-Topic Model | Joint modeling of authors and topics |
| **Neural Topic Models** | Neural Variational Document Model (NVDM) | VAE-based document representation |
| **Neural Topic Models** | ProdLDA / ETM | Neural LDA replacements using variational autoencoders |
| **Neural Topic Models** | BERTopic | BERT embeddings + UMAP + HDBSCAN clustering — topic labels via c-TF-IDF |
| **Neural Topic Models** | Top2Vec | Doc2Vec embeddings + UMAP + HDBSCAN — no stop-word removal needed |
| **Neural Topic Models** | CTM (Contextualized TM) | Combines SBERT embeddings with neural topic modeling |
| **Embedding-Based** | Word2Vec / GloVe / FastText | Word-level dense representations (foundation for many above) |
| **Embedding-Based** | Doc2Vec | Document-level embeddings |
| **Embedding-Based** | Sentence-BERT (SBERT) | Transformer sentence embeddings — powers BERTopic/Top2Vec |
| **Deep Generative** | Variational Autoencoder (VAE) | Learns latent representations — basis for neural topic models |
| **Deep Generative** | Seq2Seq + Attention | Encoder-decoder for text generation/summarization |
| **Deep Generative** | Transformer (BERT, GPT) | Contextual embeddings — replaced most classical NLP pipelines |

### Evolution Path

- **Classical path:** LSA → pLSA → **LDA** → HDP/DTM
- **Modern path:** Word2Vec → BERT → **BERTopic** (current state-of-the-art for most use cases)

BERTopic is where the field has largely converged — it combines transformer embeddings with clustering, giving you the interpretability of LDA with the representational power of deep learning. LDA remains valuable for understanding the foundations and for cases where interpretability and simplicity matter most.

---

## 3. Application to hooks4claude Data

### Best Fit: Prompt Topic Analysis

`UserPromptSubmit` events contain prompt text in `data.prompt` — a natural corpus for topic modeling.

| Use Case | What You'd Learn | Algorithm |
|---|---|---|
| **Prompt topic clustering** | What kinds of tasks do you ask Claude? (refactoring, debugging, docs, git ops...) | BERTopic / LDA |
| **Session topic evolution** | How does a session's focus shift over time? (starts with exploration → ends with implementation) | Dynamic Topic Model / BERTopic over time |
| **Topic vs cost correlation** | Which topic categories are most expensive in tokens? | BERTopic + `cost-analysis` MCP tool data |
| **Tool-topic mapping** | Which tools get used for which prompt topics? | NMF/LDA on prompts, cross-referenced with `tool_name` from PreToolUse events |

### Example Implementation

```python
# 1. Extract prompts from MeiliSearch
import meilisearch

client = meilisearch.Client('http://localhost:7700')
index = client.index('hook-prompts')  # or hook-events filtered by hook_type
results = index.search('', {'filter': 'hook_type = UserPromptSubmit', 'limit': 1000})
prompts = [hit['data']['prompt'] for hit in results['hits']]

# 2. Run BERTopic
from bertopic import BERTopic

model = BERTopic(min_topic_size=3)
topics, probs = model.fit_transform(prompts)

# 3. See what you ask Claude about
model.get_topic_info()
# → Topic 0: "refactor", "function", "rename"
# → Topic 1: "git", "commit", "branch"
# → Topic 2: "bug", "error", "fix"
# → Topic 3: "test", "coverage", "assert"
```

### Pipeline Architecture

```
Prompt arrives → Extract keywords → Store as field → MeiliSearch indexes it
                                          |
                                   keywords: ["refactor", "error-handling", "golang"]
                                          |
                                   Now filterable, facetable, aggregatable
```

---

## 4. Keywords vs MeiliSearch Inverted Index

### Core Distinction

```
Inverted index:   token → which documents contain it     (retrieval)
Keywords field:   document → what it's about              (description)
```

They point in opposite directions. The inverted index answers "where is X?". Keywords answer "what is this?".

### Comparison

| Operation | MeiliSearch Inverted Index | Keywords Field |
|---|---|---|
| Full-text search | Already optimal | No benefit |
| "What are this prompt's topics?" | Must re-read full text | `doc.keywords` → instant |
| "Top 10 topics this week" | Can't aggregate tokens | Facet query on `keywords` |
| "Sessions about debugging" | Search ranks by relevance, not categorizes | `filter: keywords = debugging` → exact match |
| "Topic distribution over time" | Not possible | Group by keyword + timestamp |
| "Compare my usage patterns" | Not designed for this | Keyword frequency analysis |

### Verdict

- **For search only** — don't bother, MeiliSearch already wins.
- **For analytics, tagging, faceting, and trend detection** — keywords as a stored field are worth it.
- The extraction step (RAKE/YAKE) is what creates value, not the data structure.

---

## 5. RAKE — Best Algorithm for Keyphrase Extraction

### How RAKE Works

1. Split text on stop words and punctuation → candidate phrases
2. Build a word co-occurrence matrix from those candidates
3. Score each word: `degree(word) / frequency(word)`
4. Phrase score = sum of its word scores
5. Rank phrases by score

```
Input:  "refactor the authentication handler to use middleware patterns"
              | split on stop words ("the", "to", "use")
Candidates:  ["refactor", "authentication handler", "middleware patterns"]
              | score by word co-occurrence
Output:  "authentication handler" (4.0), "middleware patterns" (4.0), "refactor" (1.0)
```

### Comparison of Keyword/Keyphrase Extraction Methods

| | RAKE | YAKE | TF-IDF | KeyBERT |
|---|---|---|---|---|
| **Multi-word phrases** | Native | Yes | Single words | Yes |
| **Speed** | Very fast | Fast | Very fast | Slow |
| **No training data** | Unsupervised | Unsupervised | Needs corpus | Needs model |
| **Dependencies** | Stop word list only | Small lib | Corpus stats | Transformer |
| **Go implementation** | Simple (~100 LOC) | Harder | Simple | Not practical |

### Why RAKE for hooks-store Ingest Pipeline

- Pure algorithm, no ML — implementable in Go directly in hooks-store
- Fast enough for real-time ingest (microseconds per prompt)
- Prompts are short (1-3 sentences typically) — RAKE works best on short text
- Only dependency is a stop word list (embed as a Go `map[string]bool`)

### Main Weakness

RAKE is purely statistical — it doesn't understand semantics. `"authentication handler"` and `"auth middleware"` would be separate keyphrases. For faceting and trend detection on hook data, that's acceptable.

### Enriched Document Schema

```json
{
  "id": "evt_abc123",
  "hook_type": "UserPromptSubmit",
  "prompt": "refactor the authentication handler to use middleware",
  "keywords": ["refactor", "authentication handler", "middleware"],
  "session_id": "ses_xyz"
}
```

### MeiliSearch Configuration for Keywords

```bash
# Make keywords filterable and facetable
curl -X PATCH 'http://localhost:7700/indexes/hook-prompts/settings' \
  -d '{
    "filterableAttributes": ["keywords"],
    "faceting": {"sortFacetValuesBy": {"keywords": "count"}}
  }'
```

This enables:
- **Fast access:** `filter: "keywords = refactor"` → all refactoring prompts
- **Frequency:** facet query on `keywords` → top 20 topics instantly
- **Cross-reference:** `filter: "keywords = debugging AND session_id = ses_xyz"`

---

## 6. Possible Next Steps

1. **Implement RAKE in Go** (~100 LOC) inside `hooks-store/internal/` as a new `keywords` package
2. **Enrich at ingest time** — extract keywords before indexing in MeiliSearch
3. **Add `keywords` to filterable/facetable attributes** in MeiliSearch settings
4. **Add MCP tool** — `topic-analysis` tool in hooks-mcp for keyword facet queries
5. **Explore BERTopic** offline (Python) for deeper semantic clustering on accumulated data
