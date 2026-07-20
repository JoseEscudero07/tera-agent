// Package local provides a LocalTransport: a comms.Transport that runs the Agent
// without a backend. It lets the MVP work offline (printing via CLI/HTTP) and
// keeps the seam identical to the WebSocket transport. Owner: Communication Engineer.
package local

import (
	"context"

	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/comms"
)

// Transport is a no-backend transport. It "connects" locally and never delivers
// inbound frames; outbound sends are dropped.
type Transport struct {
	log   ports.Logger
	inbox chan []byte
}

// New returns a LocalTransport.
func New(log ports.Logger) *Transport {
	return &Transport{log: log, inbox: make(chan []byte)}
}

func (t *Transport) Connect(context.Context) error {
	t.log.Info("local transport active (no backend configured)")
	return nil
}

func (t *Transport) Send(context.Context, []byte) error { return nil }

func (t *Transport) Receive() <-chan []byte { return t.inbox }

func (t *Transport) Close() error {
	close(t.inbox)
	return nil
}

var _ comms.Transport = (*Transport)(nil)
