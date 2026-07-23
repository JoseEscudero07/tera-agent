// Package renderer holds the input Renderers (PDF, image, text) that convert
// content into intermediate Artifacts. Renderers know nothing about printers.
// Owner: Printing Engineer.
package renderer

import (
	"bytes"
	"context"
	"image"

	// Register decoders for the Image renderer.
	_ "image/jpeg"
	_ "image/png"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// PDF renders a PDF to raster pages using an injected Rasterizer.
type PDF struct{ raster dp.Rasterizer }

// NewPDF returns a PDF renderer backed by the given rasterizer.
func NewPDF(raster dp.Rasterizer) dp.Renderer { return &PDF{raster: raster} }

func (PDF) CanRender(f dp.SourceFormat) bool { return f == dp.FormatPDF }
func (PDF) Produces() dp.ArtifactKind        { return dp.ArtifactRaster }

func (p *PDF) Render(ctx context.Context, content []byte, opts dp.RenderOptions) (dp.Artifact, error) {
	pages, err := p.raster.Rasterize(ctx, content, dp.RasterOptions{
		WidthDots: opts.WidthDots,
		DPI:       opts.DPI,
		// Gris salvo que el destino pida color (impresoras a color por GDI). El
		// binarizador del camino térmico convierte a gris igualmente, así que
		// esto sólo abre la puerta al color donde tiene sentido.
		Gray: !opts.Color,
	})
	if err != nil {
		return nil, err
	}
	return dp.RasterArtifact{Pages: pages, WidthDots: opts.WidthDots}, nil
}

// Image renders PNG/JPEG bytes into a single raster page.
type Image struct{}

// NewImage returns an image renderer.
func NewImage() dp.Renderer { return Image{} }

func (Image) CanRender(f dp.SourceFormat) bool { return f == dp.FormatPNG || f == dp.FormatJPEG }
func (Image) Produces() dp.ArtifactKind        { return dp.ArtifactRaster }

func (Image) Render(_ context.Context, content []byte, opts dp.RenderOptions) (dp.Artifact, error) {
	img, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	return dp.RasterArtifact{Pages: []image.Image{img}, WidthDots: opts.WidthDots}, nil
}

// Text renders plain text into a text artifact.
type Text struct{}

// NewText returns a text renderer.
func NewText() dp.Renderer { return Text{} }

func (Text) CanRender(f dp.SourceFormat) bool { return f == dp.FormatText }
func (Text) Produces() dp.ArtifactKind        { return dp.ArtifactText }

func (Text) Render(_ context.Context, content []byte, _ dp.RenderOptions) (dp.Artifact, error) {
	return dp.TextArtifact{Body: string(content)}, nil
}
