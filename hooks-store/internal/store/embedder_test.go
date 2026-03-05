package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestEmbedder_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"embeddings": [][]float32{{0.1, 0.2, 0.3}},
		})
	}))
	defer srv.Close()

	e := NewEmbedder(srv.URL)
	vecs, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 3 {
		t.Fatalf("expected 1 vector of dim 3, got %d vectors", len(vecs))
	}
}

func TestEmbedder_CircuitBreaker_OpensAfter3Failures(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := NewEmbedder(srv.URL)

	// 3 failures should open the circuit.
	for i := 0; i < 3; i++ {
		vecs, err := e.Embed(context.Background(), []string{"test"})
		if err != nil {
			t.Fatalf("embed should return nil,nil on failure, got error: %v", err)
		}
		if vecs != nil {
			t.Fatal("expected nil vecs on failure")
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}

	// Circuit is open — 4th call should NOT hit the server and return ErrCircuitOpen.
	vecs, err := e.Embed(context.Background(), []string{"test"})
	if vecs != nil {
		t.Fatal("expected nil vecs when circuit open")
	}
	if err != ErrCircuitOpen {
		t.Fatalf("expected ErrCircuitOpen, got: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected circuit to skip call, but server got %d calls", calls.Load())
	}
}

func TestEmbedder_CircuitBreaker_RecoveryAfterTimeout(t *testing.T) {
	t.Parallel()
	var shouldFail atomic.Bool
	shouldFail.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldFail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"embeddings": [][]float32{{1.0, 2.0}},
		})
	}))
	defer srv.Close()

	e := NewEmbedder(srv.URL)

	// Trip the circuit.
	for i := 0; i < 3; i++ {
		e.Embed(context.Background(), []string{"test"})
	}
	if !e.isCircuitOpen() {
		t.Fatal("circuit should be open")
	}

	// Simulate time passing: manually reset lastFailTime.
	e.mu.Lock()
	e.lastFailTime = time.Now().Add(-31 * time.Second)
	e.mu.Unlock()

	// Fix the server.
	shouldFail.Store(false)

	// Should try again and succeed.
	vecs, err := e.Embed(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vecs == nil {
		t.Fatal("expected successful embedding after recovery")
	}
	if e.isCircuitOpen() {
		t.Fatal("circuit should be closed after success")
	}
}

func TestEmbedder_SuccessResetsFailCount(t *testing.T) {
	t.Parallel()
	var callCount atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		// Fail on calls 1 and 2, succeed on 3+.
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"embeddings": [][]float32{{0.5}},
		})
	}))
	defer srv.Close()

	e := NewEmbedder(srv.URL)

	// 2 failures — circuit should NOT be open yet.
	e.Embed(context.Background(), []string{"a"})
	e.Embed(context.Background(), []string{"b"})
	if e.isCircuitOpen() {
		t.Fatal("circuit should not be open after 2 failures")
	}

	// Success resets fail count.
	vecs, _ := e.Embed(context.Background(), []string{"c"})
	if vecs == nil {
		t.Fatal("expected success on 3rd call")
	}

	e.mu.Lock()
	fc := e.failCount
	e.mu.Unlock()
	if fc != 0 {
		t.Fatalf("expected failCount=0 after success, got %d", fc)
	}
}

func TestEmbedder_BadJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	e := NewEmbedder(srv.URL)
	vecs, err := e.Embed(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("expected nil error on bad json, got: %v", err)
	}
	if vecs != nil {
		t.Fatal("expected nil vecs on bad json")
	}
}
