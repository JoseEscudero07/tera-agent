package printing

import "image"

// ArtifactKind identifies the intermediate representation a Renderer produces
// and an Encoder consumes. This is the currency that decouples the two stages
// (composition over inheritance).
type ArtifactKind string

const (
	ArtifactRaster ArtifactKind = "raster"
	ArtifactText   ArtifactKind = "text"
	ArtifactBytes  ArtifactKind = "bytes" // pass-through, already device-ready
)

// Artifact is the intermediate representation between Renderer and Encoder.
type Artifact interface{ Kind() ArtifactKind }

// RasterArtifact is one or more raster pages at a target width in dots.
type RasterArtifact struct {
	Pages     []image.Image
	WidthDots int
}

func (RasterArtifact) Kind() ArtifactKind { return ArtifactRaster }

// TextArtifact is plain text to be encoded by a text-capable encoder.
type TextArtifact struct{ Body string }

func (TextArtifact) Kind() ArtifactKind { return ArtifactText }

// BytesArtifact carries already device-ready bytes (pass-through pipeline).
type BytesArtifact struct {
	Data   []byte
	Format DeviceFormat
}

func (BytesArtifact) Kind() ArtifactKind { return ArtifactBytes }

// MonoBitmap is a 1-bpp raster produced by a Binarizer. Layout: row-major,
// Stride() bytes per row, MSB (0x80) is the leftmost pixel, bit=1 => black dot.
// This matches the ESC/POS GS v 0 raster format exactly.
type MonoBitmap struct {
	Width  int
	Height int
	Bits   []byte
}

// Stride is the number of bytes per row.
func (m *MonoBitmap) Stride() int { return (m.Width + 7) / 8 }
