package print

import (
	"context"
	"fmt"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// passthroughRenderer wraps the input bytes unchanged when the source is already
// a device-native format (e.g. PDF to a printer that consumes PDF).
type passthroughRenderer struct{ format dp.DeviceFormat }

func (passthroughRenderer) CanRender(dp.SourceFormat) bool { return true }
func (passthroughRenderer) Produces() dp.ArtifactKind      { return dp.ArtifactBytes }

func (p passthroughRenderer) Render(_ context.Context, content []byte, _ dp.RenderOptions) (dp.Artifact, error) {
	return dp.BytesArtifact{Data: content, Format: p.format}, nil
}

// passthroughEncoder emits the pass-through bytes as-is.
type passthroughEncoder struct{ format dp.DeviceFormat }

func (p passthroughEncoder) Accepts() dp.ArtifactKind  { return dp.ArtifactBytes }
func (p passthroughEncoder) Produces() dp.DeviceFormat { return p.format }

func (p passthroughEncoder) Encode(_ context.Context, a dp.Artifact, _ dp.EncodeOptions) ([]byte, error) {
	b, ok := a.(dp.BytesArtifact)
	if !ok {
		return nil, fmt.Errorf("passthrough: expected bytes artifact, got %s", a.Kind())
	}
	return b.Data, nil
}
