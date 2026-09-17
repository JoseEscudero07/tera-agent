package pdfvector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/teraerp/tera-agent/internal/adapters/logger"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Tamaño real es la decisión que más se nota en el papel: sin -noshrink una
// factura sale al ~96 % con el margen del hardware sumado al del documento.
func TestPrintArgs(t *testing.T) {
	got := printArgs("HP LaserJet Pro", "tera-agent.pdf")
	want := []string{"-print", "-printer", "HP LaserJet Pro", "-noshrink", "tera-agent.pdf"}
	if len(got) != len(want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Forzar el tamaño de papel rompería carta y A4 en la misma impresora.
func TestPrintArgsNeverForcesPaper(t *testing.T) {
	for _, a := range printArgs("X", "f.pdf") {
		if a == "-paper" || a == "-paperw" || a == "-paperh" {
			t.Errorf("printArgs fuerza el papel con %q", a)
		}
	}
}

// Sin pdftocairo el driver no debe aceptar PDF: así el resolver cae a gdi-raster
// al componer el pipeline, sin llegar a intentar imprimir.
func TestAcceptsOnlyPDFWhenAvailable(t *testing.T) {
	log := logger.New("error")

	missing := New(log, filepath.Join(t.TempDir(), "pdftocairo-no-existe"))
	if missing.Accepts(dp.DevicePDF) {
		t.Error("acepta PDF sin pdftocairo instalado")
	}

	present := New(log, existingFile(t))
	if !present.Accepts(dp.DevicePDF) {
		t.Error("no acepta PDF con pdftocairo disponible")
	}
	for _, f := range []dp.DeviceFormat{dp.DeviceGDIRaster, dp.DeviceESCPOS, dp.DevicePNG, dp.DeviceRaw} {
		if present.Accepts(f) {
			t.Errorf("acepta %q; solo debe aceptar pdf", f)
		}
	}
}

func TestSendRejectsEmptyInput(t *testing.T) {
	d := New(logger.New("error"), existingFile(t))
	if err := d.Send(context.Background(), "", []byte("%PDF")); err == nil {
		t.Error("impresora vacía aceptada")
	}
	if err := d.Send(context.Background(), "X", nil); err == nil {
		t.Error("PDF vacío aceptado")
	}
}

// existingFile devuelve una ruta que existe en cualquier SO (el propio binario de
// test). Basta para Available, que solo comprueba que el ejecutable esté ahí.
func existingFile(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}
