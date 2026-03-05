package ingest_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hooks-store/internal/ingest"
	"hooks-store/internal/store"

	"hooks4claude/shared/uds"
)

// mockStoreUDS implements store.EventStore for UDS tests.
type mockStoreUDS struct {
	mu   sync.Mutex
	docs []store.Document
}

func (m *mockStoreUDS) Index(_ context.Context, doc store.Document) error {
	m.mu.Lock()
	m.docs = append(m.docs, doc)
	m.mu.Unlock()
	return nil
}

func (m *mockStoreUDS) Close() error { return nil }

func TestUDS_RoundTrip(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	ms := &mockStoreUDS{}

	srv, err := ingest.NewUDS(sock, ms, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	var ingested atomic.Int64
	srv.SetOnIngest(func(evt ingest.IngestEvent, _ []byte) {
		ingested.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)

	// Give server time to start.
	time.Sleep(20 * time.Millisecond)

	// Send a framed event.
	conn, err := uds.Dial(sock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	evt := map[string]interface{}{
		"hook_type":  "PreToolUse",
		"timestamp":  time.Now().Format(time.RFC3339),
		"data":       map[string]interface{}{"tool_name": "Read"},
	}
	payload, _ := json.Marshal(evt)
	if err := uds.WriteMsg(conn, uds.MsgEvent, payload); err != nil {
		t.Fatal(err)
	}

	// Wait for processing.
	time.Sleep(50 * time.Millisecond)

	ms.mu.Lock()
	count := len(ms.docs)
	ms.mu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 document, got %d", count)
	}
	if ingested.Load() != 1 {
		t.Fatalf("expected 1 ingest callback, got %d", ingested.Load())
	}
}

func TestUDS_AllHookTypes(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	ms := &mockStoreUDS{}

	srv, err := ingest.NewUDS(sock, ms, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(20 * time.Millisecond)

	hookTypes := []string{
		"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse",
		"PostToolUseFailure", "PermissionRequest", "Notification",
		"SubagentStart", "SubagentStop", "Stop", "TeammateIdle",
		"TaskCompleted", "ConfigChange", "PreCompact", "SessionEnd",
	}

	for _, ht := range hookTypes {
		conn, err := uds.Dial(sock, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		evt := map[string]interface{}{
			"hook_type": ht,
			"timestamp": time.Now().Format(time.RFC3339),
			"data":      map[string]interface{}{},
		}
		payload, _ := json.Marshal(evt)
		uds.WriteMsg(conn, uds.MsgEvent, payload)
		conn.Close()
	}

	time.Sleep(100 * time.Millisecond)

	ms.mu.Lock()
	count := len(ms.docs)
	ms.mu.Unlock()

	if count != len(hookTypes) {
		t.Fatalf("expected %d documents, got %d", len(hookTypes), count)
	}
}

func TestUDS_ConcurrentBurst(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	ms := &mockStoreUDS{}

	srv, err := ingest.NewUDS(sock, ms, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(20 * time.Millisecond)

	const numWriters = 20
	var wg sync.WaitGroup
	wg.Add(numWriters)

	for i := 0; i < numWriters; i++ {
		go func() {
			defer wg.Done()
			conn, err := uds.Dial(sock, time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			evt := map[string]interface{}{
				"hook_type": "PreToolUse",
				"timestamp": time.Now().Format(time.RFC3339),
				"data":      map[string]interface{}{},
			}
			payload, _ := json.Marshal(evt)
			uds.WriteMsg(conn, uds.MsgEvent, payload)
		}()
	}
	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	ms.mu.Lock()
	count := len(ms.docs)
	ms.mu.Unlock()

	if count != numWriters {
		t.Fatalf("expected %d documents, got %d", numWriters, count)
	}
}

func TestUDS_EmptyPayload(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	ms := &mockStoreUDS{}

	srv, err := ingest.NewUDS(sock, ms, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(20 * time.Millisecond)

	conn, err := uds.Dial(sock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send empty payload — should be rejected.
	uds.WriteMsg(conn, uds.MsgEvent, nil)
	time.Sleep(50 * time.Millisecond)

	ms.mu.Lock()
	count := len(ms.docs)
	ms.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 documents for empty payload, got %d", count)
	}
}

func TestUDS_MissingHookType(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	ms := &mockStoreUDS{}

	srv, err := ingest.NewUDS(sock, ms, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(20 * time.Millisecond)

	conn, err := uds.Dial(sock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Missing hook_type.
	payload, _ := json.Marshal(map[string]interface{}{"data": map[string]interface{}{}})
	uds.WriteMsg(conn, uds.MsgEvent, payload)
	time.Sleep(50 * time.Millisecond)

	ms.mu.Lock()
	count := len(ms.docs)
	ms.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 documents for missing hook_type, got %d", count)
	}
}
