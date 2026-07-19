package escpos

import (
	"bytes"
	"context"
	"image"
	"testing"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// stubBinarizer returns a fixed 8x1 all-black bitmap regardless of input.
type stubBinarizer struct{}

func (stubBinarizer) Name() string { return "stub" }
func (stubBinarizer) Binarize(image.Image) *dp.MonoBitmap {
	return &dp.MonoBitmap{Width: 8, Height: 1, Bits: []byte{0xFF}}
}

func TestRasterEncoder_EmitsInitRasterAndCut(t *testing.T) {
	enc := NewRaster(stubBinarizer{})
	art := dp.RasterArtifact{Pages: []image.Image{image.NewGray(image.Rect(0, 0, 8, 1))}, WidthDots: 8}

	out, err := enc.Encode(context.Background(), art, dp.EncodeOptions{Cut: true})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	// ESC @ init
	if !bytes.HasPrefix(out, []byte{0x1B, 0x40}) {
		t.Fatalf("output does not start with ESC @: % x", out[:2])
	}
	// GS v 0 raster header present
	if !bytes.Contains(out, []byte{0x1D, 0x76, 0x30, 0x00}) {
		t.Fatal("missing GS v 0 raster header")
	}
	// GS V 0 cut at the end
	if !bytes.HasSuffix(out, []byte{0x1D, 0x56, 0x00}) {
		t.Fatalf("output does not end with GS V 0 cut: % x", out[len(out)-3:])
	}
}

func TestRasterEncoder_RejectsWrongArtifact(t *testing.T) {
	enc := NewRaster(stubBinarizer{})
	_, err := enc.Encode(context.Background(), dp.TextArtifact{Body: "x"}, dp.EncodeOptions{})
	if err == nil {
		t.Fatal("expected error for non-raster artifact, got nil")
	}
}
