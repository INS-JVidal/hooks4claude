package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	defaultBufferCap      = 1000
	DefaultFlushInterval  = 5 * time.Second
	flushBatchSize        = 50
)

// BufferedStore wraps an EventStore with a ring buffer that captures
// documents when the underlying store is unavailable. A background
// goroutine periodically flushes buffered documents when the store
// recovers.
//
// Safe for concurrent use.
type BufferedStore struct {
	inner EventStore

	mu      sync.Mutex
	buf     []Document
	head    int  // next write position (ring)
	count   int  // number of buffered items (<=cap)
	dropped int64

	flushInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBufferedStore creates a BufferedStore with the given capacity.
// Pass 0 for flushEvery to use the default (5 s).
func NewBufferedStore(inner EventStore, capacity int, flushEvery time.Duration) *BufferedStore {
	if capacity <= 0 {
		capacity = defaultBufferCap
	}
	if flushEvery <= 0 {
		flushEvery = DefaultFlushInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	bs := &BufferedStore{
		inner:         inner,
		buf:           make([]Document, capacity),
		flushInterval: flushEvery,
		ctx:           ctx,
		cancel:        cancel,
	}
	bs.wg.Add(1)
	go bs.flushLoop()
	return bs
}

// Index tries the inner store first. On failure, buffers the document.
func (bs *BufferedStore) Index(ctx context.Context, doc Document) error {
	err := bs.inner.Index(ctx, doc)
	if err == nil {
		return nil
	}

	// Inner store failed — buffer the document.
	bs.mu.Lock()
	bufCap := len(bs.buf)
	wasEmpty := bs.count == 0
	if bs.count == bufCap {
		// Ring full — overwrite oldest, count the drop.
		bs.dropped++
	} else {
		bs.count++
	}
	bs.buf[bs.head] = doc
	bs.head = (bs.head + 1) % bufCap
	bs.mu.Unlock()

	if wasEmpty {
		fmt.Fprintf(os.Stderr, "buffer: milvus unavailable, buffering events (cap=%d): %v\n", bufCap, err)
	}
	fmt.Fprintf(os.Stderr, "buffer: document buffered (id=%s, buffered=%d)\n", doc.ID, bs.Buffered())

	return nil // caller sees success — document is buffered
}

// Close flushes remaining buffered documents (best-effort) and stops
// the background goroutine.
func (bs *BufferedStore) Close() error {
	bs.cancel()
	bs.wg.Wait()

	// Final flush attempt with a fresh context (bs.ctx is already cancelled).
	bs.finalFlush()

	return bs.inner.Close()
}

// finalFlush is like flush but uses a fresh context (for use after bs.ctx is cancelled).
func (bs *BufferedStore) finalFlush() {
	for {
		docs := bs.drain(flushBatchSize)
		if len(docs) == 0 {
			return
		}
		for i, doc := range docs {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := bs.inner.Index(ctx, doc); err != nil {
				cancel()
				for j := i; j < len(docs); j++ {
					bs.requeue(docs[j])
				}
				fmt.Fprintf(os.Stderr, "buffer final flush: failed (%d remaining): %v\n", bs.Buffered(), err)
				return
			}
			cancel()
		}
		fmt.Fprintf(os.Stderr, "buffer final flush: flushed %d documents (%d remaining)\n", len(docs), bs.Buffered())
	}
}

// Buffered returns the current number of buffered documents.
func (bs *BufferedStore) Buffered() int {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.count
}

// Dropped returns the total number of documents dropped due to a full buffer.
func (bs *BufferedStore) Dropped() int64 {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.dropped
}

func (bs *BufferedStore) flushLoop() {
	defer bs.wg.Done()
	ticker := time.NewTicker(bs.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-bs.ctx.Done():
			return
		case <-ticker.C:
			bs.flush()
		}
	}
}

func (bs *BufferedStore) flush() {
	for {
		docs := bs.drain(flushBatchSize)
		if len(docs) == 0 {
			return
		}

		flushed := 0
		for i, doc := range docs {
			ctx, cancel := context.WithTimeout(bs.ctx, 30*time.Second)
			if err := bs.inner.Index(ctx, doc); err != nil {
				cancel()
				// Re-buffer the failed doc and all remaining docs in the batch.
				for j := i; j < len(docs); j++ {
					bs.requeue(docs[j])
				}
				fmt.Fprintf(os.Stderr, "buffer flush: milvus still down (%d buffered): %v\n", bs.Buffered(), err)
				return
			}
			cancel()
			flushed++
		}

		fmt.Fprintf(os.Stderr, "buffer flush: flushed %d documents (%d remaining)\n", flushed, bs.Buffered())
	}
}

// drain removes up to n documents from the buffer (FIFO order).
func (bs *BufferedStore) drain(n int) []Document {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.count == 0 {
		return nil
	}
	if n > bs.count {
		n = bs.count
	}

	bufCap := len(bs.buf)
	// tail is the oldest item.
	tail := (bs.head - bs.count + bufCap) % bufCap

	docs := make([]Document, n)
	for i := 0; i < n; i++ {
		docs[i] = bs.buf[(tail+i)%bufCap]
	}
	bs.count -= n
	return docs
}

// requeue puts a document back at the front of the buffer (for retry).
func (bs *BufferedStore) requeue(doc Document) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bufCap := len(bs.buf)
	if bs.count == bufCap {
		bs.dropped++
		return
	}
	// Insert at the tail (oldest position) so it's flushed first next time.
	tail := (bs.head - bs.count - 1 + bufCap) % bufCap
	bs.buf[tail] = doc
	bs.count++
}
