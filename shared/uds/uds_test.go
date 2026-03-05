package uds

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tempSocket(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.sock")
}

func TestRoundTrip(t *testing.T) {
	sock := tempSocket(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	tests := []struct {
		msgType byte
		payload []byte
	}{
		{MsgEvent, []byte(`{"hook_type":"PreToolUse"}`)},
		{MsgCacheQuery, []byte(`{"session_id":"abc","file_path":"/tmp/x"}`)},
		{MsgCacheResponse, []byte(`{"found":true}`)},
		{MsgEvent, nil},           // empty payload
		{MsgEvent, []byte("{}")},  // minimal JSON
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			done := make(chan struct{})
			var gotType byte
			var gotPayload []byte

			go func() {
				conn, err := ln.Accept()
				if err != nil {
					t.Error(err)
					close(done)
					return
				}
				defer conn.Close()
				gotType, gotPayload, err = ReadMsg(conn)
				if err != nil {
					t.Error(err)
				}
				close(done)
			}()

			conn, err := Dial(sock, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if err := WriteMsg(conn, tt.msgType, tt.payload); err != nil {
				t.Fatal(err)
			}
			conn.Close()
			<-done

			if gotType != tt.msgType {
				t.Errorf("type: got %d, want %d", gotType, tt.msgType)
			}
			if string(gotPayload) != string(tt.payload) {
				t.Errorf("payload: got %q, want %q", gotPayload, tt.payload)
			}
		})
	}
}

func TestMaxPayload(t *testing.T) {
	sock := tempSocket(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	conn, err := Dial(sock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	big := make([]byte, MaxPayload+1)
	if err := WriteMsg(conn, MsgEvent, big); err != ErrPayloadTooLarge {
		t.Errorf("expected ErrPayloadTooLarge, got %v", err)
	}
}

func TestConcurrentWriters(t *testing.T) {
	sock := tempSocket(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	const numWriters = 20
	var received sync.WaitGroup
	received.Add(numWriters)

	// Server: accept connections, read one message each.
	go func() {
		for i := 0; i < numWriters; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _, err := ReadMsg(c)
				if err != nil {
					t.Error(err)
				}
				received.Done()
			}(conn)
		}
	}()

	// Writers: each opens a connection and sends one message.
	var wg sync.WaitGroup
	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func() {
			defer wg.Done()
			conn, err := Dial(sock, time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			if err := WriteMsg(conn, MsgEvent, []byte(`{"test":true}`)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	received.Wait()
}

func TestListenRemovesStale(t *testing.T) {
	sock := tempSocket(t)
	// Create a stale file.
	if err := os.WriteFile(sock, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
}

func TestListenPermissions(t *testing.T) {
	sock := tempSocket(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	info, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("socket permissions: got %o, want 0600", perm)
	}
}

func TestSocketPath(t *testing.T) {
	// With env var set.
	t.Setenv("TEST_SOCK", "/custom/path.sock")
	if got := SocketPath("TEST_SOCK", "/default.sock"); got != "/custom/path.sock" {
		t.Errorf("got %q, want /custom/path.sock", got)
	}
	// Without env var.
	t.Setenv("TEST_SOCK", "")
	if got := SocketPath("TEST_SOCK", "/default.sock"); got != "/default.sock" {
		t.Errorf("got %q, want /default.sock", got)
	}
}

func TestDialFailure(t *testing.T) {
	_, err := Dial("/nonexistent/path.sock", 50*time.Millisecond)
	if err == nil {
		t.Error("expected error dialing nonexistent socket")
	}
}
