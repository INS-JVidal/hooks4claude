package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"

	"hooks4claude/shared/uds"
)

func tempSocket(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/pub.sock"
}

func TestSubscribeAndBroadcast(t *testing.T) {
	sock := tempSocket(t)
	ps, err := New(sock)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer ps.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ps.Serve(ctx)

	// Subscribe
	conn, err := uds.Dial(sock, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := uds.WriteMsg(conn, uds.MsgSubscribe, nil); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Give server time to register subscriber
	time.Sleep(50 * time.Millisecond)

	// Broadcast
	payload := []byte(`{"hook_type":"PreToolUse"}`)
	ps.Broadcast(payload)

	// Read broadcast
	conn.SetReadDeadline(time.Now().Add(time.Second))
	msgType, data, err := uds.ReadMsg(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if msgType != uds.MsgEvent {
		t.Errorf("got msg type %d, want %d", msgType, uds.MsgEvent)
	}
	if string(data) != string(payload) {
		t.Errorf("got %q, want %q", data, payload)
	}
}

func TestBadHandshake(t *testing.T) {
	sock := tempSocket(t)
	ps, err := New(sock)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer ps.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ps.Serve(ctx)

	conn, err := uds.Dial(sock, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// Send wrong message type
	uds.WriteMsg(conn, uds.MsgEvent, nil)
	conn.Close()

	time.Sleep(50 * time.Millisecond)

	ps.mu.Lock()
	n := len(ps.subscribers)
	ps.mu.Unlock()
	if n != 0 {
		t.Errorf("got %d subscribers, want 0", n)
	}
}

func TestDeadSubscriberRemoval(t *testing.T) {
	sock := tempSocket(t)
	ps, err := New(sock)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer ps.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ps.Serve(ctx)

	// Connect and subscribe, then close
	conn, err := uds.Dial(sock, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	uds.WriteMsg(conn, uds.MsgSubscribe, nil)
	time.Sleep(50 * time.Millisecond)
	conn.Close()

	// Broadcast should remove dead subscriber
	ps.Broadcast([]byte(`{}`))
	time.Sleep(50 * time.Millisecond)

	ps.mu.Lock()
	n := len(ps.subscribers)
	ps.mu.Unlock()
	if n != 0 {
		t.Errorf("got %d subscribers after dead removal, want 0", n)
	}
}

func TestConcurrentBroadcast(t *testing.T) {
	sock := tempSocket(t)
	ps, err := New(sock)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer ps.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ps.Serve(ctx)

	// Subscribe
	conn, err := uds.Dial(sock, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	uds.WriteMsg(conn, uds.MsgSubscribe, nil)
	time.Sleep(50 * time.Millisecond)

	// Concurrent broadcasts
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ps.Broadcast([]byte(`{"test":true}`))
		}()
	}
	wg.Wait()

	// Read all messages
	conn.SetReadDeadline(time.Now().Add(time.Second))
	count := 0
	for {
		_, _, err := uds.ReadMsg(conn)
		if err != nil {
			break
		}
		count++
	}
	if count != 20 {
		t.Errorf("received %d messages, want 20", count)
	}
}
