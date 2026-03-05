# ingest — HTTP + UDS ingest server receiving hook events from the monitor

All files stable — prefer this summary over reading source files.

## server.go

```go
type IngestEvent struct {
    HookType  string
    ToolName  string
    SessionID string
    BodySize  int
    Timestamp time.Time
}

type Server struct { /* unexported fields */ }
func New(s store.EventStore) *Server
func (s *Server) Handler() http.Handler
func (s *Server) SetOnIngest(fn func(IngestEvent))
func (s *Server) ErrCount() *atomic.Int64
```

Routes: POST /ingest, GET /health, GET /stats. Validates body size (1 MiB max), JSON depth (100 max), requires hook_type. Calls onIngest callback after successful indexing. Tracks ingested/errors via atomic counters.

Concurrency: `atomic.Int64` for ingested/errors counters, `atomic.Value` for lastEvent timestamp. onIngest callback must be non-blocking.

## server_test.go

Tests: TestHandleIngest_Success, _MethodNotAllowed, _EmptyBody, _InvalidJSON, _MissingHookType, _BodyTooLarge, _StoreError, _DeepJSON, TestHandleHealth, TestHandleStats_Empty, _AfterIngest, TestHandleIngest_Concurrent (50 goroutines), _ResponseBodyDrained, _ErrorContentType. Uses mockStore test double.

## integration_test.go

Tests: TestEndToEnd_WireFormat, _AllHookTypes (15 types), _CompanionDown, _ConcurrentBurst (100 goroutines). Simulates full monitor→companion pipeline using httptest.NewServer.

## udsserver.go

```go
type UDSIngestServer struct { /* unexported fields */ }
func NewUDS(socketPath string, s store.EventStore) (*UDSIngestServer, error)
func (u *UDSIngestServer) SetOnIngest(fn func(IngestEvent))
func (u *UDSIngestServer) Serve(ctx context.Context) error
func (u *UDSIngestServer) Close() error
```

Accepts framed MsgEvent (0x01) messages over UDS. Same validation as HTTP handler. Accepts multiple messages per connection.

## uds_integration_test.go

Tests: TestUDS_RoundTrip, _AllHookTypes, _ConcurrentBurst (20 goroutines), _EmptyPayload, _MissingHookType.

Imports: `hookevt` (HookEvent), `store` (EventStore, Document, HookEventToDocument), `shared/uds`.
