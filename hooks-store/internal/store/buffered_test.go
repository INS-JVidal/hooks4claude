package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type failStore struct {
	mu       sync.Mutex
	docs     []Document
	failNext bool
}

func (s *failStore) Index(_ context.Context, doc Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failNext {
		return fmt.Errorf("milvus down")
	}
	s.docs = append(s.docs, doc)
	return nil
}

func (s *failStore) Close() error { return nil }

func (s *failStore) setFail(fail bool) {
	s.mu.Lock()
	s.failNext = fail
	s.mu.Unlock()
}

func (s *failStore) docCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.docs)
}

func TestBufferedStore_PassThrough(t *testing.T) {
	t.Parallel()
	inner := &failStore{}
	bs := NewBufferedStore(inner, 10, 0)
	defer bs.Close()

	doc := Document{ID: "a", HookType: "Test"}
	if err := bs.Index(context.Background(), doc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.docCount() != 1 {
		t.Fatalf("expected 1 doc, got %d", inner.docCount())
	}
	if bs.Buffered() != 0 {
		t.Fatalf("expected 0 buffered, got %d", bs.Buffered())
	}
}

func TestBufferedStore_BuffersOnFailure(t *testing.T) {
	t.Parallel()
	inner := &failStore{failNext: true}
	bs := NewBufferedStore(inner, 10, 0)
	defer bs.Close()

	doc := Document{ID: "a", HookType: "Test"}
	if err := bs.Index(context.Background(), doc); err != nil {
		t.Fatalf("buffered store should not return error: %v", err)
	}
	if inner.docCount() != 0 {
		t.Fatalf("expected 0 docs in inner store, got %d", inner.docCount())
	}
	if bs.Buffered() != 1 {
		t.Fatalf("expected 1 buffered, got %d", bs.Buffered())
	}
}

func TestBufferedStore_FlushOnRecovery(t *testing.T) {
	t.Parallel()
	inner := &failStore{failNext: true}
	// Use small capacity, don't rely on background flush (too slow for test).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bs := &BufferedStore{
		inner: inner,
		buf:   make([]Document, 10),
		ctx:   ctx,
	}

	// Buffer 3 docs while "down".
	for i := 0; i < 3; i++ {
		bs.Index(context.Background(), Document{ID: fmt.Sprintf("%d", i), HookType: "Test"})
	}
	if bs.Buffered() != 3 {
		t.Fatalf("expected 3 buffered, got %d", bs.Buffered())
	}

	// "Recover" and flush manually.
	inner.setFail(false)
	bs.flush()

	if inner.docCount() != 3 {
		t.Fatalf("expected 3 docs flushed, got %d", inner.docCount())
	}
	if bs.Buffered() != 0 {
		t.Fatalf("expected 0 buffered after flush, got %d", bs.Buffered())
	}
}

func TestBufferedStore_RingOverflow(t *testing.T) {
	t.Parallel()
	inner := &failStore{failNext: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bs := &BufferedStore{
		inner: inner,
		buf:   make([]Document, 3), // capacity 3
		ctx:   ctx,
	}

	// Buffer 5 docs — first 2 should be dropped.
	for i := 0; i < 5; i++ {
		bs.Index(context.Background(), Document{ID: fmt.Sprintf("%d", i), HookType: "Test"})
	}

	if bs.Buffered() != 3 {
		t.Fatalf("expected 3 buffered (cap), got %d", bs.Buffered())
	}
	if bs.Dropped() != 2 {
		t.Fatalf("expected 2 dropped, got %d", bs.Dropped())
	}

	// Flush — should get the 3 newest docs.
	inner.setFail(false)
	bs.flush()

	if inner.docCount() != 3 {
		t.Fatalf("expected 3 docs flushed, got %d", inner.docCount())
	}
}

func TestBufferedStore_ConcurrentIndex(t *testing.T) {
	t.Parallel()
	inner := &failStore{failNext: true}
	bs := NewBufferedStore(inner, 100, 0)
	defer bs.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			bs.Index(context.Background(), Document{ID: fmt.Sprintf("%d", id)})
		}(i)
	}
	wg.Wait()

	if bs.Buffered() != 50 {
		t.Fatalf("expected 50 buffered, got %d", bs.Buffered())
	}
}

func TestBufferedStore_BackgroundFlush(t *testing.T) {
	t.Parallel()
	inner := &failStore{failNext: true}
	// Create with real background goroutine but override flush interval isn't easy,
	// so we test via manual flush after recovery.
	bs := NewBufferedStore(inner, 100, 0)
	defer bs.Close()

	bs.Index(context.Background(), Document{ID: "x", HookType: "Test"})
	if bs.Buffered() != 1 {
		t.Fatalf("expected 1 buffered, got %d", bs.Buffered())
	}

	inner.setFail(false)

	// Wait for background flush (up to 10s, flush interval is 5s).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if bs.Buffered() == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if bs.Buffered() != 0 {
		t.Fatalf("expected background flush to drain buffer, still %d buffered", bs.Buffered())
	}
	if inner.docCount() != 1 {
		t.Fatalf("expected 1 doc flushed, got %d", inner.docCount())
	}
}
