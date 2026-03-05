package milvus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hooks-mcp/internal/meili"
)

// TestSemanticSearchPrompts_AnnsField verifies that SemanticSearchPrompts sends
// "prompt_dense" as the AnnsField in its Milvus search request.
func TestSemanticSearchPrompts_AnnsField(t *testing.T) {
	var capturedReq searchRequest

	// Fake Milvus server: capture the search request body and return empty results.
	milvusSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/vectordb/entities/search" {
			if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(milvusResponse{Code: 0, Data: []json.RawMessage{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer milvusSrv.Close()

	// Fake embedding service: return a dummy vector.
	embedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(embedResponse{Embedding: []float32{0.1, 0.2, 0.3}})
	}))
	defer embedSrv.Close()

	embedder := NewEmbedder(embedSrv.URL)
	client := NewMilvusClient(milvusSrv.URL, "", "events", "prompts", "sessions", embedder)

	_, err := client.SemanticSearchPrompts(context.Background(), "test query", meili.PromptSearchOpts{})
	if err != nil {
		t.Fatalf("SemanticSearchPrompts returned error: %v", err)
	}

	if capturedReq.AnnsField != "prompt_dense" {
		t.Errorf("expected AnnsField %q, got %q", "prompt_dense", capturedReq.AnnsField)
	}
}
