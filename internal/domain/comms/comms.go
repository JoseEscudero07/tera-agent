// Package comms defines the message contracts and transport port used to talk
// to the Backend. The concrete WebSocket implementation lives in
// internal/adapters/communication. Owner ports: Software Architect;
// implementation: Communication Engineer. This package knows nothing about
// printers, devices or UI.
package comms

import "context"

// MessageType enumerates the protocol message kinds. The full protocol is
// documented in docs/PROTOCOL.md.
type MessageType string

const (
	// Agent -> Backend
	TypeAuth      MessageType = "AUTH"      // presents the Token
	TypeHeartbeat MessageType = "HEARTBEAT" // liveness ping
	TypeResult    MessageType = "RESULT"    // job outcome

	// Backend -> Agent
	TypeAuthOK  MessageType = "AUTH_OK" // carries Identity + initial config
	TypeAuthErr MessageType = "AUTH_ERR"
	TypeJob     MessageType = "JOB" // a unit of work to execute
)

// Envelope is the framing for every message on the wire. Data is the
// type-specific body, serialized by the adapter (e.g. JSON).
type Envelope struct {
	Type MessageType `json:"type"`
	Data []byte      `json:"data"`
}

// AuthRequest is sent by the Agent during the registration/authentication
// handshake. It carries ONLY the Token issued by the Backend — the Agent never
// generates or renews Tokens.
type AuthRequest struct {
	Token string `json:"token"`
}

// Transport is the driven port the core uses for bidirectional messaging. The
// Agent always initiates the connection over TLS. Implementations own
// reconnection, heartbeat, compression and timeouts.
type Transport interface {
	// Connect establishes the secure connection to the Backend.
	Connect(ctx context.Context) error
	// Send delivers one envelope to the Backend.
	Send(ctx context.Context, msg Envelope) error
	// Receive returns a channel of inbound envelopes. It is closed when the
	// connection ends.
	Receive() <-chan Envelope
	// Close terminates the connection.
	Close() error
}
