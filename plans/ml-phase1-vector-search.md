# Phase 1: Ollama + MeiliSearch Vector Search

*Foundation for all embedding-based features*

## Goal

Enable hybrid search (keyword + semantic) on hook-prompts and hook-sessions by adding Ollama to the Docker stack and configuring MeiliSearch's built-in embedder.

## Why This First

- Zero code changes needed — MeiliSearch handles embedding, storage, and search natively
- Proves the value of semantic search before investing in custom embedding pipelines
- Infrastructure (Ollama in Docker) is reused by Phases 3 and 8

## Current State

- MeiliSearch v1.36.0 running in docker-compose.yml
- No embedder configured, no vector search enabled
- Three indexes: hook-events, hook-prompts, hook-sessions

## Steps

### 1. Add Ollama to docker-compose.yml

```yaml
services:
  ollama:
    image: ollama/ollama
    volumes:
      - ollama-models:/root/.ollama
    restart: unless-stopped

volumes:
  ollama-models:
```

Add `MEILI_OLLAMA_URL: "http://ollama:11434"` to meilisearch environment.

### 2. Pull embedding model

```bash
docker compose exec ollama ollama pull nomic-embed-text
```

nomic-embed-text: 768 dims, ~274MB, SentencePiece tokenizer (handled by Ollama internally).

### 3. Configure embedder on hook-prompts

```bash
curl -X PATCH 'http://localhost:7700/indexes/hook-prompts/settings' \
  -H 'Content-Type: application/json' \
  -d '{
    "embedders": {
      "prompt-semantic": {
        "source": "ollama",
        "url": "http://ollama:11434/api/embeddings",
        "model": "nomic-embed-text",
        "documentTemplate": "A user prompt to Claude Code: {{doc.prompt}}"
      }
    }
  }'
```

MeiliSearch will auto-embed all existing and future documents. Monitor task progress via `GET /tasks`.

### 4. Configure embedder on hook-sessions

```bash
curl -X PATCH 'http://localhost:7700/indexes/hook-sessions/settings' \
  -H 'Content-Type: application/json' \
  -d '{
    "embedders": {
      "session-semantic": {
        "source": "ollama",
        "url": "http://ollama:11434/api/embeddings",
        "model": "nomic-embed-text",
        "documentTemplate": "A Claude Code session in project {{doc.project_name}}: {{doc.prompt_preview}}"
      }
    }
  }'
```

### 5. Test hybrid search

```bash
# Semantic prompt search
curl -X POST 'http://localhost:7700/indexes/hook-prompts/search' \
  -H 'Content-Type: application/json' \
  -d '{
    "q": "fix the race condition",
    "hybrid": {
      "embedder": "prompt-semantic",
      "semanticRatio": 0.5
    }
  }'

# Try different ratios:
# 0.0 = pure keyword, 0.5 = balanced, 1.0 = pure semantic
```

### 6. Evaluate quality

Test with queries that should find results by meaning, not keywords:
- "authentication issues" should find prompts about "login", "JWT", "middleware"
- "make it faster" should find prompts about "optimize", "performance", "cache"
- "clean up the code" should find prompts about "refactor", "simplify", "rename"

Document which semanticRatio gives best results for this data.

## Verification

- [ ] `docker compose up` starts Ollama alongside MeiliSearch and hooks-store
- [ ] `docker compose exec ollama ollama list` shows nomic-embed-text
- [ ] MeiliSearch task for embedder config completes successfully
- [ ] Hybrid search on hook-prompts returns semantically relevant results
- [ ] Hybrid search on hook-sessions finds similar sessions by prompt meaning

## Files Modified

- `docker-compose.yml` — Add ollama service, MEILI_OLLAMA_URL env var
- No Go code changes

## Tokenizer Note

You do NOT need to handle tokenization. The pipeline is:
MeiliSearch renders documentTemplate -> sends text to Ollama -> Ollama tokenizes with SentencePiece -> embeds -> returns vector -> MeiliSearch stores in HNSW index.

## Estimated Embedding Time

233 prompts at ~50-500ms each via Ollama = 12 seconds to 2 minutes for initial backfill. Future documents embed at ingest time (async, non-blocking).
