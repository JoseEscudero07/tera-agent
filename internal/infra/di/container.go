// Package di is the composition root: the single place where concrete adapters
// are constructed and wired to the core. Nothing else imports adapters, which
// keeps the dependency direction pointing inward. Owner: Go Core Engineer.
package di

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/teraerp/tera-agent/internal/adapters/communication/local"
	"github.com/teraerp/tera-agent/internal/adapters/communication/websocket"
	"github.com/teraerp/tera-agent/internal/adapters/config"
	"github.com/teraerp/tera-agent/internal/adapters/logger"
	"github.com/teraerp/tera-agent/internal/adapters/printing/binarizer"
	driverfile "github.com/teraerp/tera-agent/internal/adapters/printing/driver/file"
	"github.com/teraerp/tera-agent/internal/adapters/printing/encoder/escpos"
	encraster "github.com/teraerp/tera-agent/internal/adapters/printing/encoder/raster"
	"github.com/teraerp/tera-agent/internal/adapters/printing/platform"
	"github.com/teraerp/tera-agent/internal/adapters/printing/profile"
	"github.com/teraerp/tera-agent/internal/adapters/printing/rasterizer/poppler"
	"github.com/teraerp/tera-agent/internal/adapters/printing/renderer"
	"github.com/teraerp/tera-agent/internal/adapters/store"
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
	Machine      *agent.Machine
	Lifecycle    *lifecycle.Lifecycle
	Log          ports.Logger
	NewTransport func() comms.Transport
	Profiles     dp.ProfileCache
	Engine       *appprint.Engine
	Discovery    dp.Discovery
	// Roles resuelve `role → printer` para jobs automáticos del backend que
	// llegan sin printer_id. La UI lo refresca al cambiar impresoras.
	Roles *appprint.RoleResolver
	// LocalMode is true when no backend URL is configured.
	LocalMode bool
	// HTTPAddr/HTTPToken configure the optional local HTTP print service.
	HTTPAddr  string
	HTTPToken string
	// DataDir is where runtime state and (optionally) logs live.
	DataDir string
	// Info holds the identity learned from the Backend (for the UI).
	Info *agent.Info
	// Cfg is the loaded configuration (for the UI).
	Cfg ports.Config
	// SaveConfig persists configuration changes made from the UI.
	SaveConfig func(ports.Config) error
	// SetPageMargin reinstala el resolutor de margen de página en los drivers que
	// lo soportan, para que el panel pueda cambiarlo sin reiniciar el Agent.
	SetPageMargin func(func(printerID string) float64)
	// SetPageMode reinstala el resolutor del modo vectorial/imagen de las
	// impresoras de página; aplica desde el siguiente trabajo.
	SetPageMode func(func(printerID string) ports.PageMode)
}

// pageModes guarda el resolutor vigente del modo de impresión de cada impresora
// de página. El panel lo sustituye al guardar mientras los trabajos lo leen desde
// otras goroutines, de ahí el atomic.
type pageModes struct {
	fn atomic.Pointer[func(string) ports.PageMode]
}

func newPageModes(fn func(string) ports.PageMode) *pageModes {
	m := &pageModes{}
	m.Set(fn)
	return m
}

// Set instala fn; nil vuelve al modo por defecto para todas.
func (m *pageModes) Set(fn func(string) ports.PageMode) { m.fn.Store(&fn) }

// Of devuelve el modo de printerID.
func (m *pageModes) Of(printerID string) ports.PageMode {
	if fn := m.fn.Load(); fn != nil && *fn != nil {
		return (*fn)(printerID)
	}
	return ports.PageModeVector
}

// pageMarginSetter lo implementan los drivers que colocan la página respecto al
// borde físico del papel (hoy sólo el GDI de Windows).
//
// Se declara aquí, en el punto de composición, y no en el dominio: "margen de
// página" es un detalle de las impresoras de hoja, y meterlo en el puerto Driver
// obligaría a implementarlo a la térmica y al driver de fichero, que no tienen
// papel físico que medir.
type pageMarginSetter interface {
	SetPageMargin(func(printerID string) float64)
}

// applyPageMargin instala fn en todos los drivers que sepan usarla.
func applyPageMargin(drivers []dp.Driver, fn func(printerID string) float64) {
	for _, d := range drivers {
		if s, ok := d.(pageMarginSetter); ok {
			s.SetPageMargin(fn)
		}
	}
}

// pageMarginOf adapta Config.PageMargin al resolutor que instala el driver.
func pageMarginOf(cfg ports.Config) func(string) float64 {
	return func(printerID string) float64 {
		return cfg.PageMargin(printerID)
	}
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

	log, err := logger.NewWithFile(cfg.LogLevel, cfg.LogFile, cfg.LogMaxSizeMB, cfg.LogMaxBackups)
	if err != nil {
		return nil, err
	}
	machine := agent.NewMachine()
	info := agent.NewInfo()
	isLocal := cfg.BackendURL == ""

	dd := dataDir(cfg)
	jobStore, err := store.New(filepath.Join(dd, "jobs.json"))
	if err != nil {
		return nil, err
	}

	disc := platform.NewDiscovery(log)
	bin := newBinarizer(0)
	drivers := platform.Drivers(log, bin)
	modes := newPageModes(cfg.PageModeOf)
	engine, profiles := newEngine(log, drivers, bin, modes.Of)
	// Per-printer cut calibration from config (feed before cut, top margin), so a
	// client can tune the cut per printer without recompiling.
	engine.SetTuning(cfg.PrinterTuning)
	// Margen de página por impresora (impresoras de hoja: láser/inyección).
	applyPageMargin(drivers, pageMarginOf(cfg))

	roles := appprint.NewRoleResolver(cfg.Printers, cfg.DefaultPrinter)

	disp := dispatcher.New(log)
	disp.Register(job.KindPrint, appprint.NewJobHandler(engine, roles))

	var tlsCfg *tls.Config
	if cfg.InsecureSkipVerify {
		tlsCfg = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for dev/self-signed
	}
	newTransport := func() comms.Transport {
		if isLocal {
			return local.New(log)
		}
		return websocket.New(cfg.BackendURL, log, tlsCfg)
	}

	lc := lifecycle.New(newTransport, machine, disp, disc, profiles, jobStore, info, cfg, log, AgentVersion)

	return &App{
		Machine:      machine,
		Lifecycle:    lc,
		Log:          log,
		NewTransport: newTransport,
		Profiles:     profiles,
		Engine:       engine,
		Discovery:    disc,
		Roles:        roles,
		LocalMode:    isLocal,
		HTTPAddr:     cfg.HTTPAddr,
		HTTPToken:    cfg.HTTPToken,
		DataDir:      dd,
		Info:         info,
		Cfg:          cfg,
		SaveConfig:   func(c ports.Config) error { return config.New(configPath).Save(c) },
		SetPageMargin: func(fn func(printerID string) float64) {
			applyPageMargin(drivers, fn)
		},
		SetPageMode: modes.Set,
	}, nil
}

// AgentVersion is reported to the Backend in hello/register/heartbeat, shown in
// the panel and by the `version` command. Es var (no const) para que el build de
// release lo inyecte:
//
//	go build -ldflags "-X github.com/teraerp/tera-agent/internal/infra/di.AgentVersion=1.3.0" ./cmd/tera-agent
//
// El default "0.0.0-dev" marca claramente los binarios compilados sin release.
var AgentVersion = "0.0.0-dev"

// dataDir resolves where the Agent persists runtime state (job dedup/pending).
func dataDir(cfg ports.Config) string {
	if cfg.DataDir != "" {
		return cfg.DataDir
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "tera-agent")
	}
	return "tera-agent-data"
}

// BuildPrinting wires the print engine, profile cache and discovery for the CLI
// print/printers/raw/text commands (no backend needed).
func BuildPrinting(log ports.Logger, opts PrintOptions) (*appprint.Engine, dp.ProfileCache, dp.Discovery) {
	bin := newBinarizer(densityBias(opts.Density))
	drivers := platform.Drivers(log, bin)
	if opts.OutFile != "" {
		drivers = []dp.Driver{driverfile.New(opts.OutFile, log)}
	}
	// La CLI no tiene config de impresoras gestionadas: modo por defecto.
	engine, profiles := newEngine(log, drivers, bin, nil)
	return engine, profiles, platform.NewDiscovery(log)
}

// RawDriver returns the platform raw driver (for the `raw` command).
func RawDriver(log ports.Logger) dp.Driver { return platform.RawDriver(log) }

// OSName returns the running operating system name.
func OSName() string { return platform.OSName() }

// newBinarizer builds the 1-bpp strategy from a density bias (0 = default Otsu).
// Shared by the ESC/POS raster encoder and the Windows GDI driver, so both
// binarize identically.
func newBinarizer(bias int) dp.Binarizer {
	if bias == 0 {
		return binarizer.NewOtsu()
	}
	return binarizer.NewOtsuBias(bias)
}

// newEngine assembles renderers, encoders and the resolver over the given
// drivers. bin is the shared binarizer (also injected into the drivers). modeOf
// resuelve el modo vectorial/imagen de las impresoras de página (nil = vectorial).
func newEngine(log ports.Logger, drivers []dp.Driver, bin dp.Binarizer, modeOf func(string) ports.PageMode) (*appprint.Engine, dp.ProfileCache) {
	raster := poppler.New(log)

	renderers := []dp.Renderer{
		renderer.NewPDF(raster),
		renderer.NewImage(),
		renderer.NewText(),
	}
	encoders := []dp.Encoder{
		escpos.NewRaster(bin),
		escpos.NewText(),
		// raster.NewMultiPNG empaqueta páginas raster para el driver GDI de
		// Windows. En Linux/macOS existe el encoder pero el driver GDI es un
		// stub que rechaza todo, así que el resolver no lo elige nunca allí.
		encraster.NewMultiPNG(),
	}

	// El cache envuelve el memoryCache con normalización específica de plataforma:
	// en Windows los perfiles "PDF nativo" del Backend (láser/inyección) se
	// traducen al leerlos según el modo de cada impresora: vectorial (pdftocairo)
	// o imagen (GDI raster). En Linux/mac ProfileNormalizerForOS devuelve nil y el
	// wrapping no aplica.
	profiles := platform.NormalizingProfileCache(profile.NewMemoryCache(), platform.ProfileNormalizerForOS(modeOf))
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
