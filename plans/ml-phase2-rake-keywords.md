# Phase 2: RAKE Keyword Extraction

*Per-prompt keyword enrichment, zero dependencies*

## Goal

Extract multi-word keyphrases from each user prompt using RAKE (Rapid Automatic Keyword Extraction) and store as a new filterable/searchable field in MeiliSearch.

## Why

- Fills the gap between `data_flat` (noisy, all strings dumped) and neural embeddings (heavy infrastructure)
- Keywords are human-readable and immediately useful for filtering
- Preprocesses input for Phase 3 (LDA topic modeling) — cleaner vocabulary = better topics
- Zero external dependencies — pure algorithmic text processing

## How RAKE Works

1. Split text on stop words and punctuation → candidate keyphrases
2. Build word co-occurrence matrix within the document
3. Score each keyphrase by sum of word scores (degree/frequency ratio)
4. Return top-N keyphrases by score

Example: "explore the suitability and efficiency of ML algorithms on the stored data"
→ `["ml algorithms", "stored data", "suitability", "efficiency"]`

## Implementation

### Where in the pipeline

hooks-store `internal/store/transform.go` — during `HookEventToDocument`. Only for UserPromptSubmit events (where `prompt` field is non-empty).

### New fields

In `store.go` Document struct:
```go
Keywords []string `json:"keywords,omitempty"` // RAKE-extracted keyphrases
```

In `store.go` PromptDocument struct:
```go
Keywords []string `json:"keywords,omitempty"`
```

### MeiliSearch index config

In `meili.go`, add to hook-prompts settings:
- Searchable: add `keywords`
- Filterable: add `keywords`

In hook-events settings:
- Searchable: add `keywords`

### RAKE implementation

Options:
1. **Go library:** `github.com/afjoseph/rake` or implement from scratch (~100 lines)
2. **From scratch:** RAKE is simple enough to inline — needs only a stop word list and whitespace/punctuation splitting

Core algorithm (if implementing from scratch):
```
func ExtractKeywords(text string, topN int) []string {
    1. Lowercase, split on stop words → candidate phrases
    2. For each word: compute degree (co-occurrence count) and frequency
    3. Word score = degree / frequency
    4. Phrase score = sum of word scores
    5. Sort phrases by score descending, return top N
}
```

Stop word list: Use a standard English list (~150 words). Store as a package-level variable.

### Integration in transform.go

```go
func HookEventToDocument(evt hookevt.HookEvent) Document {
    // ... existing extraction ...

    if doc.Prompt != "" {
        doc.Keywords = rake.Extract(doc.Prompt, 5) // top 5 keyphrases
    }

    return doc
}
```

### Migration for existing data

Re-index existing hook-prompts documents through the transform pipeline, or run a one-time batch script that:
1. Fetches all prompts from hook-prompts
2. Extracts keywords for each
3. Updates documents via MeiliSearch API

## Verification

- [ ] RAKE extracts meaningful multi-word phrases from real prompts (not just single common words)
- [ ] Keywords are stored in hook-prompts and hook-events indexes
- [ ] MeiliSearch facet query on `keywords` shows useful distribution
- [ ] Filtering by keyword returns relevant prompts: `filter: "keywords = 'authentication'"`
- [ ] Score threshold is tuned — keywords are specific, not generic

## Files Modified

- `hooks-store/internal/store/store.go` — Add Keywords field to Document and PromptDocument
- `hooks-store/internal/store/transform.go` — Call RAKE in HookEventToDocument
- `hooks-store/internal/store/meili.go` — Add keywords to searchable/filterable attributes
- `hooks-store/internal/rake/` (new package) — RAKE implementation or wrapper

## Output Format

```json
{
  "id": "abc-123",
  "prompt": "explore the suitability of ML algorithms on the stored data",
  "keywords": ["ml algorithms", "stored data", "suitability"],
  "session_id": "def-456",
  "timestamp_unix": 1709337600
}
```

## Feeds Into

- **Phase 3 (LDA):** RAKE keywords as cleaner input vocabulary
- **Phase 5 (Session Clustering):** Keyword categories as features (count of debug-related vs feature-related keywords)
- **Phase 8 (ONNX):** RAKE feature counts in the multi-signal fingerprint vector
