package uds

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// Message types for the UDS wire protocol.
const (
	MsgEvent         byte = 0x01 // Fire-and-forget event
	MsgCacheQuery    byte = 0x02 // Cache query (expects response)
	MsgCacheResponse byte = 0x03 // Cache response
	MsgSubscribe     byte = 0x04 // Subscribe to event stream
)

// MaxPayload is the maximum allowed payload size (1 MiB).
const MaxPayload = 1 << 20

// ErrPayloadTooLarge is returned when a message exceeds MaxPayload.
var ErrPayloadTooLarge = errors.New("uds: payload exceeds 1 MiB limit")

// WriteMsg writes a framed message: [type:1][len:4][json:len].
func WriteMsg(conn net.Conn, msgType byte, payload []byte) error {
	if len(payload) > MaxPayload {
		return ErrPayloadTooLarge
	}
	// Header: 1 byte type + 4 bytes big-endian length.
	var hdr [5]byte
	hdr[0] = msgType
	binary.BigEndian.PutUint32(hdr[1:5], uint32(len(payload)))
	if _, err := conn.Write(hdr[:]); err != nil {
		return fmt.Errorf("uds: write header: %w", err)
	}
	if len(payload) > 0 {
		if _, err := conn.Write(payload); err != nil {
			return fmt.Errorf("uds: write payload: %w", err)
		}
	}
	return nil
}

// ReadMsg reads a framed message from the connection.
func ReadMsg(conn net.Conn) (msgType byte, payload []byte, err error) {
	var hdr [5]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return 0, nil, fmt.Errorf("uds: read header: %w", err)
	}
	msgType = hdr[0]
	length := binary.BigEndian.Uint32(hdr[1:5])
	if length > MaxPayload {
		return 0, nil, ErrPayloadTooLarge
	}
	if length == 0 {
		return msgType, nil, nil
	}
	payload = make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return 0, nil, fmt.Errorf("uds: read payload: %w", err)
	}
	return msgType, payload, nil
}

// Listen creates a Unix domain socket listener. It removes any stale socket
// file and sets permissions to 0600.
func Listen(socketPath string) (net.Listener, error) {
	// Remove stale socket if it exists.
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("uds: remove stale socket: %w", err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("uds: listen: %w", err)
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("uds: chmod socket: %w", err)
	}
	return ln, nil
}

// Dial connects to a Unix domain socket with the given timeout.
func Dial(socketPath string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, fmt.Errorf("uds: dial %s: %w", socketPath, err)
	}
	return conn, nil
}

// SocketPath returns the socket path from an environment variable,
// falling back to the provided default if the env var is empty.
func SocketPath(envVar, defaultPath string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultPath
}
