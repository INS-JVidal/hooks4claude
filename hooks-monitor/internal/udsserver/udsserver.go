package udsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"claude-hooks-monitor/internal/hookevt"
	"claude-hooks-monitor/internal/monitor"

	"hooks4claude/shared/filecache"
	"hooks4claude/shared/uds"
)

// UDSServer accepts hook events and cache queries over a Unix domain socket.
type UDSServer struct {
	socketPath string
	mon        *monitor.HookMonitor
	fc         *filecache.SessionFileCache // nil = cache queries return {found:false}
	listener   net.Listener
}

// New creates a UDS server bound to the given socket path.
// Pass nil for fc to disable cache query handling.
func New(socketPath string, mon *monitor.HookMonitor, fc *filecache.SessionFileCache) (*UDSServer, error) {
	ln, err := uds.Listen(socketPath)
	if err != nil {
		return nil, err
	}
	return &UDSServer{
		socketPath: socketPath,
		mon:        mon,
		fc:         fc,
		listener:   ln,
	}, nil
}

// Serve accepts connections until ctx is cancelled.
func (s *UDSServer) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.listener.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("uds monitor: accept: %w", err)
			}
		}
		go s.handleConn(conn)
	}
}

func (s *UDSServer) handleConn(conn net.Conn) {
	defer conn.Close()

	for {
		msgType, payload, err := uds.ReadMsg(conn)
		if err != nil {
			return
		}

		switch msgType {
		case uds.MsgEvent:
			s.handleEvent(payload)
		case uds.MsgCacheQuery:
			s.handleCacheQuery(conn, payload)
		}
	}
}

func (s *UDSServer) handleEvent(payload []byte) {
	if len(payload) == 0 || len(payload) > monitor.MaxBodyLen {
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}

	// Detect HookEvent envelope format: {"hook_type":"...", "data":{...}}
	// The daemon wraps events in this envelope. If present, unwrap it so
	// event.Data contains the actual payload (with tool_name, session_id, etc.)
	// rather than the envelope itself.
	hookType, _ := data["hook_type"].(string)
	eventData := data
	if hookType != "" {
		if nested, ok := data["data"].(map[string]interface{}); ok {
			eventData = nested
		}
	}

	// Fallback: flat Claude Code JSON with hook_event_name (legacy format).
	if hookType == "" {
		hookType, _ = data["hook_event_name"].(string)
	}
	if hookType == "" {
		return
	}

	// Parse timestamp from envelope if present, else use now.
	ts := time.Now()
	if tsStr, ok := data["timestamp"].(string); ok && tsStr != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
			ts = parsed
		}
	}

	event := hookevt.HookEvent{
		HookType:  hookType,
		Timestamp: ts,
		Data:      eventData,
	}
	s.mon.AddEvent(event)
}

// cacheQueryReq is the wire format for cache queries.
type cacheQueryReq struct {
	SessionID string `json:"session_id"`
	FilePath  string `json:"file_path"`
}

func (s *UDSServer) handleCacheQuery(conn net.Conn, payload []byte) {
	if s.fc == nil {
		uds.WriteMsg(conn, uds.MsgCacheResponse, []byte(`{"found":false}`))
		return
	}

	var req cacheQueryReq
	if err := json.Unmarshal(payload, &req); err != nil {
		uds.WriteMsg(conn, uds.MsgCacheResponse, []byte(`{"found":false}`))
		return
	}

	result := s.fc.Lookup(req.SessionID, req.FilePath)
	resp, _ := json.Marshal(result)
	uds.WriteMsg(conn, uds.MsgCacheResponse, resp)
}

// Close stops accepting connections.
func (s *UDSServer) Close() error {
	return s.listener.Close()
}
