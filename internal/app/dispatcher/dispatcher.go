// Package dispatcher routes inbound jobs to the handler registered for their
// Kind. It is transport-agnostic: the communication adapter decodes envelopes
// and hands Jobs here; feature agents register handlers. Owner: Go Core Engineer.
package dispatcher

import (
	"context"
	"fmt"

	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/job"
)

// Handler executes a single kind of job. Printing and device handlers live in
// their respective adapters and are registered at wiring time.
type Handler interface {
	Handle(ctx context.Context, j job.Job) error
}

// Dispatcher maps a job.Kind to its Handler.
type Dispatcher struct {
	log      ports.Logger
	handlers map[job.Kind]Handler
}

// New returns an empty Dispatcher.
func New(log ports.Logger) *Dispatcher {
	return &Dispatcher{
		log:      log,
		handlers: make(map[job.Kind]Handler),
	}
}

// Register binds a handler to a job kind. Registering a kind twice is a
// programming error and panics at wiring time.
func (d *Dispatcher) Register(kind job.Kind, h Handler) {
	if _, exists := d.handlers[kind]; exists {
		panic(fmt.Sprintf("dispatcher: handler already registered for kind %q", kind))
	}
	d.handlers[kind] = h
}

// Dispatch routes a job to its handler.
func (d *Dispatcher) Dispatch(ctx context.Context, j job.Job) error {
	h, ok := d.handlers[j.Kind]
	if !ok {
		return fmt.Errorf("dispatcher: no handler for kind %q (job %s)", j.Kind, j.ID)
	}
	d.log.Debug("dispatching job", "id", j.ID, "kind", j.Kind)
	return h.Handle(ctx, j)
}
