# udsserver — UDS listener for hook-monitor

Accepts hook events and cache queries over Unix domain socket.

```go
type UDSServer struct { /* unexported */ }
func New(socketPath string, mon *monitor.HookMonitor, fc *filecache.SessionFileCache) (*UDSServer, error)
func (s *UDSServer) Serve(ctx context.Context) error
func (s *UDSServer) Close() error
```

Message handling:
- `0x01` (MsgEvent): detects HookEvent envelope format `{"hook_type":"...", "data":{...}}` and unwraps it. Falls back to flat Claude Code JSON with `hook_event_name`. Parses envelope timestamp or uses `time.Now()`. Calls `mon.AddEvent()`.
- `0x02` (MsgCacheQuery): unmarshals `{session_id, file_path}`, calls `fc.Lookup()`, writes `0x03` response

Imports: `monitor`, `filecache`, `hookevt`, `shared/uds`.
