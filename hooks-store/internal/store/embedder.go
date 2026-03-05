package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the embedder circuit breaker is open.
// Callers should use zero vectors and set dense_valid=false.
var ErrCircuitOpen = fmt.Errorf("embedder: circuit breaker open")

// Embedder is an HTTP client for an embedding service.
// Includes a circuit breaker: after 3 consecutive failures, skips calls for 30s.
type Embedder struct {
	baseURL      string
	client       *http.Client
	failCount    int64
	lastFailTime time.Time
	mu           sync.Mutex
	circuitOpen  bool
}

// NewEmbedder creates a new Embedder pointing at the given base URL.
func NewEmbedder(baseURL string) *Embedder {
	return &Embedder{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// isCircuitOpen returns true if the circuit breaker is open (too many failures).
func (e *Embedder) isCircuitOpen() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.circuitOpen {
		return false
	}
	if time.Since(e.lastFailTime) > 30*time.Second {
		e.circuitOpen = false
		e.failCount = 0
		fmt.Fprintf(os.Stderr, "embedder: circuit breaker closed, retrying\n")
		return false
	}
	return true
}

func (e *Embedder) recordSuccess() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failCount = 0
	e.circuitOpen = false
}

func (e *Embedder) recordFailure() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failCount++
	e.lastFailTime = time.Now()
	if e.failCount >= 3 && !e.circuitOpen {
		e.circuitOpen = true
		fmt.Fprintf(os.Stderr, "embedder: circuit breaker OPEN after %d failures, skipping for 30s\n", e.failCount)
	}
}

// Embed sends texts to the embedding service and returns vectors.
// Returns ErrCircuitOpen if the circuit breaker is open.
// Returns nil vectors (not error) on transient failures — caller should use zero vectors.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.isCircuitOpen() {
		return nil, ErrCircuitOpen
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"texts": texts,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/embed", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		e.recordFailure()
		fmt.Fprintf(os.Stderr, "embedder: request failed (failures=%d): %v\n", e.failCount, err)
		return nil, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e.recordFailure()
		fmt.Fprintf(os.Stderr, "embedder: read response failed (failures=%d): %v\n", e.failCount, err)
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		e.recordFailure()
		fmt.Fprintf(os.Stderr, "embedder: HTTP %d (failures=%d): %s\n", resp.StatusCode, e.failCount, truncateLog(string(body), 200))
		return nil, nil
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		e.recordFailure()
		fmt.Fprintf(os.Stderr, "embedder: decode response failed (failures=%d): %v\n", e.failCount, err)
		return nil, nil
	}

	e.recordSuccess()
	return result.Embeddings, nil
}

func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
