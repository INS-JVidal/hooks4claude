# uds — Unix domain socket framing protocol

Wire protocol: `[type:1][len:4][json:len]` (big-endian uint32 length).

```go
const MsgEvent, MsgCacheQuery, MsgCacheResponse byte = 0x01, 0x02, 0x03
const MaxPayload = 1 << 20 // 1 MiB

func WriteMsg(conn net.Conn, msgType byte, payload []byte) error
func ReadMsg(conn net.Conn) (msgType byte, payload []byte, err error)
func Listen(socketPath string) (net.Listener, error)    // unlinks stale, chmod 0600
func Dial(socketPath string, timeout time.Duration) (net.Conn, error)
func SocketPath(envVar, defaultPath string) string       // env override with fallback
```

Socket path defaults (env overridable):
- `HOOK_CLIENT_SOCK` → `/tmp/hook-client.sock`
- `HOOK_MONITOR_SOCK` → `/tmp/hook-monitor.sock`
- `HOOKS_STORE_SOCK` → `/tmp/hooks-store.sock`

No internal imports. No concurrency primitives.
