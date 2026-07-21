// Package print is the application-layer print engine: it receives a PrintJob,
// asks the Resolver for a Pipeline (by capability), and runs the stages
// Renderer -> Encoder -> Driver. It depends only on domain ports.
// Owner: Go Core Engineer (orchestration) + Printing Engineer (components).
package print

import (
	"context"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Engine orchestrates a print job through its resolved pipeline.
type Engine struct {
	resolver dp.Resolver
	profiles dp.ProfileProvider
	log      ports.Logger
	// tuning resolves per-printer cut calibration (feed before cut, top margin).
	// Optional; nil means the encoder uses its built-in defaults.
	tuning func(printerID string) (cutFeedDots, topMarginDots int)
}

// NewEngine wires the engine with a resolver and a profile provider.
func NewEngine(resolver dp.Resolver, profiles dp.ProfileProvider, log ports.Logger) *Engine {
	return &Engine{resolver: resolver, profiles: profiles, log: log}
}

// SetTuning installs a per-printer cut-calibration resolver (from config), so
// clients can tune the cut per printer without recompiling.
func (e *Engine) SetTuning(fn func(printerID string) (cutFeedDots, topMarginDots int)) {
	e.tuning = fn
}

// Print resolves and executes the pipeline for a job.
func (e *Engine) Print(ctx context.Context, job dp.PrintJob) error {
	profile, err := e.profiles.Profile(job.PrinterID)
	if err != nil {
		return err
	}

	pipe, err := e.resolver.Resolve(job.Format, profile)
	if err != nil {
		return err
	}

	artifact, err := pipe.Renderer.Render(ctx, job.Content, dp.RenderOptions{
		WidthDots: profile.WidthDots,
		DPI:       profile.DPI,
		Color:     false,
	})
	if err != nil {
		return err
	}

	var cutFeedDots, topMarginDots int
	if e.tuning != nil {
		cutFeedDots, topMarginDots = e.tuning(job.PrinterID)
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
