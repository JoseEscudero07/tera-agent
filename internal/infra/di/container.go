// Package di is the composition root: the single place where concrete adapters
// are constructed and wired to the core. Nothing else imports adapters, which
// keeps the dependency direction pointing inward. Owner: Go Core Engineer;
// wiring of new adapters is approved by the Software Architect.
package di

import (
	"github.com/teraerp/tera-agent/internal/adapters/communication/websocket"
	"github.com/teraerp/tera-agent/internal/adapters/config"
	"github.com/teraerp/tera-agent/internal/adapters/logger"
	"github.com/teraerp/tera-agent/internal/adapters/printing/binarizer"
	"github.com/teraerp/tera-agent/internal/adapters/printing/discovery"
	drivercups "github.com/teraerp/tera-agent/internal/adapters/printing/driver/cups"
	driverfile "github.com/teraerp/tera-agent/internal/adapters/printing/driver/file"
	"github.com/teraerp/tera-agent/internal/adapters/printing/encoder/escpos"
	"github.com/teraerp/tera-agent/internal/adapters/printing/profile"
	"github.com/teraerp/tera-agent/internal/adapters/printing/rasterizer/poppler"
	"github.com/teraerp/tera-agent/internal/adapters/printing/renderer"
	"github.com/teraerp/tera-agent/internal/app/dispatcher"
	"github.com/teraerp/tera-agent/internal/app/lifecycle"
	"github.com/teraerp/tera-agent/internal/app/ports"
	appprint "github.com/teraerp/tera-agent/internal/app/print"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	"github.com/teraerp/tera-agent/internal/domain/job"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// App bundles the wired components main needs to run and observe the Agent.
type App struct {
	Machine   *agent.Machine
	Lifecycle *lifecycle.Lifecycle
	Log       ports.Logger
	// Profiles is populated by the communication layer when the Backend pushes
	// printer profiles. Exposed so that layer (and tests) can seed it.
	Profiles dp.ProfileCache
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

	engine, profiles := newEngine(log, drivers(log))

	disp := dispatcher.New(log)
	disp.Register(job.KindPrint, appprint.NewJobHandler(engine))

	lc := lifecycle.New(machine, transport, disp, cfg, log)

	return &App{Machine: machine, Lifecycle: lc, Log: log, Profiles: profiles}, nil
}

// BuildPrinting wires the print engine, its profile cache and printer discovery
// without needing the agent configuration. Used by the CLI print/printers
// subcommands. drivers default to CUPS raw + native.
func BuildPrinting(log ports.Logger) (*appprint.Engine, dp.ProfileCache, dp.Discovery) {
	engine, profiles := newEngine(log, drivers(log))
	return engine, profiles, discovery.NewCUPS(log)
}

// BuildPrintingToFile wires the print engine so the encoded output is written to
// outPath instead of a printer (safe dry run, no paper used).
func BuildPrintingToFile(log ports.Logger, outPath string) (*appprint.Engine, dp.ProfileCache) {
	return newEngine(log, []dp.Driver{driverfile.New(outPath, log)})
}

// newEngine assembles renderers, encoders and the resolver over the given drivers.
func newEngine(log ports.Logger, ds []dp.Driver) (*appprint.Engine, dp.ProfileCache) {
	bin := binarizer.NewOtsu() // recommended default; see docs/BINARIZATION.md
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
	engine := appprint.NewEngine(appprint.NewResolver(renderers, encoders, ds), profiles, log)
	return engine, profiles
}

func drivers(log ports.Logger) []dp.Driver {
	return []dp.Driver{
		drivercups.NewRaw(log),
		drivercups.NewNative(log),
	}
}
