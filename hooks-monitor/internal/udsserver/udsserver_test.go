package udsserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"claude-hooks-monitor/internal/hookevt"

	"hooks4claude/shared/filecache"
	"claude-hooks-monitor/internal/monitor"

	"hooks4claude/shared/uds"
)

func TestUDSServer_EventDispatch(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	eventCh := make(chan hookevt.HookEvent, 10)
	mon := monitor.NewHookMonitor(eventCh)

	srv, err := New(sock, mon, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(20 * time.Millisecond)

	// Send an event.
	conn, err := uds.Dial(sock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	evt := map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"session_id":      "test-session",
	}
	payload, _ := json.Marshal(evt)
	if err := uds.WriteMsg(conn, uds.MsgEvent, payload); err != nil {
		t.Fatal(err)
	}

	// Wait for event to be processed.
	select {
	case received := <-eventCh:
		if received.HookType != "PreToolUse" {
			t.Errorf("expected PreToolUse, got %s", received.HookType)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestUDSServer_CacheQueryRoundTrip(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	eventCh := make(chan hookevt.HookEvent, 10)
	mon := monitor.NewHookMonitor(eventCh)
	fc := filecache.New()
	fc.RecordRead("session-1", "/tmp/test.go", 12345, 100)

	srv, err := New(sock, mon, fc)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(20 * time.Millisecond)

	// Send cache query.
	conn, err := uds.Dial(sock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	query := map[string]string{
		"session_id": "session-1",
		"file_path":  "/tmp/test.go",
	}
	payload, _ := json.Marshal(query)
	if err := uds.WriteMsg(conn, uds.MsgCacheQuery, payload); err != nil {
		t.Fatal(err)
	}

	// Read response.
	conn.SetReadDeadline(time.Now().Add(time.Second))
	msgType, respPayload, err := uds.ReadMsg(conn)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != uds.MsgCacheResponse {
		t.Fatalf("expected MsgCacheResponse, got %d", msgType)
	}

	var result filecache.CacheQuery
	if err := json.Unmarshal(respPayload, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Found {
		t.Error("expected cache hit, got miss")
	}
	if result.MtimeNS != 12345 {
		t.Errorf("expected mtime_ns 12345, got %d", result.MtimeNS)
	}
}

func TestUDSServer_CacheQueryNoCache(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	mon := monitor.NewHookMonitor(nil)

	// No file cache configured.
	srv, err := New(sock, mon, nil)
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

	query, _ := json.Marshal(map[string]string{"session_id": "x", "file_path": "/y"})
	uds.WriteMsg(conn, uds.MsgCacheQuery, query)

	conn.SetReadDeadline(time.Now().Add(time.Second))
	msgType, respPayload, err := uds.ReadMsg(conn)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != uds.MsgCacheResponse {
		t.Fatalf("expected MsgCacheResponse, got %d", msgType)
	}

	var result filecache.CacheQuery
	json.Unmarshal(respPayload, &result)
	if result.Found {
		t.Error("expected cache miss when no cache configured")
	}
}

func TestUDSServer_FallbackHookType(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	eventCh := make(chan hookevt.HookEvent, 10)
	mon := monitor.NewHookMonitor(eventCh)

	srv, err := New(sock, mon, nil)
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

	// Use hook_type instead of hook_event_name.
	evt := map[string]interface{}{
		"hook_type": "SessionStart",
	}
	payload, _ := json.Marshal(evt)
	uds.WriteMsg(conn, uds.MsgEvent, payload)

	select {
	case received := <-eventCh:
		if received.HookType != "SessionStart" {
			t.Errorf("expected SessionStart, got %s", received.HookType)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}
