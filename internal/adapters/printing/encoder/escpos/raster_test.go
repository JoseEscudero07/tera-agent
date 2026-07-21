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

// tallBinarizer returns a bitmap with content only in the first row and many
// trailing blank rows, to exercise trailing-blank trimming.
type tallBinarizer struct{}

func (tallBinarizer) Name() string { return "tall" }
func (tallBinarizer) Binarize(image.Image) *dp.MonoBitmap {
	bits := make([]byte, 100) // 100 rows x 1 byte (8px wide)
	bits[0] = 0xFF            // only the first row has content
	return &dp.MonoBitmap{Width: 8, Height: 100, Bits: bits}
}

func TestRasterEncoder_TrimsTrailingBlankAndFeedsBeforeCut(t *testing.T) {
	enc := NewRaster(tallBinarizer{})
	art := dp.RasterArtifact{Pages: []image.Image{image.NewGray(image.Rect(0, 0, 8, 100))}, WidthDots: 8}

	out, err := enc.Encode(context.Background(), art, dp.EncodeOptions{Cut: true})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	// GS v 0 header yL/yH must encode height 1 (trimmed from 100), not 100.
	i := bytes.Index(out, []byte{0x1D, 0x76, 0x30, 0x00})
	if i < 0 {
		t.Fatal("missing raster header")
	}
	height := int(out[i+6]) | int(out[i+7])<<8
	if height != 1 {
		t.Fatalf("raster height = %d, want 1 (trailing blanks trimmed)", height)
	}
	// Must feed (ESC J) before the cut.
	if !bytes.Contains(out, []byte{0x1B, 0x4A}) {
		t.Fatal("missing ESC J feed before cut")
	}
	if !bytes.HasSuffix(out, []byte{0x1D, 0x56, 0x00}) {
		t.Fatal("must end with GS V 0 cut")
	}
}

// leadingBlankBinarizer returns content in the middle with 40 blank rows on top,
// to exercise the leading-blank halving (top margin).
type leadingBlankBinarizer struct{}

func (leadingBlankBinarizer) Name() string { return "leading" }
func (leadingBlankBinarizer) Binarize(image.Image) *dp.MonoBitmap {
	bits := make([]byte, 50) // 50 rows x 1 byte
	bits[40] = 0xFF          // 40 blank rows on top, then content
	bits[49] = 0xFF          // content on the last row → no trailing blank to trim
	return &dp.MonoBitmap{Width: 8, Height: 50, Bits: bits}
}

func TestRasterEncoder_HalvesLeadingBlankTopMargin(t *testing.T) {
	enc := NewRaster(leadingBlankBinarizer{})
	art := dp.RasterArtifact{Pages: []image.Image{image.NewGray(image.Rect(0, 0, 8, 50))}, WidthDots: 8}

	out, err := enc.Encode(context.Background(), art, dp.EncodeOptions{})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	// After halving 40 leading blanks (remove 20) the raster height is 50-20 = 30.
	i := bytes.Index(out, []byte{0x1D, 0x76, 0x30, 0x00})
	if i < 0 {
		t.Fatal("missing raster header")
	}
	height := int(out[i+6]) | int(out[i+7])<<8
	if height != 30 {
		t.Fatalf("raster height = %d, want 30 (half of 40 leading blanks trimmed)", height)
	}
}

func TestRasterEncoder_RejectsWrongArtifact(t *testing.T) {
	enc := NewRaster(stubBinarizer{})
	_, err := enc.Encode(context.Background(), dp.TextArtifact{Body: "x"}, dp.EncodeOptions{})
	if err == nil {
		t.Fatal("expected error for non-raster artifact, got nil")
	}
}
