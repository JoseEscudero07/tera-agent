// Package local provides a LocalTransport: a comms.Transport that runs the Agent
// without a backend. It lets the MVP work offline (printing via CLI) and keeps
// the seam ready for the future WebSocketTransport. Owner: Communication Engineer.
package local

import (
	"context"

	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/comms"
)

// Transport is a no-backend transport. It "connects" locally and never delivers
// inbound messages; outbound sends are dropped (there is no server yet).
type Transport struct {
	log   ports.Logger
	inbox chan comms.Envelope
}

// New returns a LocalTransport.
func New(log ports.Logger) *Transport {
	return &Transport{log: log, inbox: make(chan comms.Envelope)}
}

func (t *Transport) Connect(context.Context) error {
	t.log.Info("local transport active (no backend configured)")
	return nil
}

func (t *Transport) Send(context.Context, comms.Envelope) error { return nil }

func (t *Transport) Receive() <-chan comms.Envelope { return t.inbox }

func (t *Transport) Close() error {
	close(t.inbox)
	return nil
}

var _ comms.Transport = (*Transport)(nil)
