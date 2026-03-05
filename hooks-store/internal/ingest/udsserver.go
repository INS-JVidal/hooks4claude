package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"time"

	"hooks-store/internal/hookevt"
	"hooks-store/internal/store"

	"hooks4claude/shared/filecache"
	"hooks4claude/shared/uds"
)

// UDSIngestServer accepts hook events over a Unix domain socket using
// the shared framing protocol (1-byte type + 4-byte length + JSON payload).
type UDSIngestServer struct {
	socketPath string
	store      store.EventStore
	listener   net.Listener
	ingested   atomic.Int64
	errors     atomic.Int64
	lastEvent  atomic.Value // stores time.Time
	onIngest   func(IngestEvent, []byte)
	fc         *filecache.SessionFileCache
}

// NewUDS creates a UDS ingest server bound to the given socket path.
// Pass a non-nil filecache to enable MsgCacheQuery handling and file read tracking.
func NewUDS(socketPath string, s store.EventStore, fc *filecache.SessionFileCache) (*UDSIngestServer, error) {
	ln, err := uds.Listen(socketPath)
	if err != nil {
		return nil, err
	}
	return &UDSIngestServer{
		socketPath: socketPath,
		store:      s,
		listener:   ln,
		fc:         fc,
	}, nil
}

// SetOnIngest registers a callback invoked after each successful ingest.
func (u *UDSIngestServer) SetOnIngest(fn func(IngestEvent, []byte)) {
	u.onIngest = fn
}

// Serve accepts connections and processes framed messages until ctx is cancelled.
func (u *UDSIngestServer) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		u.listener.Close()
	}()

	for {
		conn, err := u.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("uds ingest: accept: %w", err)
			}
		}
		go u.handleConn(ctx, conn)
	}
}

func (u *UDSIngestServer) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	for {
		msgType, payload, err := uds.ReadMsg(conn)
		if err != nil {
			return // connection closed or read error
		}

		switch msgType {
		case uds.MsgEvent:
			u.processEvent(ctx, payload)
		case uds.MsgCacheQuery:
			u.handleCacheQuery(conn, payload)
		default:
			fmt.Fprintf(os.Stderr, "uds ingest: unknown message type 0x%02x (%d bytes)\n", msgType, len(payload))
		}
	}
}

func (u *UDSIngestServer) processEvent(ctx context.Context, payload []byte) {
	if len(payload) == 0 {
		u.errors.Add(1)
		fmt.Fprintf(os.Stderr, "uds ingest: empty payload\n")
		return
	}
	if len(payload) > maxBodyLen {
		u.errors.Add(1)
		fmt.Fprintf(os.Stderr, "uds ingest: payload too large (%d bytes)\n", len(payload))
		return
	}
	if err := checkJSONDepth(payload, maxJSONDepth); err != nil {
		u.errors.Add(1)
		fmt.Fprintf(os.Stderr, "uds ingest: %v\n", err)
		return
	}

	var evt hookevt.HookEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		u.errors.Add(1)
		fmt.Fprintf(os.Stderr, "uds ingest: unmarshal: %v\n", err)
		return
	}
	if evt.HookType == "" {
		u.errors.Add(1)
		fmt.Fprintf(os.Stderr, "uds ingest: missing hook_type\n")
		return
	}

	doc := store.HookEventToDocument(evt)
	if err := u.store.Index(ctx, doc); err != nil {
		u.errors.Add(1)
		fmt.Fprintf(os.Stderr, "uds ingest: index failed for %s: %v\n", evt.HookType, err)
		return
	}

	u.ingested.Add(1)
	u.lastEvent.Store(time.Now())

	// Process file cache updates if configured.
	if u.fc != nil {
		u.processFileCache(evt)
	}

	if u.onIngest != nil {
		toolName, _ := evt.Data["tool_name"].(string)
		sessionID, _ := evt.Data["session_id"].(string)
		u.onIngest(IngestEvent{
			HookType:  evt.HookType,
			ToolName:  toolName,
			SessionID: sessionID,
			BodySize:  len(payload),
			Timestamp: evt.Timestamp,
		}, payload)
	}
}

// handleCacheQuery responds to a file cache lookup request.
// Each conn is handled by a single goroutine, so WriteMsg is safe without extra locking.
func (u *UDSIngestServer) handleCacheQuery(conn net.Conn, payload []byte) {
	if u.fc == nil {
		uds.WriteMsg(conn, uds.MsgCacheResponse, []byte(`{"found":false}`))
		return
	}

	var query struct {
		SessionID string `json:"session_id"`
		FilePath  string `json:"file_path"`
	}
	if err := json.Unmarshal(payload, &query); err != nil || query.SessionID == "" || query.FilePath == "" {
		uds.WriteMsg(conn, uds.MsgCacheResponse, []byte(`{"found":false}`))
		return
	}

	result := u.fc.Lookup(query.SessionID, query.FilePath)
	resp, _ := json.Marshal(result)
	uds.WriteMsg(conn, uds.MsgCacheResponse, resp)
}

// processFileCache records file reads and cleans up sessions.
func (u *UDSIngestServer) processFileCache(evt hookevt.HookEvent) {
	switch evt.HookType {
	case "PostToolUse":
		toolName, _ := evt.Data["tool_name"].(string)
		if toolName != "Read" {
			return
		}
		cache, ok := evt.Data["_cache"].(map[string]interface{})
		if !ok {
			return
		}
		filePath, _ := cache["file_path"].(string)
		if filePath == "" {
			return
		}
		mtimeNS := jsonInt64(cache["mtime_ns"])
		if mtimeNS == 0 {
			return
		}
		size := jsonInt64(cache["size"])
		sessionID, _ := evt.Data["session_id"].(string)
		if sessionID == "" {
			return
		}
		u.fc.RecordRead(sessionID, filePath, mtimeNS, size)

	case "SessionEnd":
		sessionID, _ := evt.Data["session_id"].(string)
		if sessionID != "" {
			u.fc.EndSession(sessionID)
		}
	}
}

// jsonInt64 extracts an int64 from a value that may be json.Number or float64.
func jsonInt64(v interface{}) int64 {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case float64:
		return int64(n)
	}
	return 0
}

// Ingested returns the total number of events ingested via UDS.
func (u *UDSIngestServer) Ingested() int64 {
	return u.ingested.Load()
}

// Errors returns the total number of UDS ingest errors.
func (u *UDSIngestServer) Errors() int64 {
	return u.errors.Load()
}

// LastEvent returns the timestamp of the most recent UDS-ingested event.
// Returns zero time if no events have been ingested.
func (u *UDSIngestServer) LastEvent() time.Time {
	if v := u.lastEvent.Load(); v != nil {
		if t, ok := v.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// Close stops accepting connections and removes the socket file.
func (u *UDSIngestServer) Close() error {
	return u.listener.Close()
}
