// Package milvus implements the meili.Searcher interface using Milvus REST API v2.
package milvus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"time"

	"hooks-mcp/internal/meili"
)

// MilvusClient implements meili.Searcher against a Milvus REST API v2 backend.
type MilvusClient struct {
	baseURL     string
	token       string
	httpClient  *http.Client
	eventsCol   string
	promptsCol  string
	sessionsCol string
	embedder    *Embedder
}

// NewMilvusClient creates a new Milvus REST API client.
func NewMilvusClient(baseURL, token, eventsCol, promptsCol, sessionsCol string, embedder *Embedder) *MilvusClient {
	return &MilvusClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		token:       token,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		eventsCol:   eventsCol,
		promptsCol:  promptsCol,
		sessionsCol: sessionsCol,
		embedder:    embedder,
	}
}

// --- Milvus REST API v2 request/response types ---

type queryRequest struct {
	CollectionName string   `json:"collectionName"`
	Filter         string   `json:"filter,omitempty"`
	OutputFields   []string `json:"outputFields,omitempty"`
	Limit          int      `json:"limit,omitempty"`
	Offset         int      `json:"offset,omitempty"`
}

type searchRequest struct {
	CollectionName string    `json:"collectionName"`
	Data           [][]float32 `json:"data"`
	AnnsField      string    `json:"annsField"`
	Limit          int       `json:"limit,omitempty"`
	Filter         string    `json:"filter,omitempty"`
	OutputFields   []string  `json:"outputFields,omitempty"`
}

type milvusResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message,omitempty"`
	Data    []json.RawMessage `json:"data"`
}

// --- HTTP helpers ---

func (c *MilvusClient) doPost(ctx context.Context, path string, body interface{}) (*milvusResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("milvus request %s: %w", path, err)
	}
	defer resp.Body.Close()

	const maxResponseSize = 50 << 20 // 50 MB
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read milvus response: %w", err)
	}
	if int64(len(respBody)) > maxResponseSize {
		return nil, fmt.Errorf("milvus response exceeds %d bytes limit", maxResponseSize)
	}

	var out milvusResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode milvus response: %w (body: %s)", err, string(respBody))
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("milvus error code %d: %s", out.Code, out.Message)
	}
	return &out, nil
}

func decodeHits[T any](data []json.RawMessage) ([]T, error) {
	hits := make([]T, 0, len(data))
	for _, raw := range data {
		var h T
		if err := json.Unmarshal(raw, &h); err != nil {
			return nil, fmt.Errorf("decode hit: %w", err)
		}
		hits = append(hits, h)
	}
	return hits, nil
}

// --- Filter builders (Milvus syntax: ==, &&, ||, like) ---

func buildMilvusSessionFilter(opts meili.SessionSearchOpts) string {
	var parts []string
	if opts.SessionID != "" {
		parts = append(parts, fmt.Sprintf("session_id == %q", opts.SessionID))
	}
	if opts.ProjectName != "" {
		parts = append(parts, fmt.Sprintf("project_name == %q", opts.ProjectName))
	}
	if opts.Model != "" {
		parts = append(parts, fmt.Sprintf("model == %q", opts.Model))
	}
	if opts.Source != "" {
		parts = append(parts, fmt.Sprintf("source == %q", opts.Source))
	}
	if !opts.DateRange.IsZero {
		parts = append(parts, fmt.Sprintf("started_at >= %q", opts.DateRange.StartISO))
		parts = append(parts, fmt.Sprintf("started_at < %q", opts.DateRange.EndISO))
	}
	return strings.Join(parts, " && ")
}

func buildMilvusPromptFilter(opts meili.PromptSearchOpts) string {
	var parts []string
	if opts.ProjectName != "" {
		parts = append(parts, fmt.Sprintf("project_name == %q", opts.ProjectName))
	}
	if opts.SessionID != "" {
		parts = append(parts, fmt.Sprintf("session_id == %q", opts.SessionID))
	}
	if !opts.DateRange.IsZero {
		parts = append(parts, fmt.Sprintf("timestamp_unix >= %d", opts.DateRange.StartUnix))
		parts = append(parts, fmt.Sprintf("timestamp_unix < %d", opts.DateRange.EndUnix))
	}
	return strings.Join(parts, " && ")
}

func buildMilvusEventFilter(opts meili.EventSearchOpts) string {
	var parts []string
	if opts.ProjectName != "" {
		parts = append(parts, fmt.Sprintf("project_name == %q", opts.ProjectName))
	}
	if opts.SessionID != "" {
		parts = append(parts, fmt.Sprintf("session_id == %q", opts.SessionID))
	}
	if opts.HookType != "" {
		parts = append(parts, fmt.Sprintf("hook_type == %q", opts.HookType))
	}
	if opts.ToolName != "" {
		parts = append(parts, fmt.Sprintf("tool_name == %q", opts.ToolName))
	}
	if !opts.DateRange.IsZero {
		parts = append(parts, fmt.Sprintf("timestamp_unix >= %d", opts.DateRange.StartUnix))
		parts = append(parts, fmt.Sprintf("timestamp_unix < %d", opts.DateRange.EndUnix))
	}
	return strings.Join(parts, " && ")
}

func clamp(val, defaultVal, maxVal int64) int64 {
	if val <= 0 {
		return defaultVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

// --- Session output fields ---

var sessionOutputFields = []string{
	"id", "session_id", "project_name", "project_dir", "cwd",
	"started_at", "ended_at", "duration_s", "source", "model",
	"permission_mode", "has_claude_md", "total_events", "total_prompts",
	"compaction_count", "read_count", "edit_count", "bash_count",
	"grep_count", "glob_count", "write_count", "agent_count", "other_tool_count",
	"total_cost_usd", "input_tokens", "output_tokens",
	"prompt_preview", "files_read", "files_written",
}

var promptOutputFields = []string{
	"id", "hook_type", "timestamp", "timestamp_unix", "session_id",
	"prompt", "prompt_length", "cwd", "project_dir", "project_name",
	"permission_mode", "has_claude_md",
}

var eventOutputFields = []string{
	"id", "hook_type", "timestamp", "timestamp_unix", "session_id",
	"tool_name", "has_claude_md", "input_tokens", "output_tokens",
	"cache_read_tokens", "cache_create_tokens", "cost_usd",
	"prompt", "file_path", "error_message", "project_dir", "project_name",
	"permission_mode", "cwd", "last_message", "transcript_path",
	"source", "task_subject", "model", "data_flat", "data",
}

// --- Searcher implementation ---

// countEntities returns the number of entities matching filter in the collection.
// On error, returns -1 (callers can use len(hits) as fallback).
func (c *MilvusClient) countEntities(ctx context.Context, collection, filter string) int64 {
	q := queryRequest{
		CollectionName: collection,
		Filter:         filter,
		OutputFields:   []string{"count(*)"},
	}
	resp, err := c.doPost(ctx, "/v2/vectordb/entities/query", q)
	if err != nil {
		return -1
	}
	if len(resp.Data) == 0 {
		return -1
	}
	var row map[string]interface{}
	if err := json.Unmarshal(resp.Data[0], &row); err != nil {
		return -1
	}
	if cnt, ok := row["count(*)"]; ok {
		switch v := cnt.(type) {
		case float64:
			return int64(v)
		case json.Number:
			n, _ := v.Int64()
			return n
		}
	}
	return -1
}

func (c *MilvusClient) SearchSessions(ctx context.Context, opts meili.SessionSearchOpts) ([]meili.SessionHit, int64, error) {
	filter := buildMilvusSessionFilter(opts)
	limit := int(clamp(opts.Limit, 20, 100))

	// If query is set, add a LIKE filter on prompt_preview (closest text field in sessions).
	if opts.Query != "" {
		likeClause := fmt.Sprintf("prompt_preview like \"%%%s%%\"", escapeMilvusLike(opts.Query))
		if filter != "" {
			filter = filter + " && " + likeClause
		} else {
			filter = likeClause
		}
	}

	resp, err := c.doPost(ctx, "/v2/vectordb/entities/query", queryRequest{
		CollectionName: c.sessionsCol,
		Filter:         filter,
		OutputFields:   sessionOutputFields,
		Limit:          limit,
		Offset:         int(opts.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("search sessions: %w", err)
	}

	hits, err := decodeHits[meili.SessionHit](resp.Data)
	if err != nil {
		return nil, 0, fmt.Errorf("decode session hits: %w", err)
	}

	// Client-side sort (Milvus query API doesn't support ordering).
	sortBy := opts.SortBy
	if sortBy == "" {
		sortBy = "started_at"
	}
	sort.Slice(hits, func(i, j int) bool {
		switch sortBy {
		case "duration_s":
			return hits[i].DurationS > hits[j].DurationS
		case "total_cost_usd":
			return hits[i].TotalCostUSD > hits[j].TotalCostUSD
		case "total_events":
			return hits[i].TotalEvents > hits[j].TotalEvents
		default: // started_at
			return hits[i].StartedAt > hits[j].StartedAt
		}
	})

	total := c.countEntities(ctx, c.sessionsCol, filter)
	if total < 0 {
		total = int64(len(hits))
	}
	return hits, total, nil
}

func (c *MilvusClient) SearchPrompts(ctx context.Context, opts meili.PromptSearchOpts) ([]meili.PromptHit, int64, error) {
	filter := buildMilvusPromptFilter(opts)
	limit := int(clamp(opts.Limit, 50, 200))

	// If query is set, add a LIKE filter on prompt field.
	if opts.Query != "" {
		likeClause := fmt.Sprintf("prompt like \"%%%s%%\"", escapeMilvusLike(opts.Query))
		if filter != "" {
			filter = filter + " && " + likeClause
		} else {
			filter = likeClause
		}
	}

	resp, err := c.doPost(ctx, "/v2/vectordb/entities/query", queryRequest{
		CollectionName: c.promptsCol,
		Filter:         filter,
		OutputFields:   promptOutputFields,
		Limit:          limit,
		Offset:         int(opts.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("search prompts: %w", err)
	}

	hits, err := decodeHits[meili.PromptHit](resp.Data)
	if err != nil {
		return nil, 0, fmt.Errorf("decode prompt hits: %w", err)
	}

	// Sort by timestamp ascending.
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].TimestampUnix < hits[j].TimestampUnix
	})

	total := c.countEntities(ctx, c.promptsCol, filter)
	if total < 0 {
		total = int64(len(hits))
	}
	return hits, total, nil
}

func (c *MilvusClient) SearchEvents(ctx context.Context, opts meili.EventSearchOpts) ([]meili.EventHit, int64, error) {
	filter := buildMilvusEventFilter(opts)
	limit := int(clamp(opts.Limit, 20, 100))

	// If query is set, add LIKE filter on data_flat.
	if opts.Query != "" {
		likeClause := fmt.Sprintf("data_flat like \"%%%s%%\"", escapeMilvusLike(opts.Query))
		if filter != "" {
			filter = filter + " && " + likeClause
		} else {
			filter = likeClause
		}
	}

	resp, err := c.doPost(ctx, "/v2/vectordb/entities/query", queryRequest{
		CollectionName: c.eventsCol,
		Filter:         filter,
		OutputFields:   eventOutputFields,
		Limit:          limit,
		Offset:         int(opts.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("search events: %w", err)
	}

	hits, err := decodeHits[meili.EventHit](resp.Data)
	if err != nil {
		return nil, 0, fmt.Errorf("decode event hits: %w", err)
	}

	// Default sort: timestamp descending.
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].TimestampUnix > hits[j].TimestampUnix
	})

	total := c.countEntities(ctx, c.eventsCol, filter)
	if total < 0 {
		total = int64(len(hits))
	}
	return hits, total, nil
}

func (c *MilvusClient) FacetEvents(ctx context.Context, filter string, facets []string) (*meili.FacetResult, error) {
	// Milvus has no native faceting. Fetch up to 10000 events and aggregate client-side.
	// The facets parameter maps to Milvus outputFields (the fields to retrieve and group by).
	outputFields := facets
	resp, err := c.doPost(ctx, "/v2/vectordb/entities/query", queryRequest{
		CollectionName: c.eventsCol,
		Filter:         filter,
		OutputFields:   outputFields,
		Limit:          10000,
	})
	if err != nil {
		return nil, fmt.Errorf("facet events: %w", err)
	}

	dist := make(map[string]map[string]int)
	for _, field := range outputFields {
		dist[field] = make(map[string]int)
	}

	for _, raw := range resp.Data {
		var row map[string]interface{}
		if err := json.Unmarshal(raw, &row); err != nil {
			continue
		}
		for _, field := range outputFields {
			if val, ok := row[field]; ok {
				s := fmt.Sprintf("%v", val)
				if s != "" {
					dist[field][s]++
				}
			}
		}
	}

	totalResults := len(resp.Data)
	approx := totalResults == 10000
	if approx {
		fmt.Fprintf(os.Stderr, "hooks-mcp: WARNING: FacetEvents hit 10000 result limit; facet counts may be incomplete\n")
	}

	return &meili.FacetResult{
		TotalHits:    int64(totalResults),
		Approximate:  approx,
		Distribution: dist,
	}, nil
}

func (c *MilvusClient) ResolveSessionPrefix(ctx context.Context, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", fmt.Errorf("empty session ID")
	}
	if len(prefix) == 36 {
		return prefix, nil
	}

	filter := fmt.Sprintf("session_id like \"%s%%\"", escapeMilvusLike(prefix))
	resp, err := c.doPost(ctx, "/v2/vectordb/entities/query", queryRequest{
		CollectionName: c.sessionsCol,
		Filter:         filter,
		OutputFields:   []string{"session_id"},
		Limit:          10,
	})
	if err != nil {
		return "", fmt.Errorf("search for session prefix %q: %w", prefix, err)
	}

	var matches []string
	for _, raw := range resp.Data {
		var row struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			continue
		}
		if strings.HasPrefix(row.SessionID, prefix) {
			matches = append(matches, row.SessionID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session found matching prefix %q", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix %q matches %d sessions: %s",
			prefix, len(matches), strings.Join(matches[:min(3, len(matches))], ", "))
	}
}

// --- Semantic search methods ---

func (c *MilvusClient) SemanticSearchPrompts(ctx context.Context, queryText string, opts meili.PromptSearchOpts) ([]meili.PromptHit, error) {
	if c.embedder == nil {
		return nil, fmt.Errorf("embedding service not configured; set EMBED_SVC_URL to enable semantic search")
	}
	vec, err := c.embedder.Embed(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	filter := buildMilvusPromptFilter(opts)
	limit := int(clamp(opts.Limit, 20, 100))

	resp, err := c.doPost(ctx, "/v2/vectordb/entities/search", searchRequest{
		CollectionName: c.promptsCol,
		Data:           [][]float32{vec},
		AnnsField:      "prompt_dense",
		Limit:          limit,
		Filter:         filter,
		OutputFields:   promptOutputFields,
	})
	if err != nil {
		return nil, fmt.Errorf("semantic search prompts: %w", err)
	}

	return decodeHits[meili.PromptHit](resp.Data)
}

func (c *MilvusClient) SemanticSearchEvents(ctx context.Context, queryText string, opts meili.EventSearchOpts) ([]meili.EventHit, error) {
	if c.embedder == nil {
		return nil, fmt.Errorf("embedding service not configured; set EMBED_SVC_URL to enable semantic search")
	}
	vec, err := c.embedder.Embed(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	filter := buildMilvusEventFilter(opts)
	limit := int(clamp(opts.Limit, 20, 100))

	resp, err := c.doPost(ctx, "/v2/vectordb/entities/search", searchRequest{
		CollectionName: c.eventsCol,
		Data:           [][]float32{vec},
		AnnsField:      "dense_embedding",
		Limit:          limit,
		Filter:         filter,
		OutputFields:   eventOutputFields,
	})
	if err != nil {
		return nil, fmt.Errorf("semantic search events: %w", err)
	}

	return decodeHits[meili.EventHit](resp.Data)
}

func (c *MilvusClient) HybridSearch(ctx context.Context, queryText string, opts meili.EventSearchOpts) ([]meili.EventHit, error) {
	limit := int(clamp(opts.Limit, 20, 100))
	filter := buildMilvusEventFilter(opts)

	// If no embedder, fall back to sparse-only BM25 search.
	if c.embedder == nil {
		return c.FullTextSearchEvents(ctx, queryText, opts)
	}

	vec, err := c.embedder.Embed(ctx, queryText)
	if err != nil || vec == nil {
		// Embedding failed, fall back to sparse-only.
		return c.FullTextSearchEvents(ctx, queryText, opts)
	}

	// Native hybrid search: dense + sparse with server-side RRF.
	searchRequests := []map[string]interface{}{
		{
			"data":      [][]float32{vec},
			"annsField": "dense_embedding",
			"limit":     limit * 3,
		},
		{
			"data":      []map[string]string{{"data_flat": queryText}},
			"annsField": "sparse_embedding",
			"limit":     limit * 3,
		},
	}
	if filter != "" {
		for i := range searchRequests {
			searchRequests[i]["filter"] = filter
		}
	}

	rerank := map[string]interface{}{
		"strategy": "rrf",
		"params":   map[string]interface{}{"k": 60},
	}

	body := map[string]interface{}{
		"collectionName": c.eventsCol,
		"search":         searchRequests,
		"rerank":         rerank,
		"limit":          limit,
		"outputFields":   eventOutputFields,
	}

	resp, err := c.doPost(ctx, "/v2/vectordb/entities/hybrid_search", body)
	if err != nil {
		// Fall back to sparse-only on hybrid_search failure.
		return c.FullTextSearchEvents(ctx, queryText, opts)
	}

	return decodeHits[meili.EventHit](resp.Data)
}

func (c *MilvusClient) FullTextSearchEvents(ctx context.Context, query string, opts meili.EventSearchOpts) ([]meili.EventHit, error) {
	filter := buildMilvusEventFilter(opts)
	limit := int(clamp(opts.Limit, 20, 100))

	body := map[string]interface{}{
		"collectionName": c.eventsCol,
		"data":           []map[string]string{{"data_flat": query}},
		"annsField":      "sparse_embedding",
		"limit":          limit,
		"outputFields":   eventOutputFields,
	}
	if filter != "" {
		body["filter"] = filter
	}

	resp, err := c.doPost(ctx, "/v2/vectordb/entities/search", body)
	if err != nil {
		return nil, fmt.Errorf("full-text search events: %w", err)
	}
	return decodeHits[meili.EventHit](resp.Data)
}

func (c *MilvusClient) FullTextSearchPrompts(ctx context.Context, query string, opts meili.PromptSearchOpts) ([]meili.PromptHit, error) {
	filter := buildMilvusPromptFilter(opts)
	limit := int(clamp(opts.Limit, 20, 100))

	body := map[string]interface{}{
		"collectionName": c.promptsCol,
		"data":           []map[string]string{{"prompt": query}},
		"annsField":      "sparse_embedding",
		"limit":          limit,
		"outputFields":   promptOutputFields,
	}
	if filter != "" {
		body["filter"] = filter
	}

	resp, err := c.doPost(ctx, "/v2/vectordb/entities/search", body)
	if err != nil {
		return nil, fmt.Errorf("full-text search prompts: %w", err)
	}
	return decodeHits[meili.PromptHit](resp.Data)
}

// escapeMilvusLike escapes % and _ in user input for Milvus LIKE expressions.
func escapeMilvusLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
