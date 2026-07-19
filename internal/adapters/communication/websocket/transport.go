// Package websocket will hold the secure WebSocket implementation of
// comms.Transport: TLS, heartbeat, reconnection with backoff, compression,
// timeouts and the Token handshake. Owner: Communication Engineer.
//
// This file is a compiling STUB so the composition root wires end to end.
// Replace it with the real implementation (analyze -> propose -> approve ->
// implement). It must not know about printers, devices or UI.
package websocket

import (
	"context"
	"errors"

	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/comms"
)

// ErrNotImplemented marks the pending real implementation.
var ErrNotImplemented = errors.New("websocket transport: not implemented yet")

// Transport is the stub WebSocket transport.
type Transport struct {
	cfg    ports.Config
	log    ports.Logger
	inbox  chan comms.Envelope
}

// New returns a stub Transport. The real one dials cfg.BackendURL over TLS.
func New(cfg ports.Config, log ports.Logger) *Transport {
	return &Transport{
		cfg:   cfg,
		log:   log,
		inbox: make(chan comms.Envelope),
	}
}

func (t *Transport) Connect(ctx context.Context) error {
	t.log.Warn("websocket transport is a stub", "backend", t.cfg.BackendURL)
	return ErrNotImplemented
}

func (t *Transport) Send(ctx context.Context, msg comms.Envelope) error {
	return ErrNotImplemented
}

func (t *Transport) Receive() <-chan comms.Envelope { return t.inbox }

func (t *Transport) Close() error {
	close(t.inbox)
	return nil
}

// compile-time assertion that the stub satisfies the port.
var _ comms.Transport = (*Transport)(nil)
