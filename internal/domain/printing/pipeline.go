package printing

import (
	"context"
	"image"
)

// Renderer converts input content into an intermediate Artifact. It knows
// nothing about printers or device languages.
type Renderer interface {
	CanRender(SourceFormat) bool
	Produces() ArtifactKind
	Render(ctx context.Context, content []byte, opts RenderOptions) (Artifact, error)
}

// Encoder converts an Artifact into the byte language of a device. It knows
// nothing about the transport.
type Encoder interface {
	Accepts() ArtifactKind
	Produces() DeviceFormat
	Encode(ctx context.Context, a Artifact, opts EncodeOptions) ([]byte, error)
}

// Driver only sends bytes to the device. It never renders or converts.
type Driver interface {
	Accepts(DeviceFormat) bool
	Send(ctx context.Context, printerID string, data []byte) error
}

// Rasterizer turns a PDF into raster pages. Replaceable implementation
// (Poppler today; MuPDF/PDFium/Ghostscript tomorrow) behind this interface.
type Rasterizer interface {
	Rasterize(ctx context.Context, pdf []byte, opts RasterOptions) ([]image.Image, error)
}

// Binarizer converts a grayscale/color image to a 1-bpp MonoBitmap. Strategy is
// pluggable (Otsu default, Atkinson for images, Threshold, ...). See
// docs/BINARIZATION.md.
type Binarizer interface {
	Binarize(img image.Image) *MonoBitmap
	Name() string
}

// Pipeline is a resolved (Renderer, Encoder, Driver) chain.
type Pipeline struct {
	Renderer Renderer
	Encoder  Encoder
	Driver   Driver
}

// Resolver selects a Pipeline by capability composition (no per-format
// conditionals). Registering a new component makes new paths available.
type Resolver interface {
	Resolve(format SourceFormat, profile PrinterProfile) (Pipeline, error)
}
