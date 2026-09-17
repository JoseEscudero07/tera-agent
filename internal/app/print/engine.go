// Package print is the application-layer print engine: it receives a PrintJob,
// asks the Resolver for a Pipeline (by capability), and runs the stages
// Renderer -> Encoder -> Driver. It depends only on domain ports.
// Owner: Go Core Engineer (orchestration) + Printing Engineer (components).
package print

import (
	"context"
	"sync"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Engine orchestrates a print job through its resolved pipeline.
type Engine struct {
	resolver dp.Resolver
	profiles dp.ProfileProvider
	log      ports.Logger
	// tuning resolves per-printer cut calibration (feed before cut, top margin).
	// Optional; nil means the encoder uses its built-in defaults. Guarded by mu
	// because the UI can swap it at runtime (SetTuning) while jobs are printing.
	mu     sync.RWMutex
	tuning func(printerID string) (cutFeedDots, topMarginDots int)
}

// NewEngine wires the engine with a resolver and a profile provider.
func NewEngine(resolver dp.Resolver, profiles dp.ProfileProvider, log ports.Logger) *Engine {
	return &Engine{resolver: resolver, profiles: profiles, log: log}
}

// SetTuning installs a per-printer cut-calibration resolver (from config), so
// clients can tune the cut per printer without recompiling. Safe to call at
// runtime (e.g. after the panel saves a new calibration).
func (e *Engine) SetTuning(fn func(printerID string) (cutFeedDots, topMarginDots int)) {
	e.mu.Lock()
	e.tuning = fn
	e.mu.Unlock()
}

// Print resolves the printer's cached profile and executes the job with it.
func (e *Engine) Print(ctx context.Context, job dp.PrintJob) error {
	profile, err := e.profiles.Profile(job.PrinterID)
	if err != nil {
		return err
	}
	return e.PrintWithProfile(ctx, job, profile)
}

// PrintWithProfile ejecuta job con un perfil explícito, sin leer ni escribir la
// caché de perfiles. Lo usan las acciones locales (probar, cajón, POST /print),
// que resuelven el perfil con ProfileOr para no pisar el que envió el Backend.
func (e *Engine) PrintWithProfile(ctx context.Context, job dp.PrintJob, profile dp.PrinterProfile) error {
	pipe, err := e.resolver.Resolve(job.Format, profile)
	if err != nil {
		return err
	}

	artifact, err := pipe.Renderer.Render(ctx, job.Content, dp.RenderOptions{
		WidthDots: profile.WidthDots,
		DPI:       profile.DPI,
		// El camino GDI (láser/inyección/color) puede aprovechar color; el
		// térmico no. El driver GDI decide por contenido si va a 1 bpp o 24 bpp.
		Color: pipe.Encoder.Produces() == dp.DeviceGDIRaster,
	})
	if err != nil {
		return err
	}

	e.mu.RLock()
	tune := e.tuning
	e.mu.RUnlock()
	var cutFeedDots, topMarginDots int
	if tune != nil {
		cutFeedDots, topMarginDots = tune(job.PrinterID)
	}
	data, err := pipe.Encoder.Encode(ctx, artifact, dp.EncodeOptions{
		WidthDots:     profile.WidthDots,
		Cut:           profile.SupportsCut && job.Options.Cut,
		OpenDrawer:    profile.SupportsDrawer && job.Options.OpenDrawer,
		CutFeedDots:   cutFeedDots,
		TopMarginDots: topMarginDots,
	})
	if err != nil {
		return err
	}

	e.log.Info("print",
		"printer", job.PrinterID,
		"source", string(job.Format),
		"device", string(pipe.Encoder.Produces()),
		"bytes", len(data),
	)
	return pipe.Driver.Send(ctx, job.PrinterID, data)
}
