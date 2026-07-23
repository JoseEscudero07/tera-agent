//go:build windows

package gdi

import (
	"context"
	"os"
	"testing"

	"github.com/teraerp/tera-agent/internal/adapters/printing/binarizer"
	"github.com/teraerp/tera-agent/internal/adapters/printing/encoder/raster"
	"github.com/teraerp/tera-agent/internal/adapters/printing/rasterizer/poppler"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

type tlog struct{ t *testing.T }

func (l tlog) Debug(m string, a ...any) { l.t.Logf("DEBUG "+m+" %v", a) }
func (l tlog) Info(m string, a ...any)  { l.t.Logf("INFO  "+m+" %v", a) }
func (l tlog) Warn(m string, a ...any)  { l.t.Logf("WARN  "+m+" %v", a) }
func (l tlog) Error(m string, a ...any) { l.t.Logf("ERROR "+m+" %v", a) }

// TestRealPaper ejerce el path de producción real (poppler -> multipng -> GDI)
// contra una impresora física. GASTA PAPEL: sólo corre con TERA_PAPER=1.
//
//	$env:TERA_PAPER=1; $env:TERA_DIAG_PRINTER="HP Laser MFP 131 133 135-138"
//	go test ./internal/adapters/printing/driver/gdi -run TestRealPaper -v
func TestRealPaper(t *testing.T) {
	if os.Getenv("TERA_PAPER") != "1" {
		t.Skip("TERA_PAPER != 1; se omite para no gastar papel")
	}
	printer := os.Getenv("TERA_DIAG_PRINTER")
	if printer == "" {
		t.Fatal("define TERA_DIAG_PRINTER")
	}
	pdf, err := os.ReadFile(os.Getenv("TERA_TESTPDF"))
	if err != nil {
		t.Fatalf("lee PDF de prueba (TERA_TESTPDF): %v", err)
	}

	gray := os.Getenv("TERA_GRAY") == "1" // por defecto color (path 24bpp/auto)
	log := tlog{t}
	ras := poppler.New(log)
	pages, err := ras.Rasterize(context.Background(), pdf, dp.RasterOptions{WidthDots: 4960, Gray: gray})
	if err != nil {
		t.Fatalf("rasterize: %v", err)
	}
	t.Logf("páginas rasterizadas: %d (gray=%v)", len(pages), gray)

	enc := raster.NewMultiPNG()
	data, err := enc.Encode(context.Background(), dp.RasterArtifact{Pages: pages, WidthDots: 4960}, dp.EncodeOptions{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	drv := New(log, binarizer.NewOtsu())
	if err := drv.Send(context.Background(), printer, data); err != nil {
		t.Fatalf("SEND (impresión real) falló: %v", err)
	}
	t.Log("OK: enviado a la impresora; revisa la bandeja.")
}
