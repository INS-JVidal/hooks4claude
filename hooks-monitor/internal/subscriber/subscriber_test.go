package subscriber

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"claude-hooks-monitor/internal/hookevt"
	"claude-hooks-monitor/internal/monitor"

	"hooks4claude/shared/uds"
)

func TestConnectAndReceive(t *testing.T) {
	sock := t.TempDir() + "/pub.sock"

	// Start a minimal pub server.
	ln, err := uds.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	eventCh := make(chan hookevt.HookEvent, 16)
	mon := monitor.NewHookMonitor(eventCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Accept one subscriber, send one event, then close.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read subscribe handshake.
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		msgType, _, err := uds.ReadMsg(conn)
		if err != nil || msgType != uds.MsgSubscribe {
			return
		}

		evt := hookevt.HookEvent{
			HookType:  "PreToolUse",
			Timestamp: time.Now(),
			Data:      map[string]interface{}{"tool_name": "Read"},
		}
		payload, _ := json.Marshal(evt)
		uds.WriteMsg(conn, uds.MsgEvent, payload)

		// Keep conn open briefly so subscriber can read.
		time.Sleep(200 * time.Millisecond)
	}()

	sub := New(sock)
	go sub.Run(ctx, mon)

	select {
	case evt := <-eventCh:
		if evt.HookType != "PreToolUse" {
			t.Errorf("got hook type %q, want PreToolUse", evt.HookType)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestContextCancellation(t *testing.T) {
	sock := t.TempDir() + "/pub.sock"

	// No server — subscriber should respect context cancellation during backoff.
	mon := monitor.NewHookMonitor(nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- New(sock).Run(ctx, mon)
	}()

	// Cancel quickly.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
