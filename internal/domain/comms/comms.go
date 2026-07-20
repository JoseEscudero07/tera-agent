// Package comms defines the transport port and the wire message contracts used
// to talk to the Backend. The concrete WebSocket implementation lives in
// internal/adapters/communication. This package knows nothing about printers,
// devices or UI. See docs/protocol/ for the full protocol.
package comms

import "context"

// Transport is the driven port for bidirectional messaging. The Agent always
// initiates the connection. Messages are raw JSON frames; the session layer
// encodes/decodes the typed messages in messages.go.
type Transport interface {
	// Connect establishes the connection to the Backend.
	Connect(ctx context.Context) error
	// Send delivers one JSON frame.
	Send(ctx context.Context, data []byte) error
	// Receive returns a channel of inbound JSON frames, closed when the
	// connection ends.
	Receive() <-chan []byte
	// Close terminates the connection.
	Close() error
}
