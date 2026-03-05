package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const vectorDim = 384

// MilvusStore implements EventStore using Milvus as the backend.
type MilvusStore struct {
	client      *MilvusClient
	embedder    *Embedder
	eventsCol   string
	promptsCol  string
	sessionsCol string
}

// NewMilvusStore creates a MilvusStore connected to the given Milvus instance.
// It ensures collections exist with correct schemas and indexes.
func NewMilvusStore(milvusURL, milvusToken, eventsCol, promptsCol, sessionsCol, embedURL string, recreate ...bool) (*MilvusStore, error) {
	client := NewMilvusClient(milvusURL, milvusToken)

	var embedder *Embedder
	if embedURL != "" {
		embedder = NewEmbedder(embedURL)
	}

	ctx := context.Background()

	if len(recreate) > 0 && recreate[0] {
		for _, col := range []string{eventsCol, promptsCol, sessionsCol} {
			if col == "" {
				continue
			}
			if has, _ := client.HasCollection(ctx, col); has {
				fmt.Fprintf(os.Stderr, "Dropping collection %s for recreation...\n", col)
				if err := client.DropCollection(ctx, col); err != nil {
					return nil, fmt.Errorf("drop %s: %w", col, err)
				}
			}
		}
	}

	if err := ensureEventsCollection(ctx, client, eventsCol); err != nil {
		return nil, fmt.Errorf("events collection: %w", err)
	}
	if promptsCol != "" {
		if err := ensurePromptsCollection(ctx, client, promptsCol); err != nil {
			return nil, fmt.Errorf("prompts collection: %w", err)
		}
	}
	if sessionsCol != "" {
		if err := ensureSessionsCollection(ctx, client, sessionsCol); err != nil {
			return nil, fmt.Errorf("sessions collection: %w", err)
		}
	}

	return &MilvusStore{
		client:      client,
		embedder:    embedder,
		eventsCol:   eventsCol,
		promptsCol:  promptsCol,
		sessionsCol: sessionsCol,
	}, nil
}

func ensureEventsCollection(ctx context.Context, client *MilvusClient, name string) error {
	has, err := client.HasCollection(ctx, name)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	schema := map[string]interface{}{
		"collectionName": name,
		"schema": map[string]interface{}{
			"autoId": false,
			"fields": []map[string]interface{}{
				{"fieldName": "id", "dataType": "VarChar", "isPrimary": true, "elementTypeParams": map[string]interface{}{"max_length": "36"}},
				{"fieldName": "hook_type", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "64"}},
				{"fieldName": "timestamp", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "30"}},
				{"fieldName": "timestamp_unix", "dataType": "Int64"},
				{"fieldName": "session_id", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "64"}},
				{"fieldName": "tool_name", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "128"}},
				{"fieldName": "file_path", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "512"}},
				{"fieldName": "error_message", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "4096"}},
				{"fieldName": "prompt", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "32768"}},
				{"fieldName": "project_dir", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "512"}},
				{"fieldName": "project_name", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "128"}},
				{"fieldName": "cwd", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "512"}},
				{"fieldName": "permission_mode", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "32"}},
				{"fieldName": "cost_usd", "dataType": "Float"},
				{"fieldName": "input_tokens", "dataType": "Int64"},
				{"fieldName": "output_tokens", "dataType": "Int64"},
				{"fieldName": "cache_read_tokens", "dataType": "Int64"},
				{"fieldName": "cache_create_tokens", "dataType": "Int64"},
				{"fieldName": "has_claude_md", "dataType": "Bool"},
				{"fieldName": "data_flat", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{
					"max_length": "65535", "enable_analyzer": true,
					"analyzer_params": map[string]interface{}{"type": "english"},
				}},
				{"fieldName": "data_json", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "65535"}},
				{"fieldName": "dense_embedding", "dataType": "FloatVector", "elementTypeParams": map[string]interface{}{"dim": "384"}},
				{"fieldName": "sparse_embedding", "dataType": "SparseFloatVector"},
				{"fieldName": "dense_valid", "dataType": "Bool"},
			},
			"functions": []map[string]interface{}{{
				"name":             "bm25_events",
				"type":             "BM25",
				"inputFieldNames":  []string{"data_flat"},
				"outputFieldNames": []string{"sparse_embedding"},
			}},
		},
		"indexParams": []map[string]interface{}{
			{"fieldName": "dense_embedding", "indexType": "AUTOINDEX", "metricType": "COSINE"},
			{"fieldName": "sparse_embedding", "indexType": "SPARSE_INVERTED_INDEX", "metricType": "BM25"},
			{"fieldName": "hook_type", "indexType": "AUTOINDEX"},
			{"fieldName": "session_id", "indexType": "AUTOINDEX"},
			{"fieldName": "timestamp_unix", "indexType": "AUTOINDEX"},
			{"fieldName": "tool_name", "indexType": "AUTOINDEX"},
			{"fieldName": "project_name", "indexType": "AUTOINDEX"},
		},
	}
	return client.CreateCollection(ctx, name, schema)
}

func ensurePromptsCollection(ctx context.Context, client *MilvusClient, name string) error {
	has, err := client.HasCollection(ctx, name)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	schema := map[string]interface{}{
		"collectionName": name,
		"schema": map[string]interface{}{
			"autoId": false,
			"fields": []map[string]interface{}{
				{"fieldName": "id", "dataType": "VarChar", "isPrimary": true, "elementTypeParams": map[string]interface{}{"max_length": "36"}},
				{"fieldName": "hook_type", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "64"}},
				{"fieldName": "timestamp", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "30"}},
				{"fieldName": "timestamp_unix", "dataType": "Int64"},
				{"fieldName": "session_id", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "64"}},
				{"fieldName": "prompt", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{
					"max_length": "32768", "enable_analyzer": true,
					"analyzer_params": map[string]interface{}{"type": "english"},
				}},
				{"fieldName": "prompt_length", "dataType": "Int64"},
				{"fieldName": "cwd", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "512"}},
				{"fieldName": "project_dir", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "512"}},
				{"fieldName": "project_name", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "128"}},
				{"fieldName": "permission_mode", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "32"}},
				{"fieldName": "has_claude_md", "dataType": "Bool"},
				{"fieldName": "prompt_dense", "dataType": "FloatVector", "elementTypeParams": map[string]interface{}{"dim": "384"}},
				{"fieldName": "sparse_embedding", "dataType": "SparseFloatVector"},
				{"fieldName": "dense_valid", "dataType": "Bool"},
			},
			"functions": []map[string]interface{}{{
				"name":             "bm25_prompts",
				"type":             "BM25",
				"inputFieldNames":  []string{"prompt"},
				"outputFieldNames": []string{"sparse_embedding"},
			}},
		},
		"indexParams": []map[string]interface{}{
			{"fieldName": "prompt_dense", "indexType": "AUTOINDEX", "metricType": "COSINE"},
			{"fieldName": "sparse_embedding", "indexType": "SPARSE_INVERTED_INDEX", "metricType": "BM25"},
			{"fieldName": "session_id", "indexType": "AUTOINDEX"},
			{"fieldName": "timestamp_unix", "indexType": "AUTOINDEX"},
			{"fieldName": "project_name", "indexType": "AUTOINDEX"},
		},
	}
	return client.CreateCollection(ctx, name, schema)
}

func ensureSessionsCollection(ctx context.Context, client *MilvusClient, name string) error {
	has, err := client.HasCollection(ctx, name)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	schema := map[string]interface{}{
		"collectionName": name,
		"schema": map[string]interface{}{
			"autoId": false,
			"fields": []map[string]interface{}{
				{"fieldName": "id", "dataType": "VarChar", "isPrimary": true, "elementTypeParams": map[string]interface{}{"max_length": "36"}},
				{"fieldName": "session_id", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "64"}},
				{"fieldName": "project_name", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "128"}},
				{"fieldName": "project_dir", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "512"}},
				{"fieldName": "cwd", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "512"}},
				{"fieldName": "started_at", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "30"}},
				{"fieldName": "ended_at", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "30"}},
				{"fieldName": "duration_s", "dataType": "Float"},
				{"fieldName": "source", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "64"}},
				{"fieldName": "model", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "64"}},
				{"fieldName": "permission_mode", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "32"}},
				{"fieldName": "has_claude_md", "dataType": "Bool"},
				{"fieldName": "total_events", "dataType": "Int64"},
				{"fieldName": "total_prompts", "dataType": "Int64"},
				{"fieldName": "total_cost_usd", "dataType": "Float"},
				{"fieldName": "input_tokens", "dataType": "Int64"},
				{"fieldName": "output_tokens", "dataType": "Int64"},
				{"fieldName": "prompt_preview", "dataType": "VarChar", "elementTypeParams": map[string]interface{}{"max_length": "1024"}},
				{"fieldName": "dummy_vector", "dataType": "FloatVector", "elementTypeParams": map[string]interface{}{"dim": "2"}},
			},
		},
		"indexParams": []map[string]interface{}{
			{"fieldName": "dummy_vector", "indexType": "AUTOINDEX", "metricType": "COSINE"},
			{"fieldName": "session_id", "indexType": "AUTOINDEX"},
			{"fieldName": "project_name", "indexType": "AUTOINDEX"},
		},
	}
	return client.CreateCollection(ctx, name, schema)
}

// zeroVector returns a zero vector of the given dimension.
func zeroVector(dim int) []float32 {
	return make([]float32, dim)
}

// Index persists a Document to Milvus. Generates embeddings if an embedder is available.
func (s *MilvusStore) Index(ctx context.Context, doc Document) error {
	// Generate embedding
	embText := EmbeddingText(doc)
	var embedding []float32
	denseValid := false

	if s.embedder != nil && embText != "" {
		vecs, err := s.embedder.Embed(ctx, []string{embText})
		if err == nil && vecs != nil && len(vecs) > 0 {
			embedding = vecs[0]
			denseValid = true
		}
	}
	if embedding == nil {
		embedding = zeroVector(vectorDim)
	}

	dataJSON := DataToJSON(doc.Data)

	// Truncate fields to fit Milvus VarChar limits.
	record := map[string]interface{}{
		"id":                 doc.ID,
		"hook_type":          truncate(doc.HookType, 64),
		"timestamp":          truncate(doc.Timestamp, 30),
		"timestamp_unix":     doc.TimestampUnix,
		"session_id":         truncate(doc.SessionID, 64),
		"tool_name":          truncate(doc.ToolName, 128),
		"file_path":          truncate(doc.FilePath, 512),
		"error_message":      truncate(doc.ErrorMessage, 4096),
		"prompt":             truncate(doc.Prompt, 32768),
		"project_dir":        truncate(doc.ProjectDir, 512),
		"project_name":       truncate(doc.ProjectName, 128),
		"cwd":                truncate(doc.Cwd, 512),
		"permission_mode":    truncate(doc.PermissionMode, 32),
		"cost_usd":           float32(doc.CostUSD),
		"input_tokens":       doc.InputTokens,
		"output_tokens":      doc.OutputTokens,
		"cache_read_tokens":  doc.CacheReadTokens,
		"cache_create_tokens": doc.CacheCreateTokens,
		"has_claude_md":      doc.HasClaudeMD,
		"data_flat":          truncate(doc.DataFlat, 65535),
		"data_json":          truncate(dataJSON, 65535),
		"dense_embedding":    embedding,
		"dense_valid":        denseValid,
	}

	if err := s.client.Insert(ctx, s.eventsCol, []map[string]interface{}{record}); err != nil {
		return fmt.Errorf("insert event %s: %w", doc.ID, err)
	}

	// Dual-write UserPromptSubmit to prompts collection.
	if s.promptsCol != "" && doc.HookType == "UserPromptSubmit" {
		var promptEmb []float32
		promptValid := false
		if s.embedder != nil && doc.Prompt != "" {
			vecs, err := s.embedder.Embed(ctx, []string{doc.Prompt})
			if err == nil && vecs != nil && len(vecs) > 0 {
				promptEmb = vecs[0]
				promptValid = true
			}
		}
		if promptEmb == nil {
			promptEmb = zeroVector(vectorDim)
		}

		promptRecord := map[string]interface{}{
			"id":              doc.ID,
			"hook_type":       truncate(doc.HookType, 64),
			"timestamp":       truncate(doc.Timestamp, 30),
			"timestamp_unix":  doc.TimestampUnix,
			"session_id":      truncate(doc.SessionID, 64),
			"prompt":          truncate(doc.Prompt, 32768),
			"prompt_length":   int64(len(doc.Prompt)),
			"cwd":             truncate(doc.Cwd, 512),
			"project_dir":     truncate(doc.ProjectDir, 512),
			"project_name":    truncate(doc.ProjectName, 128),
			"permission_mode": truncate(doc.PermissionMode, 32),
			"has_claude_md":   doc.HasClaudeMD,
			"prompt_dense":    promptEmb,
			"dense_valid":     promptValid,
		}

		if err := s.client.Insert(ctx, s.promptsCol, []map[string]interface{}{promptRecord}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: prompts insert failed for %s: %v\n", doc.ID, err)
		}
	}

	return nil
}

// Close is a no-op for MilvusStore — the HTTP client has no persistent resources.
func (s *MilvusStore) Close() error {
	return nil
}

// truncate returns s truncated to at most maxLen bytes, ensuring the result
// does not cut a multi-byte UTF-8 codepoint in half.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Walk back from maxLen to find a valid rune boundary.
	for maxLen > 0 && !utf8.RuneStart(s[maxLen]) {
		maxLen--
	}
	return s[:maxLen]
}

// EmbeddingText concatenates prompt, tool_name, and error_message for embedding.
func EmbeddingText(doc Document) string {
	parts := []string{}
	if doc.Prompt != "" {
		parts = append(parts, doc.Prompt)
	}
	if doc.ToolName != "" {
		parts = append(parts, doc.ToolName)
	}
	if doc.ErrorMessage != "" {
		parts = append(parts, doc.ErrorMessage)
	}
	return strings.Join(parts, " ")
}

// DataToJSON serializes a data map to a JSON string.
func DataToJSON(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	b, err := json.Marshal(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DataToJSON: marshal failed: %v\n", err)
		return "{}"
	}
	return string(b)
}
