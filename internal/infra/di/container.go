// Package di is the composition root: the single place where concrete adapters
// are constructed and wired to the core. Nothing else imports adapters, which
// keeps the dependency direction pointing inward. Owner: Go Core Engineer.
package di

import (
	"os"

	"github.com/teraerp/tera-agent/internal/adapters/communication/local"
	"github.com/teraerp/tera-agent/internal/adapters/communication/websocket"
	"github.com/teraerp/tera-agent/internal/adapters/config"
	"github.com/teraerp/tera-agent/internal/adapters/logger"
	"github.com/teraerp/tera-agent/internal/adapters/printing/binarizer"
	driverfile "github.com/teraerp/tera-agent/internal/adapters/printing/driver/file"
	"github.com/teraerp/tera-agent/internal/adapters/printing/encoder/escpos"
	"github.com/teraerp/tera-agent/internal/adapters/printing/platform"
	"github.com/teraerp/tera-agent/internal/adapters/printing/profile"
	"github.com/teraerp/tera-agent/internal/adapters/printing/rasterizer/poppler"
	"github.com/teraerp/tera-agent/internal/adapters/printing/renderer"
	"github.com/teraerp/tera-agent/internal/app/dispatcher"
	"github.com/teraerp/tera-agent/internal/app/lifecycle"
	"github.com/teraerp/tera-agent/internal/app/ports"
	appprint "github.com/teraerp/tera-agent/internal/app/print"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	"github.com/teraerp/tera-agent/internal/domain/comms"
	"github.com/teraerp/tera-agent/internal/domain/job"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// App bundles the wired components main needs to run and observe the Agent.
type App struct {
	Machine   *agent.Machine
	Lifecycle *lifecycle.Lifecycle
	Log       ports.Logger
	Transport comms.Transport
	Profiles  dp.ProfileCache
	Engine    *appprint.Engine
	Discovery dp.Discovery
	// LocalMode is true when no backend URL is configured.
	LocalMode bool
	// HTTPAddr/HTTPToken configure the optional local HTTP print service.
	HTTPAddr  string
	HTTPToken string
}

// PrintOptions tune the print engine wiring for a command.
type PrintOptions struct {
	Density int    // 1..5 (3 = normal); 0 = normal
	OutFile string // when set, encoded output is written here instead of printing
}

// Build reads configuration and wires the full object graph for `run`. A missing
// config file is not an error: the Agent starts in local mode with defaults.
func Build(configPath string) (*App, error) {
	cfg, err := config.New(configPath).Load()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		cfg = ports.Config{LogLevel: "info"}
	}

	log := logger.New(cfg.LogLevel)
	machine := agent.NewMachine()

	local := cfg.BackendURL == ""
	var transport comms.Transport
	if local {
		transport = localTransport(log)
	} else {
		transport = websocket.New(cfg, log)
	}

	engine, profiles := newEngine(log, platform.Drivers(log), 0)

	disp := dispatcher.New(log)
	disp.Register(job.KindPrint, appprint.NewJobHandler(engine))

	lc := lifecycle.New(machine, transport, disp, cfg, log)

	return &App{
		Machine:   machine,
		Lifecycle: lc,
		Log:       log,
		Transport: transport,
		Profiles:  profiles,
		Engine:    engine,
		Discovery: platform.NewDiscovery(log),
		LocalMode: local,
		HTTPAddr:  cfg.HTTPAddr,
		HTTPToken: cfg.HTTPToken,
	}, nil
}

// BuildPrinting wires the print engine, profile cache and discovery for the CLI
// print/printers/raw/text commands (no backend needed).
func BuildPrinting(log ports.Logger, opts PrintOptions) (*appprint.Engine, dp.ProfileCache, dp.Discovery) {
	drivers := platform.Drivers(log)
	if opts.OutFile != "" {
		drivers = []dp.Driver{driverfile.New(opts.OutFile, log)}
	}
	engine, profiles := newEngine(log, drivers, densityBias(opts.Density))
	return engine, profiles, platform.NewDiscovery(log)
}

// RawDriver returns the platform raw driver (for the `raw` command).
func RawDriver(log ports.Logger) dp.Driver { return platform.RawDriver(log) }

// OSName returns the running operating system name.
func OSName() string { return platform.OSName() }

// newEngine assembles renderers, encoders and the resolver over the given drivers.
func newEngine(log ports.Logger, drivers []dp.Driver, bias int) (*appprint.Engine, dp.ProfileCache) {
	var bin dp.Binarizer
	if bias == 0 {
		bin = binarizer.NewOtsu()
	} else {
		bin = binarizer.NewOtsuBias(bias)
	}
	raster := poppler.New(log)

	renderers := []dp.Renderer{
		renderer.NewPDF(raster),
		renderer.NewImage(),
		renderer.NewText(),
	}
	encoders := []dp.Encoder{
		escpos.NewRaster(bin),
		escpos.NewText(),
	}

	profiles := profile.NewMemoryCache()
	engine := appprint.NewEngine(appprint.NewResolver(renderers, encoders, drivers), profiles, log)
	return engine, profiles
}

// densityBias maps a 1..5 density level to an Otsu threshold bias.
func densityBias(density int) int {
	if density == 0 {
		return 0
	}
	return (density - 3) * 20
}

func localTransport(log ports.Logger) comms.Transport { return local.New(log) }
