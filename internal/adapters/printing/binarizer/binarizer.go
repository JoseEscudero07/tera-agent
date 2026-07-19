// Package binarizer holds the pluggable 1-bpp strategies for thermal printing.
// Otsu is the recommended default; Atkinson is for image/logo content. See
// docs/BINARIZATION.md. Owner: Printing Engineer.
package binarizer

import (
	"image"
	"image/color"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// toGray returns img as *image.Gray (no-op if already gray).
func toGray(img image.Image) *image.Gray {
	if g, ok := img.(*image.Gray); ok {
		return g
	}
	b := img.Bounds()
	g := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g.Set(x, y, color.GrayModel.Convert(img.At(x, y)))
		}
	}
	return g
}

// pack builds a MonoBitmap from a grayscale image, setting a black dot wherever
// black(v) is true. Layout matches ESC/POS GS v 0 (MSB leftmost).
func pack(g *image.Gray, black func(v uint8) bool) *dp.MonoBitmap {
	b := g.Bounds()
	w, h := b.Dx(), b.Dy()
	stride := (w + 7) / 8
	bits := make([]byte, stride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if black(g.GrayAt(b.Min.X+x, b.Min.Y+y).Y) {
				bits[y*stride+x/8] |= 0x80 >> (uint(x) % 8)
			}
		}
	}
	return &dp.MonoBitmap{Width: w, Height: h, Bits: bits}
}
