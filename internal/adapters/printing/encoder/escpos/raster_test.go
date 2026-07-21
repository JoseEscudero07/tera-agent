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

func rasterHeight(t *testing.T, out []byte) int {
	t.Helper()
	i := bytes.Index(out, []byte{0x1D, 0x76, 0x30, 0x00})
	if i < 0 {
		t.Fatal("missing raster header")
	}
	return int(out[i+6]) | int(out[i+7])<<8
}

func TestRasterEncoder_TrimsLeadingBlankToConfiguredMargin(t *testing.T) {
	enc := NewRaster(leadingBlankBinarizer{}) // 40 blank top rows, content at 40 and 49
	art := dp.RasterArtifact{Pages: []image.Image{image.NewGray(image.Rect(0, 0, 8, 50))}, WidthDots: 8}

	// Explicit top margin of 10 dots: keep 10, remove 30 → height 50-30 = 20.
	out, err := enc.Encode(context.Background(), art, dp.EncodeOptions{TopMarginDots: 10})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	if h := rasterHeight(t, out); h != 20 {
		t.Fatalf("raster height = %d, want 20 (kept 10 of 40 leading blanks)", h)
	}

	// Default (0) uses defaultTopMarginDots: keep 16, remove 24 → height 26.
	out, err = enc.Encode(context.Background(), art, dp.EncodeOptions{})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	if h := rasterHeight(t, out); h != 50-(40-defaultTopMarginDots) {
		t.Fatalf("raster height = %d, want %d (default top margin)", h, 50-(40-defaultTopMarginDots))
	}
}

func TestRasterEncoder_CutFeedDotsFromOptions(t *testing.T) {
	enc := NewRaster(stubBinarizer{})
	art := dp.RasterArtifact{Pages: []image.Image{image.NewGray(image.Rect(0, 0, 8, 1))}, WidthDots: 8}

	out, err := enc.Encode(context.Background(), art, dp.EncodeOptions{Cut: true, CutFeedDots: 199})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	// ESC J 199 must appear right before the cut.
	if !bytes.Contains(out, []byte{0x1B, 0x4A, 199}) {
		t.Fatalf("missing ESC J 199 (per-printer feed): % x", out)
	}
}

func TestRasterEncoder_RejectsWrongArtifact(t *testing.T) {
	enc := NewRaster(stubBinarizer{})
	_, err := enc.Encode(context.Background(), dp.TextArtifact{Body: "x"}, dp.EncodeOptions{})
	if err == nil {
		t.Fatal("expected error for non-raster artifact, got nil")
	}
}
