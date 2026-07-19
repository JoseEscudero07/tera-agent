package binarizer

import (
	"image"
	"image/color"
	"testing"
)

// halfBlackImage returns a w x h gray image whose left half is black and right
// half is white.
func halfBlackImage(w, h int) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				g.SetGray(x, y, color.Gray{Y: 0})
			} else {
				g.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	return g
}

func TestOtsu_SeparatesBlackAndWhite(t *testing.T) {
	img := halfBlackImage(16, 4)
	mb := NewOtsu().Binarize(img)

	if mb.Width != 16 || mb.Height != 4 {
		t.Fatalf("bitmap size = %dx%d, want 16x4", mb.Width, mb.Height)
	}
	stride := mb.Stride() // 16px -> 2 bytes
	if stride != 2 {
		t.Fatalf("stride = %d, want 2", stride)
	}
	// Left byte (pixels 0..7) should be all black bits (0xFF); right byte white (0x00).
	for y := 0; y < mb.Height; y++ {
		if got := mb.Bits[y*stride]; got != 0xFF {
			t.Fatalf("row %d left byte = %08b, want 11111111", y, got)
		}
		if got := mb.Bits[y*stride+1]; got != 0x00 {
			t.Fatalf("row %d right byte = %08b, want 00000000", y, got)
		}
	}
}

func TestThreshold_MSBIsLeftmostPixel(t *testing.T) {
	// Single black pixel at x=0 must set bit 0x80 of the first byte.
	g := image.NewGray(image.Rect(0, 0, 8, 1))
	for x := 0; x < 8; x++ {
		g.SetGray(x, 0, color.Gray{Y: 255})
	}
	g.SetGray(0, 0, color.Gray{Y: 0})

	mb := NewThreshold(128).Binarize(g)
	if mb.Bits[0] != 0x80 {
		t.Fatalf("first byte = %08b, want 10000000", mb.Bits[0])
	}
}

func TestAtkinson_ProducesBitmapOfSameSize(t *testing.T) {
	img := halfBlackImage(32, 8)
	mb := NewAtkinson().Binarize(img)
	if mb.Width != 32 || mb.Height != 8 {
		t.Fatalf("bitmap size = %dx%d, want 32x8", mb.Width, mb.Height)
	}
	if len(mb.Bits) != mb.Stride()*mb.Height {
		t.Fatalf("bits length = %d, want %d", len(mb.Bits), mb.Stride()*mb.Height)
	}
}
