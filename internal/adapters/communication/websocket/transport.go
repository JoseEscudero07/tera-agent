// Package websocket implements comms.Transport over a WebSocket connection using
// gorilla/websocket. Supports ws:// (no TLS) and wss://. Owner: Communication
// Engineer. It knows nothing about printers, devices or UI.
package websocket

import (
	"context"
	"sync"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

// Transport is a WebSocket client transport.
type Transport struct {
	url string
	log ports.Logger

	mu    sync.Mutex // guards writes (gorilla allows a single concurrent writer)
	conn  *gws.Conn
	inbox chan []byte
	done  chan struct{}
	once  sync.Once
}

// New returns a WebSocket transport dialing url.
func New(url string, log ports.Logger) *Transport {
	return &Transport{url: url, log: log, inbox: make(chan []byte, 32), done: make(chan struct{})}
}

// Connect dials the server and starts the read pump.
func (t *Transport) Connect(ctx context.Context) error {
	dialer := gws.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, t.url, nil)
	if err != nil {
		return err
	}
	t.conn = conn
	go t.readPump()
	return nil
}

func (t *Transport) readPump() {
	defer close(t.inbox)
	for {
		_, msg, err := t.conn.ReadMessage()
		if err != nil {
			return
		}
		select {
		case t.inbox <- msg:
		case <-t.done:
			return
		}
	}
}

// Send writes one JSON frame.
func (t *Transport) Send(_ context.Context, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn.WriteMessage(gws.TextMessage, data)
}

// Receive returns the inbound frame channel.
func (t *Transport) Receive() <-chan []byte { return t.inbox }

// Close terminates the connection.
func (t *Transport) Close() error {
	t.once.Do(func() { close(t.done) })
	if t.conn != nil {
		return t.conn.Close()
	}
	return nil
}
