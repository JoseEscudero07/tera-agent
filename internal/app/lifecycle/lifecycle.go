// Package lifecycle orchestrates the Agent's connection to the Backend: it runs
// the protocol session (handshake, registration, capabilities, profile sync,
// heartbeat and job handling) and drives the domain state machine, reconnecting
// with backoff. It depends only on ports/domain interfaces. Owner: Communication
// Engineer (session) + Go Core Engineer (state machine).
package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/teraerp/tera-agent/internal/app/dispatcher"
	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	"github.com/teraerp/tera-agent/internal/domain/comms"
	"github.com/teraerp/tera-agent/internal/domain/job"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

const (
	authTimeout   = 30 * time.Second
	maxBackoff    = 60 * time.Second
	defaultHeartb = 30 * time.Second
)

// Lifecycle runs the protocol session over a transport it creates per attempt.
type Lifecycle struct {
	newTransport func() comms.Transport
	machine      *agent.Machine
	dispatcher   *dispatcher.Dispatcher
	discovery    dp.Discovery
	profiles     dp.ProfileCache
	cfg          ports.Config
	log          ports.Logger
	version      string

	started   time.Time
	mu        sync.Mutex
	processed map[string]bool // job idempotency
}

// New constructs a Lifecycle. newTransport returns a fresh transport per connect
// so the session can reconnect.
func New(
	newTransport func() comms.Transport,
	machine *agent.Machine,
	disp *dispatcher.Dispatcher,
	discovery dp.Discovery,
	profiles dp.ProfileCache,
	cfg ports.Config,
	log ports.Logger,
	version string,
) *Lifecycle {
	return &Lifecycle{
		newTransport: newTransport,
		machine:      machine,
		dispatcher:   disp,
		discovery:    discovery,
		profiles:     profiles,
		cfg:          cfg,
		log:          log,
		version:      version,
		processed:    make(map[string]bool),
	}
}

// Run connects and serves until ctx is cancelled, reconnecting with backoff.
func (l *Lifecycle) Run(ctx context.Context) error {
	l.started = time.Now()
	backoff := time.Second
	for {
		err := l.runOnce(ctx)
		if ctx.Err() != nil {
			l.setState(agent.StateShuttingDown)
			return nil
		}
		l.log.Warn("connection lost; reconnecting", "err", err, "retry_in", backoff.String())
		l.setState(agent.StateReconnecting)
		select {
		case <-ctx.Done():
			l.setState(agent.StateShuttingDown)
			return nil
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (l *Lifecycle) runOnce(ctx context.Context) error {
	l.setState(agent.StateConnecting)
	t := l.newTransport()
	if err := t.Connect(ctx); err != nil {
		l.setState(agent.StateDisconnected)
		return err
	}
	defer func() {
		_ = t.Close()
		l.setState(agent.StateDisconnected)
	}()

	in := t.Receive()
	if err := l.authenticate(ctx, t, in); err != nil {
		return err
	}
	l.register(ctx, t)
	l.setState(agent.StateConnected)
	l.log.Info("connected to backend", "url", l.cfg.BackendURL)
	return l.serve(ctx, t, in)
}

func (l *Lifecycle) authenticate(ctx context.Context, t comms.Transport, in <-chan []byte) error {
	l.setState(agent.StateAuthenticating)
	host, _ := os.Hostname()

	if err := l.send(ctx, t, comms.Hello{
		Type: comms.TypeHello, AgentVersion: l.version, ProtocolVersion: comms.ProtocolVersion,
		InstallationID: l.cfg.AgentID, MachineID: host,
	}); err != nil {
		return err
	}
	if err := l.send(ctx, t, comms.Authenticate{Type: comms.TypeAuthenticate, Token: l.cfg.Token}); err != nil {
		return err
	}

	timeout := time.After(authTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("authentication timeout")
		case raw, ok := <-in:
			if !ok {
				return errors.New("connection closed during authentication")
			}
			switch typeOf(raw) {
			case comms.TypeAuthenticated:
				var m comms.Authenticated
				_ = json.Unmarshal(raw, &m)
				l.log.Info("authenticated", "agent_id", m.AgentID, "company", m.CompanyID, "branch", m.BranchID)
				return nil
			case comms.TypeAuthError:
				var m comms.AuthError
				_ = json.Unmarshal(raw, &m)
				l.setState(agent.StateError)
				return fmt.Errorf("authentication rejected: %s", m.Error.Code)
			}
		}
	}
}

func (l *Lifecycle) register(ctx context.Context, t comms.Transport) {
	l.setState(agent.StateRegistering)
	host, _ := os.Hostname()
	_ = l.send(ctx, t, comms.Register{
		Type: comms.TypeRegister, Hostname: host, OS: runtime.GOOS, AgentVersion: l.version,
	})

	printers, _ := l.discovery.List(ctx)
	dtos := make([]comms.PrinterDTO, 0, len(printers))
	for _, p := range printers {
		dtos = append(dtos, comms.PrinterDTO{Name: p.Name, Driver: p.Driver, Type: p.Driver})
	}
	_ = l.send(ctx, t, comms.Capabilities{Type: comms.TypeCapabilities, Devices: []string{"printer"}, Printers: dtos})
}

func (l *Lifecycle) serve(ctx context.Context, t comms.Transport, in <-chan []byte) error {
	interval := l.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = defaultHeartb
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			l.sendHeartbeat(ctx, t)
		case raw, ok := <-in:
			if !ok {
				return errors.New("connection closed")
			}
			l.handle(ctx, t, raw, ticker)
		}
	}
}

func (l *Lifecycle) handle(ctx context.Context, t comms.Transport, raw []byte, ticker *time.Ticker) {
	switch typeOf(raw) {
	case comms.TypeProfilesSync:
		var m comms.ProfilesSync
		if err := json.Unmarshal(raw, &m); err != nil {
			return
		}
		l.profiles.SetAll(mapProfiles(m.Profiles))
		l.log.Info("printer profiles synced", "count", len(m.Profiles), "version", m.Version)
		_ = l.send(ctx, t, comms.ProfilesAck{Type: comms.TypeProfilesAck, Version: m.Version})

	case comms.TypeProfileUpdate:
		var m comms.ProfileUpdate
		if err := json.Unmarshal(raw, &m); err == nil {
			l.profiles.Set(mapProfile(m.Profile))
		}

	case comms.TypeConfig:
		var m comms.Config
		if err := json.Unmarshal(raw, &m); err == nil && m.HeartbeatSeconds > 0 {
			ticker.Reset(time.Duration(m.HeartbeatSeconds) * time.Second)
			l.log.Debug("heartbeat interval updated", "seconds", m.HeartbeatSeconds)
		}

	case comms.TypeJob:
		var m comms.Job
		if err := json.Unmarshal(raw, &m); err == nil {
			l.handleJob(ctx, t, m)
		}

	case comms.TypeError:
		l.log.Warn("server error message", "raw", string(raw))

	default:
		l.log.Debug("ignoring message", "type", typeOf(raw))
	}
}

func (l *Lifecycle) handleJob(ctx context.Context, t comms.Transport, m comms.Job) {
	_ = l.send(ctx, t, comms.JobReceived{Type: comms.TypeJobReceived, ID: m.ID})

	l.mu.Lock()
	dup := l.processed[m.ID]
	l.mu.Unlock()
	if dup {
		l.log.Warn("duplicate job ignored", "id", m.ID)
		_ = l.send(ctx, t, comms.JobCompleted{Type: comms.TypeJobCompleted, ID: m.ID})
		return
	}

	kind := job.KindPrint
	if m.JobType == "device" {
		kind = job.KindDevice
	}
	start := time.Now()
	err := l.dispatcher.Dispatch(ctx, job.Job{ID: m.ID, Kind: kind, Payload: []byte(m.Payload), Received: start})
	if err != nil {
		l.log.Error("job failed", "id", m.ID, "err", err)
		_ = l.send(ctx, t, comms.JobFailed{
			Type: comms.TypeJobFailed, ID: m.ID,
			Error: comms.ErrorBody{Code: "JOB_FAILED", Message: err.Error()},
		})
		return
	}
	l.mu.Lock()
	l.processed[m.ID] = true
	l.mu.Unlock()
	_ = l.send(ctx, t, comms.JobCompleted{
		Type: comms.TypeJobCompleted, ID: m.ID, DurationMS: time.Since(start).Milliseconds(),
	})
}

func (l *Lifecycle) sendHeartbeat(ctx context.Context, t comms.Transport) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	_ = l.send(ctx, t, comms.Heartbeat{
		Type: comms.TypeHeartbeat, State: string(l.machine.Current()),
		UptimeS: int64(time.Since(l.started).Seconds()), MemBytes: ms.Alloc,
		Version: l.version, PendingJobs: 0,
	})
}

func (l *Lifecycle) send(ctx context.Context, t comms.Transport, msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return t.Send(ctx, b)
}

func (l *Lifecycle) setState(s agent.State) {
	if err := l.machine.Transition(s); err != nil {
		l.log.Debug("state transition skipped", "to", string(s), "err", err)
	}
}

func typeOf(raw []byte) string {
	var x comms.Typed
	_ = json.Unmarshal(raw, &x)
	return x.Type
}

func mapProfiles(dtos []comms.ProfileDTO) []dp.PrinterProfile {
	out := make([]dp.PrinterProfile, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, mapProfile(d))
	}
	return out
}

func mapProfile(d comms.ProfileDTO) dp.PrinterProfile {
	formats := make([]dp.DeviceFormat, 0, len(d.NativeFormats))
	for _, f := range d.NativeFormats {
		formats = append(formats, dp.DeviceFormat(f))
	}
	if len(formats) == 0 {
		formats = []dp.DeviceFormat{dp.DeviceESCPOS}
	}
	return dp.PrinterProfile{
		PrinterID:      d.PrinterID,
		NativeFormats:  formats,
		WidthDots:      d.WidthDots,
		DPI:            d.DPI,
		SupportsCut:    d.SupportsCut,
		SupportsDrawer: d.SupportsDrawer,
		Meta:           d.Meta,
	}
}
