package print

import (
	"fmt"
	"strings"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// resolver composes a pipeline by matching component capabilities. Adding a
// Renderer/Encoder/Driver makes new source->device paths available with no
// changes here. There is no per-format ("if PDF") logic.
type resolver struct {
	renderers []dp.Renderer
	encoders  []dp.Encoder
	drivers   []dp.Driver
}

// NewResolver builds a capability resolver over the given component registries.
func NewResolver(renderers []dp.Renderer, encoders []dp.Encoder, drivers []dp.Driver) dp.Resolver {
	return &resolver{renderers: renderers, encoders: encoders, drivers: drivers}
}

// Resolve tries each device format the printer accepts (in priority order) and
// composes: driver(target) <- encoder(produces target) <- renderer(produces the
// encoder's input kind and can render the source). A pass-through pipeline is
// used when the source is already the target device format.
//
// Cuando no hay match, el error explica qué se probó y qué faltó (driver del
// dispositivo, encoder del formato, renderer del origen). Ese mensaje sube tal
// cual al toast de la UI, así que evita mostrar "no pipeline" pelado.
func (r *resolver) Resolve(format dp.SourceFormat, profile dp.PrinterProfile) (dp.Pipeline, error) {
	if len(profile.NativeFormats) == 0 {
		return dp.Pipeline{}, fmt.Errorf(
			"impresora %q sin formatos nativos configurados (¿el Backend envió el perfil?)",
			profile.PrinterID,
		)
	}

	var reasons []string
	for _, target := range profile.NativeFormats {
		driver := r.driverFor(target)
		if driver == nil {
			reasons = append(reasons, fmt.Sprintf(
				"formato %q: no hay driver en esta plataforma", target,
			))
			continue
		}

		if dp.SourceIsDevice(format, target) {
			return dp.Pipeline{
				Renderer: passthroughRenderer{format: target},
				Encoder:  passthroughEncoder{format: target},
				Driver:   driver,
			}, nil
		}

		encoderFound := false
		for _, enc := range r.encoders {
			if enc.Produces() != target {
				continue
			}
			encoderFound = true
			for _, ren := range r.renderers {
				if ren.CanRender(format) && ren.Produces() == enc.Accepts() {
					return dp.Pipeline{Renderer: ren, Encoder: enc, Driver: driver}, nil
				}
			}
		}
		if !encoderFound {
			reasons = append(reasons, fmt.Sprintf(
				"formato %q: no hay encoder que lo produzca", target,
			))
		} else {
			reasons = append(reasons, fmt.Sprintf(
				"formato %q: ningún renderer sabe convertir %q a lo que espera el encoder",
				target, format,
			))
		}
	}

	return dp.Pipeline{}, fmt.Errorf(
		"no puedo imprimir %q en %q: %s",
		format, profile.PrinterID, strings.Join(reasons, "; "),
	)
}

func (r *resolver) driverFor(f dp.DeviceFormat) dp.Driver {
	for _, d := range r.drivers {
		if d.Accepts(f) {
			return d
		}
	}
	return nil
}
