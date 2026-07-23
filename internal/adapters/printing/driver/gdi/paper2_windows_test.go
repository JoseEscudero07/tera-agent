//go:build windows

package gdi

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/teraerp/tera-agent/internal/adapters/printing/binarizer"
	"github.com/teraerp/tera-agent/internal/adapters/printing/encoder/raster"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// TestRealPaperSynthetic imprime imágenes sintéticas A4 completas (4960x7016)
// para ejercer el backoff (1bpp gris no cabe a 600dpi -> baja resolución) y el
// camino color (24bpp). GASTA PAPEL: sólo con TERA_PAPER=1.
//
//	$env:TERA_PAPER=1; $env:TERA_DIAG_PRINTER="HP Laser MFP 131 133 135-138"
//	go test ./internal/adapters/printing/driver/gdi -run TestRealPaperSynthetic -v
func TestRealPaperSynthetic(t *testing.T) {
	if os.Getenv("TERA_PAPER") != "1" {
		t.Skip("TERA_PAPER != 1")
	}
	printer := os.Getenv("TERA_DIAG_PRINTER")
	if printer == "" {
		t.Fatal("define TERA_DIAG_PRINTER")
	}
	log := tlog{t}
	drv := New(log, binarizer.NewOtsu())

	send := func(img image.Image) error {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			return err
		}
		data, err := raster.NewMultiPNG().Encode(context.Background(),
			dp.RasterArtifact{Pages: []image.Image{img}, WidthDots: img.Bounds().Dx()}, dp.EncodeOptions{})
		if err != nil {
			return err
		}
		return drv.Send(context.Background(), printer, data)
	}

	t.Run("gris_A4_completa_backoff", func(t *testing.T) {
		img := patternGray(4960, 7016)
		if err := send(img); err != nil {
			t.Fatalf("gris A4: %v", err)
		}
		t.Log("OK gris A4 completa (backoff): revisa la hoja")
	})

	t.Run("color_A4", func(t *testing.T) {
		img := patternColor(4960, 7016)
		if err := send(img); err != nil {
			t.Fatalf("color A4: %v", err)
		}
		t.Log("OK color A4 (24bpp): revisa la hoja")
	})
}

// patternGray: fondo blanco, marco negro y franjas horizontales cada 200px, con
// texto-like (bloques) — para juzgar nitidez a lo alto de la página.
func patternGray(w, h int) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 0xFF
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			black := x < 10 || x >= w-10 || y < 10 || y >= h-10 || (y%200 < 3) || (x%40 < 2 && y%400 < 60)
			if black {
				g.Pix[y*g.Stride+x] = 0
			}
		}
	}
	return g
}

// patternColor: barras verticales RGB para verificar que el color viaja.
func patternColor(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	cols := []color.RGBA{{255, 0, 0, 255}, {0, 180, 0, 255}, {0, 0, 255, 255}, {0, 0, 0, 255}}
	band := w / len(cols)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cols[min(x/band, len(cols)-1)]
			if x < 10 || x >= w-10 || y < 10 || y >= h-10 {
				c = color.RGBA{0, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
