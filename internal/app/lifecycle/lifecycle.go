// Package lifecycle orchestrates the Agent's startup and shutdown, driving the
// domain state machine as it connects, authenticates, registers and serves
// jobs. It depends only on ports/domain interfaces, never on concrete adapters.
// Owner: Go Core Engineer.
package lifecycle

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/teraerp/tera-agent/internal/app/dispatcher"
	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	"github.com/teraerp/tera-agent/internal/domain/comms"
	"github.com/teraerp/tera-agent/internal/domain/job"
)

// ErrAuthRejected is returned when the Backend rejects the Token.
var ErrAuthRejected = errors.New("lifecycle: authentication rejected by backend")

// Lifecycle wires the state machine to the transport and dispatcher. It owns no
// business logic of its own beyond sequencing the handshake and serve loop.
type Lifecycle struct {
	machine    *agent.Machine
	transport  comms.Transport
	dispatcher *dispatcher.Dispatcher
	cfg        ports.Config
	log        ports.Logger

	identity agent.Identity
}

// New constructs a Lifecycle. The state machine is created here so observers
// (e.g. the UI) can subscribe before Run.
func New(
	machine *agent.Machine,
	transport comms.Transport,
	disp *dispatcher.Dispatcher,
	cfg ports.Config,
	log ports.Logger,
) *Lifecycle {
	return &Lifecycle{
		machine:    machine,
		transport:  transport,
		dispatcher: disp,
		cfg:        cfg,
		log:        log,
	}
}

// Run executes the full lifecycle until ctx is cancelled or an unrecoverable
// error occurs. Reconnection strategy (RECONNECTING/OFFLINE) is intentionally
// left to a follow-up implementation approved by the Software Architect.
func (l *Lifecycle) Run(ctx context.Context) error {
	if err := l.connect(ctx); err != nil {
		_ = l.machine.Transition(agent.StateError)
		return err
	}
	if err := l.authenticate(ctx); err != nil {
		_ = l.machine.Transition(agent.StateError)
		return err
	}
	return l.serve(ctx)
}

func (l *Lifecycle) connect(ctx context.Context) error {
	if err := l.machine.Transition(agent.StateConnecting); err != nil {
		return err
	}
	l.log.Info("connecting to backend", "url", l.cfg.BackendURL)
	return l.transport.Connect(ctx)
}

// authenticate performs the Token handshake. The Agent only transmits the Token
// issued by the Backend; it never generates or renews it.
func (l *Lifecycle) authenticate(ctx context.Context) error {
	if err := l.machine.Transition(agent.StateAuthenticating); err != nil {
		return err
	}

	body, err := json.Marshal(comms.AuthRequest{Token: l.cfg.Token})
	if err != nil {
		return err
	}
	if err := l.transport.Send(ctx, comms.Envelope{Type: comms.TypeAuth, Data: body}); err != nil {
		return err
	}

	if err := l.machine.Transition(agent.StateRegistering); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case msg, ok := <-l.transport.Receive():
		if !ok {
			return ErrAuthRejected
		}
		switch msg.Type {
		case comms.TypeAuthOK:
			if err := json.Unmarshal(msg.Data, &l.identity); err != nil {
				return err
			}
			l.log.Info("registered", "uuid", l.identity.UUID, "empresa", l.identity.Empresa)
			return l.machine.Transition(agent.StateConnected)
		case comms.TypeAuthErr:
			return ErrAuthRejected
		default:
			return ErrAuthRejected
		}
	}
}

// serve consumes inbound envelopes and dispatches jobs until shutdown.
func (l *Lifecycle) serve(ctx context.Context) error {
	inbound := l.transport.Receive()
	for {
		select {
		case <-ctx.Done():
			return l.Shutdown()
		case msg, ok := <-inbound:
			if !ok {
				return l.machine.Transition(agent.StateDisconnected)
			}
			if msg.Type != comms.TypeJob {
				l.log.Debug("ignoring non-job message", "type", string(msg.Type))
				continue
			}
			var j job.Job
			if err := json.Unmarshal(msg.Data, &j); err != nil {
				l.log.Error("cannot decode job", "err", err)
				continue
			}
			if err := l.dispatcher.Dispatch(ctx, j); err != nil {
				l.log.Error("job failed", "id", j.ID, "err", err)
			}
		}
	}
}

// Shutdown transitions to SHUTTING_DOWN and closes the transport.
func (l *Lifecycle) Shutdown() error {
	if err := l.machine.Transition(agent.StateShuttingDown); err != nil {
		l.log.Warn("shutdown transition rejected", "from", string(l.machine.Current()))
	}
	l.log.Info("shutting down")
	return l.transport.Close()
}
