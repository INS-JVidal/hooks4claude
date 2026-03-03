# Phase 3: HDP/LDA Topic Modeling

*Discover latent themes across all prompts, auto-label sessions*

## Goal

Run topic modeling on the prompt corpus to discover recurring themes (exploration, debugging, feature work, refactoring, etc.) and assign per-prompt topic distributions. Store topic vectors as a new field — these are lightweight embeddings (5-10 dims) that enable session classification.

## Why

- Topic distributions ARE embeddings — `[0.6, 0.2, 0.1, 0.05, 0.03, 0.02]` is a 6-dim vector
- Auto-discovers session types from text without manual rules
- 33-dim fingerprint (with LDA) trains better than 404-dim (with MiniLM) at 200 sessions
- Interpretable — each topic has human-readable word lists
- Enables: "Am I spending more time debugging than building?"

## LDA vs HDP

| | LDA | HDP |
|---|---|---|
| Number of topics | You specify k | Auto-discovered |
| At 233 prompts | k=5-8 is a safe starting point | Likely finds 3-5 stable topics |
| Stability | More stable with fixed k | Can be noisy at small corpus size |
| Recommendation | **Start with LDA k=6** | Try HDP later to validate k choice |

## Prerequisites

- Phase 2 (RAKE keywords) is optional but recommended — feeding RAKE keyphrases instead of raw prompts reduces vocabulary noise and gives LDA cleaner signal

## Implementation Options

### Option A: Python batch script (recommended for prototyping)

```python
# scripts/topic_model.py
from gensim import corpora, models
import requests

# 1. Fetch all prompts from MeiliSearch
prompts = fetch_all_prompts("http://localhost:7700")

# 2. Preprocess: tokenize, remove stop words
# (or use RAKE keywords from Phase 2 if available)
corpus = [preprocess(p["prompt"]) for p in prompts]

# 3. Build dictionary and bag-of-words
dictionary = corpora.Dictionary(corpus)
bow_corpus = [dictionary.doc2bow(doc) for doc in corpus]

# 4. Train LDA
lda = models.LdaModel(bow_corpus, num_topics=6, id2word=dictionary, passes=15)

# 5. Extract topic distributions per prompt
for prompt, bow in zip(prompts, bow_corpus):
    topic_dist = lda.get_document_topics(bow, minimum_probability=0.01)
    # topic_dist = [(topic_id, probability), ...]

# 6. Write topic vectors back to MeiliSearch
update_prompts_with_topics(prompts, topic_distributions)
```

### Option B: Go implementation

Use `github.com/james-bowman/nlp` or implement LDA from scratch. More complex but keeps the stack in Go. Best deferred until the approach is validated with Python.

### Option C: Ollama-assisted topic labeling

After LDA discovers topics, use Ollama to generate human-readable topic names:
```
Prompt: "These words define a topic in developer conversations:
refactor, rename, extract, clean, simplify, handler, function.
Give this topic a short 2-3 word name."
→ "Code Refactoring"
```

## New Fields

### hook-prompts (per prompt)

```json
{
  "topic_vector": [0.6, 0.2, 0.1, 0.05, 0.03, 0.02],
  "dominant_topic": 0,
  "dominant_topic_name": "Exploration"
}
```

### hook-sessions (aggregated)

```json
{
  "session_topic_vector": [0.35, 0.25, 0.2, 0.1, 0.05, 0.05],
  "session_type": "Exploration/Debugging"
}
```

Session topic vector = mean of all prompt topic vectors in that session.
Session type = top 1-2 dominant topics, named.

## MeiliSearch Config

- `topic_vector`: filterable (enables range queries on topic proportions)
- `dominant_topic_name`: filterable, searchable
- `session_type`: filterable, searchable on hook-sessions

## Data Scale Considerations

- 233 prompts is borderline for LDA. Expect coarse topics.
- With RAKE preprocessing, effective vocabulary shrinks — helps stability.
- At 500+ prompts, retrain LDA for finer-grained topics.
- Topic model should be retrained periodically (weekly or on-demand) as new prompts accumulate.

## Verification

- [ ] LDA produces 5-8 topics with distinct, interpretable word distributions
- [ ] Each prompt has a topic_vector that sums to ~1.0
- [ ] Topic labels match human intuition (manually review 10-20 prompts)
- [ ] Session-level aggregation correctly reflects the session's work type
- [ ] Filtering by `dominant_topic_name = "Debugging"` returns sensible sessions

## Files Modified/Created

- `scripts/topic_model.py` (new) — Python LDA training + MeiliSearch update
- `hooks-store/internal/store/store.go` — Add topic fields to PromptDocument and SessionDocument
- `hooks-store/internal/store/meili.go` — Add topic fields to searchable/filterable attributes

## Feeds Into

- **Phase 5 (Session Clustering):** LDA topic vector replaces or augments tool-only features
- **Phase 8 (ONNX):** LDA topics as 8-dim features in the 33-dim fingerprint (much lighter than MiniLM's 384 dims)
