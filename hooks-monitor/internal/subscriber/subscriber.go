package subscriber

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"claude-hooks-monitor/internal/hookevt"
	"claude-hooks-monitor/internal/monitor"

	"hooks4claude/shared/uds"
)

// Subscriber connects to hooks-store's pub socket and feeds events to the monitor.
type Subscriber struct {
	socketPath string
}

// New creates a Subscriber targeting the given pub socket path.
func New(socketPath string) *Subscriber {
	return &Subscriber{socketPath: socketPath}
}

// Run connects to the pub socket, sends the subscribe handshake, and reads
// events in a loop, feeding them to mon.AddEvent(). On disconnect it
// reconnects with exponential backoff. Blocks until ctx is cancelled.
func (s *Subscriber) Run(ctx context.Context, mon *monitor.HookMonitor) error {
	backoff := time.Second
	const maxBackoff = 10 * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		err := s.connectAndRead(ctx, mon)
		if err != nil && ctx.Err() == nil {
			fmt.Printf("subscriber: disconnected: %v (reconnecting in %v)\n", err, backoff)
		} else {
			// Successful connection ended cleanly; reset backoff.
			backoff = time.Second
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (s *Subscriber) connectAndRead(ctx context.Context, mon *monitor.HookMonitor) error {
	conn, err := uds.Dial(s.socketPath, 2*time.Second)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// Send subscribe handshake.
	if err := uds.WriteMsg(conn, uds.MsgSubscribe, nil); err != nil {
		return fmt.Errorf("subscribe handshake: %w", err)
	}

	// Reset backoff on successful connect (caller handles this via return nil on ctx cancel).
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msgType, payload, err := uds.ReadMsg(conn)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		if msgType != uds.MsgEvent {
			continue
		}

		var evt hookevt.HookEvent
		if err := json.Unmarshal(payload, &evt); err != nil {
			continue
		}
		mon.AddEvent(evt)
	}
}
