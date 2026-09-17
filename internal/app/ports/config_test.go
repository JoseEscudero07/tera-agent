package ports

import "testing"

// Sin nada configurado se usan los defaults internos: página completa (0mm) y
// 300 dpi.
func TestPageTuningDefaults(t *testing.T) {
	var c Config
	margin, dpi := c.PageTuning("cualquiera")
	if margin != DefaultPageMarginMM {
		t.Errorf("margen = %v; se esperaba %v", margin, DefaultPageMarginMM)
	}
	if dpi != DefaultRenderDPI {
		t.Errorf("dpi = %d; se esperaba %d", dpi, DefaultRenderDPI)
	}
}

// El default global aplica a las impresoras que no traen override propio.
func TestPageTuningGlobal(t *testing.T) {
	c := Config{PageMarginMM: 12, RenderDPI: 600}
	margin, dpi := c.PageTuning("sin-override")
	if margin != 12 || dpi != 600 {
		t.Errorf("(%v, %d); se esperaba (12, 600)", margin, dpi)
	}
}

// El override por impresora gana al default global, campo a campo.
func TestPageTuningOverridePorImpresora(t *testing.T) {
	c := Config{
		PageMarginMM: 12,
		RenderDPI:    600,
		Printers: []ManagedPrinter{
			{Name: "HP Laser", Kind: KindPDF, PageMarginMM: 5}, // solo margen
			{Name: "Otra", Kind: KindPDF, RenderDPI: 150},      // solo dpi
			{Name: "Ambos", Kind: KindPDF, PageMarginMM: 3, RenderDPI: 200},
		},
	}
	casos := []struct {
		printer string
		margin  float64
		dpi     int
	}{
		{"HP Laser", 5, 600}, // margen propio, dpi global
		{"Otra", 12, 150},    // margen global, dpi propio
		{"Ambos", 3, 200},    // ambos propios
		{"Desconocida", 12, 600},
	}
	for _, k := range casos {
		m, d := c.PageTuning(k.printer)
		if m != k.margin || d != k.dpi {
			t.Errorf("%s: (%v, %d); se esperaba (%v, %d)", k.printer, m, d, k.margin, k.dpi)
		}
	}
}

// Un DPI negativo o cero en la config no debe propagarse al rasterizador: cae al
// default interno.
func TestPageTuningDPIInvalido(t *testing.T) {
	c := Config{RenderDPI: -50}
	if _, dpi := c.PageTuning("x"); dpi != DefaultRenderDPI {
		t.Errorf("dpi = %d; se esperaba el default %d", dpi, DefaultRenderDPI)
	}
}

// La calibración del corte y la de página son independientes: tocar una no debe
// alterar la otra.
func TestPageTuningNoInterfiereConElCorte(t *testing.T) {
	c := Config{
		CutFeedDots: 248, TopMarginDots: 12,
		PageMarginMM: 8, RenderDPI: 400,
		Printers: []ManagedPrinter{{Name: "XP-80", CutFeedDots: 200}},
	}
	cut, top := c.PrinterTuning("XP-80")
	if cut != 200 || top != 12 {
		t.Errorf("corte = (%d, %d); se esperaba (200, 12)", cut, top)
	}
	m, d := c.PageTuning("XP-80")
	if m != 8 || d != 400 {
		t.Errorf("página = (%v, %d); se esperaba (8, 400)", m, d)
	}
}
