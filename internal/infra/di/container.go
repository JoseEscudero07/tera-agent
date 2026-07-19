// Package di is the composition root: the single place where concrete adapters
// are constructed and wired to the core. Nothing else imports adapters, which
// keeps the dependency direction pointing inward. Owner: Go Core Engineer;
// wiring of new adapters is approved by the Software Architect.
package di

import (
	"github.com/teraerp/tera-agent/internal/adapters/communication/websocket"
	"github.com/teraerp/tera-agent/internal/adapters/config"
	"github.com/teraerp/tera-agent/internal/adapters/logger"
	"github.com/teraerp/tera-agent/internal/adapters/printing"
	"github.com/teraerp/tera-agent/internal/app/dispatcher"
	"github.com/teraerp/tera-agent/internal/app/lifecycle"
	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	"github.com/teraerp/tera-agent/internal/domain/job"
)

// App bundles the wired components main needs to run and observe the Agent.
type App struct {
	Machine   *agent.Machine
	Lifecycle *lifecycle.Lifecycle
	Log       ports.Logger
}

// Build reads configuration from configPath and wires the full object graph.
func Build(configPath string) (*App, error) {
	cfg, err := config.New(configPath).Load()
	if err != nil {
		return nil, err
	}

	log := logger.New(cfg.LogLevel)
	machine := agent.NewMachine()

	transport := websocket.New(cfg, log)
	disp := dispatcher.New(log)

	// Printing: wire the platform Port and register the PRINT job handler.
	printPort, _ := printing.New(log)
	disp.Register(job.KindPrint, printing.NewHandler(printPort, log))
	// Device handlers are registered here once implemented and approved:
	//   disp.Register(job.KindDevice, device.NewHandler(...))

	lc := lifecycle.New(machine, transport, disp, cfg, log)

	return &App{Machine: machine, Lifecycle: lc, Log: log}, nil
}
