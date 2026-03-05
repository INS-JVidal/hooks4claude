package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// MilvusClient is a thin HTTP client for the Milvus REST API v2.
type MilvusClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewMilvusClient creates a new Milvus REST API client.
func NewMilvusClient(baseURL, token string) *MilvusClient {
	return &MilvusClient{
		baseURL: baseURL,
		token:   token,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest performs an HTTP request with exponential backoff retry.
// Max 3 retries, delays 1s→2s→4s, max 30s total.
func (c *MilvusClient) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("milvus %s %s: status %d: %s", method, path, resp.StatusCode, string(respBody))
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("milvus %s %s: status %d: %s", method, path, resp.StatusCode, string(respBody))
		}

		return respBody, nil
	}
	return nil, fmt.Errorf("milvus %s %s failed after 3 retries: %w", method, path, lastErr)
}

// milvusResponse is the common response wrapper for Milvus REST API v2.
type milvusResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (c *MilvusClient) checkResponse(respBody []byte) error {
	var resp milvusResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	if resp.Code != 0 {
		return fmt.Errorf("milvus error code %d: %s", resp.Code, resp.Message)
	}
	return nil
}

// HasCollection checks if a collection exists.
func (c *MilvusClient) HasCollection(ctx context.Context, name string) (bool, error) {
	body := map[string]interface{}{
		"collectionName": name,
	}
	respBody, err := c.doRequest(ctx, "POST", "/v2/vectordb/collections/has", body)
	if err != nil {
		return false, err
	}
	var resp milvusResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return false, err
	}
	if resp.Code != 0 {
		return false, fmt.Errorf("milvus error: %s", resp.Message)
	}
	var has struct {
		Has bool `json:"has"`
	}
	if err := json.Unmarshal(resp.Data, &has); err != nil {
		return false, err
	}
	return has.Has, nil
}

// DropCollection drops a collection.
func (c *MilvusClient) DropCollection(ctx context.Context, name string) error {
	body := map[string]interface{}{
		"collectionName": name,
	}
	respBody, err := c.doRequest(ctx, "POST", "/v2/vectordb/collections/drop", body)
	if err != nil {
		return err
	}
	return c.checkResponse(respBody)
}

// SearchText performs a BM25 full-text search on a sparse vector field.
func (c *MilvusClient) SearchText(ctx context.Context, collectionName string, textField, annsField, query string, limit int, filter string, outputFields []string) ([]map[string]interface{}, error) {
	body := map[string]interface{}{
		"collectionName": collectionName,
		"data":           []map[string]string{{textField: query}},
		"annsField":      annsField,
		"limit":          limit,
		"outputFields":   outputFields,
	}
	if filter != "" {
		body["filter"] = filter
	}
	respBody, err := c.doRequest(ctx, "POST", "/v2/vectordb/entities/search", body)
	if err != nil {
		return nil, err
	}
	var resp milvusResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("milvus search error: %s", resp.Message)
	}
	var results []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// CreateCollection creates a collection with the given schema.
func (c *MilvusClient) CreateCollection(ctx context.Context, name string, schema interface{}) error {
	respBody, err := c.doRequest(ctx, "POST", "/v2/vectordb/collections/create", schema)
	if err != nil {
		return err
	}
	return c.checkResponse(respBody)
}

// Insert inserts data into a collection.
func (c *MilvusClient) Insert(ctx context.Context, collectionName string, data []map[string]interface{}) error {
	body := map[string]interface{}{
		"collectionName": collectionName,
		"data":           data,
	}
	respBody, err := c.doRequest(ctx, "POST", "/v2/vectordb/entities/insert", body)
	if err != nil {
		return err
	}
	return c.checkResponse(respBody)
}

// Upsert upserts data into a collection.
func (c *MilvusClient) Upsert(ctx context.Context, collectionName string, data []map[string]interface{}) error {
	body := map[string]interface{}{
		"collectionName": collectionName,
		"data":           data,
	}
	respBody, err := c.doRequest(ctx, "POST", "/v2/vectordb/entities/upsert", body)
	if err != nil {
		return err
	}
	return c.checkResponse(respBody)
}

// Query queries entities by filter expression.
func (c *MilvusClient) Query(ctx context.Context, collectionName, filter string, outputFields []string, limit, offset int) ([]map[string]interface{}, error) {
	body := map[string]interface{}{
		"collectionName": collectionName,
		"filter":         filter,
		"outputFields":   outputFields,
		"limit":          limit,
		"offset":         offset,
	}
	respBody, err := c.doRequest(ctx, "POST", "/v2/vectordb/entities/query", body)
	if err != nil {
		return nil, err
	}
	var resp milvusResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("milvus query error: %s", resp.Message)
	}
	var results []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// Search performs a vector similarity search.
func (c *MilvusClient) Search(ctx context.Context, collectionName string, vector []float32, annsField string, limit int, filter string, outputFields []string) ([]map[string]interface{}, error) {
	body := map[string]interface{}{
		"collectionName": collectionName,
		"data":           [][]float32{vector},
		"annsField":      annsField,
		"limit":          limit,
		"outputFields":   outputFields,
	}
	if filter != "" {
		body["filter"] = filter
	}
	respBody, err := c.doRequest(ctx, "POST", "/v2/vectordb/entities/search", body)
	if err != nil {
		return nil, err
	}
	var resp milvusResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("milvus search error: %s", resp.Message)
	}
	var results []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// HybridSearch performs a hybrid search with multiple search requests and reranking.
func (c *MilvusClient) HybridSearch(ctx context.Context, collectionName string, searchRequests []map[string]interface{}, rerank map[string]interface{}, limit int, outputFields []string) ([]map[string]interface{}, error) {
	body := map[string]interface{}{
		"collectionName": collectionName,
		"search":         searchRequests,
		"rerank":         rerank,
		"limit":          limit,
		"outputFields":   outputFields,
	}
	respBody, err := c.doRequest(ctx, "POST", "/v2/vectordb/entities/hybrid_search", body)
	if err != nil {
		return nil, err
	}
	var resp milvusResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("milvus hybrid_search error: %s", resp.Message)
	}
	var results []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, err
	}
	return results, nil
}
