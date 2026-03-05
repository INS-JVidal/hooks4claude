package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"hooks4claude/shared/filecache"
	"hooks4claude/shared/uds"
)

// runDaemon starts the hook-client in daemon mode, listening on a UDS for
// events from hook-shim. It validates, enriches, and forwards events to the
// monitor (or directly to hooks-store if monitor is not configured).
func runDaemon(socketPath string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handler for graceful shutdown.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sig)
		<-sig
		cancel()
	}()

	ln, err := uds.Listen(socketPath)
	if err != nil {
		return fmt.Errorf("daemon: listen: %w", err)
	}
	defer ln.Close()
	defer os.Remove(socketPath)

	fmt.Printf("hook-client daemon listening on %s\n", socketPath)

	// Discover downstream target (monitor or store).
	ds := newConnPool(discoverDownstreamSocket(), 8)

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				ds.close()
				return nil
			default:
				return fmt.Errorf("daemon: accept: %w", err)
			}
		}
		go handleDaemonConn(ctx, conn, ds)
	}
}

// discoverDownstreamSocket returns the socket path for the next hop.
func discoverDownstreamSocket() string {
	if s := uds.SocketPath("HOOKS_STORE_SOCK", ""); s != "" {
		return s
	}
	return uds.SocketPath("HOOK_MONITOR_SOCK", "") // legacy fallback
}

// connPool maintains a pool of reusable UDS connections to the downstream.
// Goroutines grab a connection, write their message, and return it.
// If a write fails the broken connection is discarded and a fresh one dialed.
//
// All methods are safe for concurrent use.
type connPool struct {
	socketPath string
	maxSize    int
	mu         sync.Mutex
	conns      []net.Conn
	closed     bool
}

func newConnPool(socketPath string, size int) *connPool {
	return &connPool{
		socketPath: socketPath,
		maxSize:    size,
		conns:      make([]net.Conn, 0, size),
	}
}

// get returns a pooled connection or dials a new one.
func (p *connPool) get() (net.Conn, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("pool closed")
	}
	if n := len(p.conns); n > 0 {
		conn := p.conns[n-1]
		p.conns = p.conns[:n-1]
		p.mu.Unlock()
		return conn, nil
	}
	p.mu.Unlock()
	return uds.Dial(p.socketPath, 500*time.Millisecond)
}

// put returns a healthy connection to the pool. If the pool is full or closed,
// the connection is closed instead.
func (p *connPool) put(conn net.Conn) {
	p.mu.Lock()
	if p.closed || len(p.conns) >= p.maxSize {
		p.mu.Unlock()
		conn.Close()
		return
	}
	p.conns = append(p.conns, conn)
	p.mu.Unlock()
}

// send writes a framed message using a pooled connection. On write error it
// retries once with a fresh connection.
func (p *connPool) send(msgType byte, payload []byte) {
	if p.socketPath == "" {
		return
	}
	conn, err := p.get()
	if err != nil {
		return
	}
	if err := uds.WriteMsg(conn, msgType, payload); err != nil {
		conn.Close()
		// Retry once with a fresh connection.
		conn, err = uds.Dial(p.socketPath, 500*time.Millisecond)
		if err != nil {
			return
		}
		if err := uds.WriteMsg(conn, msgType, payload); err != nil {
			conn.Close()
			return
		}
	}
	p.put(conn)
}

// queryCacheAndForward opens a dedicated connection for a request/response
// cache query. This is inherently sequential per query so no pooling needed.
func (p *connPool) queryCacheAndForward(payload []byte) *filecache.CacheQuery {
	if p.socketPath == "" {
		return nil
	}
	conn, err := uds.Dial(p.socketPath, 500*time.Millisecond)
	if err != nil {
		return nil
	}
	defer conn.Close()

	if err := uds.WriteMsg(conn, uds.MsgCacheQuery, payload); err != nil {
		return nil
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))
	msgType, respPayload, err := uds.ReadMsg(conn)
	if err != nil || msgType != uds.MsgCacheResponse {
		return nil
	}

	var q filecache.CacheQuery
	if err := json.Unmarshal(respPayload, &q); err != nil {
		return nil
	}
	return &q
}

func (p *connPool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = nil
}

func handleDaemonConn(ctx context.Context, conn net.Conn, ds *connPool) {
	defer conn.Close()

	for {
		msgType, payload, err := uds.ReadMsg(conn)
		if err != nil {
			return
		}

		switch msgType {
		case uds.MsgEvent:
			processDaemonEvent(payload, ds)
		case uds.MsgCacheQuery:
			processDaemonCacheQuery(conn, payload, ds)
		}
	}
}

func processDaemonEvent(payload []byte, ds *connPool) {
	var inputData map[string]interface{}
	if err := json.Unmarshal(payload, &inputData); err != nil {
		return
	}

	hookType, _ := inputData["hook_event_name"].(string)
	if hookType == "" {
		return
	}
	if !isAlphaOnly(hookType) {
		return
	}

	// Config-based toggle check.
	execPath, _ := os.Executable()
	hookDir := ""
	if execPath != "" {
		hookDir = filepath.Dir(execPath)
	}
	xdgDir := xdgConfigDir()
	projectDir := os.Getenv("CLAUDE_PROJECT_DIR")
	configPath := discoverFile("HOOK_MONITOR_CONFIG", "hook_monitor.conf", xdgDir, hookDir, projectDir)

	if !isHookEnabled(configPath, hookType) {
		return
	}

	// Capture event time once — used for both envelope and monitor metadata.
	now := time.Now().UTC()

	// Enrich with monitor metadata.
	inputData["_monitor"] = map[string]interface{}{
		"timestamp":    now.Format(time.RFC3339Nano),
		"project_dir":  inputData["cwd"],
		"plugin_root":  os.Getenv("CLAUDE_PLUGIN_ROOT"),
		"is_remote":    os.Getenv("CLAUDE_CODE_REMOTE") == "true",
		"has_claude_md": hasClaudeMD(inputData),
	}

	// PostToolUse Read: enrich with file stat metadata.
	if hookType == "PostToolUse" {
		enrichPostToolUseCache(inputData)
	}

	// Wrap in HookEvent envelope: {"hook_type":"...", "timestamp":"...", "data":{...}}
	// This matches the hookevt.HookEvent struct that hooks-store expects.
	envelope := map[string]interface{}{
		"hook_type": hookType,
		"timestamp": now.Format(time.RFC3339Nano),
		"data":      inputData,
	}

	enriched, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	ds.send(uds.MsgEvent, enriched)
}

func processDaemonCacheQuery(conn net.Conn, payload []byte, ds *connPool) {
	var inputData map[string]interface{}
	if err := json.Unmarshal(payload, &inputData); err != nil {
		uds.WriteMsg(conn, uds.MsgCacheResponse, []byte(`{}`))
		return
	}

	sessionID, _ := inputData["session_id"].(string)
	toolInput, _ := inputData["tool_input"].(map[string]interface{})
	filePath := ""
	if toolInput != nil {
		filePath, _ = toolInput["file_path"].(string)
	}
	if filePath == "" {
		uds.WriteMsg(conn, uds.MsgCacheResponse, []byte(`{}`))
		processDaemonEvent(payload, ds)
		return
	}
	filePath = filepath.Clean(filePath)

	var output *hookSpecificOutput

	if sessionID != "" && filePath != "" {
		queryPayload, _ := json.Marshal(map[string]string{
			"session_id": sessionID,
			"file_path":  filePath,
		})

		cached := ds.queryCacheAndForward(queryPayload)
		if cached != nil && cached.Found {
			info, err := os.Stat(filePath)
			if err == nil {
				currentMtime := info.ModTime().UnixNano()
				currentSize := info.Size()

				if currentMtime == cached.MtimeNS && currentSize == cached.Size {
					baseName := filepath.Base(filePath)
					timeStr := cached.LastReadAt.Format("15:04:05")
					msg := fmt.Sprintf(
						"File %s is unchanged since you last read it (%d reads ago, at %s). Consider whether you need to re-read it.",
						baseName, cached.ReadsAgo, timeStr,
					)
					output = &hookSpecificOutput{}
					output.HookSpecificOutput.HookEventName = "PreToolUse"
					output.HookSpecificOutput.PermissionDecision = "allow"
					output.HookSpecificOutput.AdditionalContext = msg
				}
			}
		}
	}

	if output != nil {
		respPayload, _ := json.Marshal(output)
		uds.WriteMsg(conn, uds.MsgCacheResponse, respPayload)
	} else {
		uds.WriteMsg(conn, uds.MsgCacheResponse, []byte(`{}`))
	}

	processDaemonEvent(payload, ds)
}
