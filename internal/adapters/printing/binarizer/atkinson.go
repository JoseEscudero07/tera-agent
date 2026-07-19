package binarizer

import (
	"image"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Atkinson error-diffusion dithering. Diffuses only 6/8 of the error, giving
// higher contrast and lighter ink than Floyd-Steinberg — better suited to
// thermal printers for images/logos. Not recommended for text or codes.
type Atkinson struct{}

// NewAtkinson returns the Atkinson binarizer.
func NewAtkinson() dp.Binarizer { return Atkinson{} }

func (Atkinson) Name() string { return "atkinson" }

// atkinsonSpread are the 6 neighbours that each receive 1/8 of the error.
var atkinsonSpread = [...]struct{ dx, dy int }{
	{1, 0}, {2, 0}, {-1, 1}, {0, 1}, {1, 1}, {0, 2},
}

func (Atkinson) Binarize(img image.Image) *dp.MonoBitmap {
	g := toGray(img)
	b := g.Bounds()
	w, h := b.Dx(), b.Dy()

	// Float working buffer of luminance values.
	buf := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			buf[y*w+x] = float64(g.GrayAt(b.Min.X+x, b.Min.Y+y).Y)
		}
	}

	stride := (w + 7) / 8
	bits := make([]byte, stride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			old := buf[y*w+x]
			var newv float64
			black := old < 128
			if !black {
				newv = 255
			}
			qErr := (old - newv) / 8.0
			for _, s := range atkinsonSpread {
				nx, ny := x+s.dx, y+s.dy
				if nx >= 0 && nx < w && ny >= 0 && ny < h {
					buf[ny*w+nx] += qErr
				}
			}
			if black {
				bits[y*stride+x/8] |= 0x80 >> (uint(x) % 8)
			}
		}
	}
	return &dp.MonoBitmap{Width: w, Height: h, Bits: bits}
}
