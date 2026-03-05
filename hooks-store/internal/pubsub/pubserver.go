package pubsub

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"hooks4claude/shared/uds"
)

// subscriber wraps a UDS connection with a write mutex to ensure
// atomic framed writes (header + payload cannot interleave).
type subscriber struct {
	conn net.Conn
	mu   sync.Mutex
	dead bool
}

// PubServer accepts subscriber connections and broadcasts events to them.
// Subscribers connect via UDS and send a MsgSubscribe handshake.
type PubServer struct {
	mu          sync.Mutex
	subscribers []*subscriber
	listener    net.Listener
}

// New creates a PubServer listening on the given Unix socket path.
func New(socketPath string) (*PubServer, error) {
	ln, err := uds.Listen(socketPath)
	if err != nil {
		return nil, fmt.Errorf("pubsub: listen: %w", err)
	}
	return &PubServer{listener: ln}, nil
}

// Serve accepts subscriber connections until ctx is cancelled.
func (p *PubServer) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		p.listener.Close()
	}()

	for {
		conn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				fmt.Fprintf(os.Stderr, "pubsub: accept error: %v\n", err)
				continue
			}
		}
		go p.handleConn(conn)
	}
}

func (p *PubServer) handleConn(conn net.Conn) {
	// Expect MsgSubscribe handshake within 5 seconds.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msgType, _, err := uds.ReadMsg(conn)
	if err != nil || msgType != uds.MsgSubscribe {
		conn.Close()
		return
	}
	// Clear deadline for the long-lived connection.
	conn.SetReadDeadline(time.Time{})

	sub := &subscriber{conn: conn}
	p.mu.Lock()
	p.subscribers = append(p.subscribers, sub)
	p.mu.Unlock()
}

// Broadcast sends a framed MsgEvent to all subscribers.
// Dead subscribers (write failure or timeout) are removed.
// This method is safe for concurrent use.
func (p *PubServer) Broadcast(payload []byte) {
	p.mu.Lock()
	subs := make([]*subscriber, len(p.subscribers))
	copy(subs, p.subscribers)
	p.mu.Unlock()

	var dead []*subscriber
	for _, sub := range subs {
		sub.mu.Lock()
		if sub.dead {
			sub.mu.Unlock()
			continue
		}
		sub.conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		if err := uds.WriteMsg(sub.conn, uds.MsgEvent, payload); err != nil {
			sub.dead = true
			sub.conn.Close()
			dead = append(dead, sub)
		}
		sub.mu.Unlock()
	}

	if len(dead) > 0 {
		fmt.Fprintf(os.Stderr, "pubsub: removed %d dead subscriber(s) (%d remaining)\n", len(dead), len(subs)-len(dead))
		p.removeDead(dead)
	}
}

func (p *PubServer) removeDead(dead []*subscriber) {
	deadSet := make(map[*subscriber]struct{}, len(dead))
	for _, d := range dead {
		deadSet[d] = struct{}{}
	}

	p.mu.Lock()
	alive := p.subscribers[:0]
	for _, sub := range p.subscribers {
		if _, ok := deadSet[sub]; !ok {
			alive = append(alive, sub)
		}
	}
	p.subscribers = alive
	p.mu.Unlock()
}

// Close stops the listener and disconnects all subscribers.
func (p *PubServer) Close() error {
	err := p.listener.Close()
	p.mu.Lock()
	for _, sub := range p.subscribers {
		sub.conn.Close()
	}
	p.subscribers = nil
	p.mu.Unlock()
	return err
}
