package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hooks4claude/shared/uds"
)

// fakeMonitor is a UDS server that counts received events.
type fakeMonitor struct {
	received atomic.Int64
}

func startFakeMonitor(t *testing.T, sock string) *fakeMonitor {
	t.Helper()
	fm := &fakeMonitor{}
	ln, err := uds.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				for {
					_, _, err := uds.ReadMsg(conn)
					if err != nil {
						return
					}
					fm.received.Add(1)
				}
			}()
		}
	}()
	return fm
}

func TestDaemon_ConcurrentBurst(t *testing.T) {
	dir := t.TempDir()
	daemonSock := filepath.Join(dir, "daemon.sock")
	monitorSock := filepath.Join(dir, "monitor.sock")

	// Start a fake monitor that counts events.
	fm := startFakeMonitor(t, monitorSock)

	// Set env so daemon discovers the monitor socket.
	t.Setenv("HOOK_MONITOR_SOCK", monitorSock)
	t.Setenv("HOOK_MONITOR_CONFIG", filepath.Join(dir, "nonexistent.conf")) // fail-open

	// Start daemon.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln, err := uds.Listen(daemonSock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ds := newConnPool(monitorSock, 8)
	defer ds.close()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleDaemonConn(ctx, conn, ds)
		}
	}()

	time.Sleep(20 * time.Millisecond) // let daemon start

	// Fire 30 simultaneous shim-like writers.
	const numWriters = 30
	var wg sync.WaitGroup
	wg.Add(numWriters)

	for i := 0; i < numWriters; i++ {
		go func() {
			defer wg.Done()
			conn, err := uds.Dial(daemonSock, time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			evt := map[string]interface{}{
				"hook_event_name": "PostToolUse",
				"tool_name":       "Bash",
				"session_id":      "test-session",
				"cwd":             dir,
			}
			payload, _ := json.Marshal(evt)
			if err := uds.WriteMsg(conn, uds.MsgEvent, payload); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	// Give the daemon time to process and forward.
	time.Sleep(200 * time.Millisecond)

	got := fm.received.Load()
	if got != numWriters {
		t.Errorf("fake monitor received %d events, want %d", got, numWriters)
	}
}

func TestDaemon_ConcurrentCacheQueries(t *testing.T) {
	dir := t.TempDir()
	daemonSock := filepath.Join(dir, "daemon.sock")
	monitorSock := filepath.Join(dir, "monitor.sock")

	// Fake monitor that answers cache queries with "not found" and counts events.
	fm := &fakeMonitor{}
	ln, err := uds.Listen(monitorSock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				for {
					msgType, _, err := uds.ReadMsg(conn)
					if err != nil {
						return
					}
					if msgType == uds.MsgCacheQuery {
						uds.WriteMsg(conn, uds.MsgCacheResponse, []byte(`{"found":false}`))
					}
					fm.received.Add(1)
				}
			}()
		}
	}()

	t.Setenv("HOOK_MONITOR_SOCK", monitorSock)
	t.Setenv("HOOK_MONITOR_CONFIG", filepath.Join(dir, "nonexistent.conf"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dln, err := uds.Listen(daemonSock)
	if err != nil {
		t.Fatal(err)
	}
	defer dln.Close()

	ds := newConnPool(monitorSock, 8)
	defer ds.close()

	go func() {
		<-ctx.Done()
		dln.Close()
	}()
	go func() {
		for {
			conn, err := dln.Accept()
			if err != nil {
				return
			}
			go handleDaemonConn(ctx, conn, ds)
		}
	}()

	time.Sleep(20 * time.Millisecond)

	// Fire 20 simultaneous PreToolUse Read cache queries.
	const numWriters = 20
	var wg sync.WaitGroup
	wg.Add(numWriters)
	var responses atomic.Int64

	for i := 0; i < numWriters; i++ {
		go func() {
			defer wg.Done()
			conn, err := uds.Dial(daemonSock, time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			evt := map[string]interface{}{
				"hook_event_name": "PreToolUse",
				"tool_name":       "Read",
				"session_id":      "test-session",
				"cwd":             dir,
				"tool_input":      map[string]interface{}{"file_path": "/tmp/nonexistent.go"},
			}
			payload, _ := json.Marshal(evt)
			if err := uds.WriteMsg(conn, uds.MsgCacheQuery, payload); err != nil {
				t.Error(err)
				return
			}
			// Read response.
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			msgType, _, err := uds.ReadMsg(conn)
			if err != nil {
				t.Error(err)
				return
			}
			if msgType == uds.MsgCacheResponse {
				responses.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := responses.Load(); got != int64(numWriters) {
		t.Errorf("got %d cache responses, want %d", got, numWriters)
	}
}

func TestConnPool_ExceedsPoolSize(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "test.sock")

	// Simple echo server.
	ln, err := uds.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				for {
					if _, _, err := uds.ReadMsg(conn); err != nil {
						return
					}
				}
			}()
		}
	}()

	// Pool of size 2, but 10 concurrent senders.
	pool := newConnPool(sock, 2)
	defer pool.close()

	time.Sleep(10 * time.Millisecond)

	const numWriters = 10
	var wg sync.WaitGroup
	wg.Add(numWriters)
	var success atomic.Int64

	for i := 0; i < numWriters; i++ {
		go func() {
			defer wg.Done()
			pool.send(uds.MsgEvent, []byte(`{"test":true}`))
			success.Add(1)
		}()
	}
	wg.Wait()

	if got := success.Load(); got != numWriters {
		t.Errorf("expected %d sends to succeed, got %d", numWriters, got)
	}
}

func TestConnPool_DownstreamDown(t *testing.T) {
	// Pool targeting a nonexistent socket — send should not panic or block.
	pool := newConnPool("/tmp/nonexistent-daemon-test.sock", 4)
	defer pool.close()

	// Should return silently (no panic, no long block).
	done := make(chan struct{})
	go func() {
		pool.send(uds.MsgEvent, []byte(`{"test":true}`))
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("send to nonexistent socket blocked too long")
	}
}
