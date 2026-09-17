package ports

import "testing"

// Sin nada configurado se usa el default interno: página completa (0 mm).
func TestPageMarginDefault(t *testing.T) {
	var c Config
	if m := c.PageMargin("cualquiera"); m != DefaultPageMarginMM {
		t.Errorf("margen = %v; se esperaba %v", m, DefaultPageMarginMM)
	}
}

// El default global aplica a las impresoras que no traen override propio, y el
// override por impresora gana al global.
func TestPageMarginOverridePorImpresora(t *testing.T) {
	c := Config{
		PageMarginMM: 12,
		Printers: []ManagedPrinter{
			{Name: "HP Laser", Kind: KindPDF, PageMarginMM: 5},
			{Name: "Sin margen propio", Kind: KindPDF},
		},
	}
	for printer, want := range map[string]float64{
		"HP Laser":          5,
		"Sin margen propio": 12,
		"Desconocida":       12,
	} {
		if m := c.PageMargin(printer); m != want {
			t.Errorf("%s: margen = %v; se esperaba %v", printer, m, want)
		}
	}
}

// La calibración del corte y la de página son independientes: tocar una no debe
// alterar la otra.
func TestPageMarginNoInterfiereConElCorte(t *testing.T) {
	c := Config{
		CutFeedDots: 248, TopMarginDots: 12,
		PageMarginMM: 8,
		Printers:     []ManagedPrinter{{Name: "XP-80", CutFeedDots: 200}},
	}
	cut, top := c.PrinterTuning("XP-80")
	if cut != 200 || top != 12 {
		t.Errorf("corte = (%d, %d); se esperaba (200, 12)", cut, top)
	}
	if m := c.PageMargin("XP-80"); m != 8 {
		t.Errorf("margen = %v; se esperaba 8", m)
	}
}

// Sin nada configurado, toda impresora de página imprime en vectorial: es el
// modo que no degrada el texto.
func TestPageModeOfDefaultsToVector(t *testing.T) {
	var c Config
	if got := c.PageModeOf("HP"); got != PageModeVector {
		t.Errorf("sin config = %q, want %q", got, PageModeVector)
	}
	c.Printers = []ManagedPrinter{{Name: "HP", Kind: KindPDF}}
	if got := c.PageModeOf("HP"); got != PageModeVector {
		t.Errorf("gestionada sin modo = %q, want %q", got, PageModeVector)
	}
}

func TestPageModeOfPerPrinter(t *testing.T) {
	c := Config{Printers: []ManagedPrinter{
		{Name: "Samsung M2020", Kind: KindPDF, PageMode: PageModeImage},
		{Name: "HP", Kind: KindPDF, PageMode: PageModeVector},
	}}
	if got := c.PageModeOf("Samsung M2020"); got != PageModeImage {
		t.Errorf("Samsung = %q, want image", got)
	}
	if got := c.PageModeOf("HP"); got != PageModeVector {
		t.Errorf("HP = %q, want vector", got)
	}
	if got := c.PageModeOf("otra"); got != PageModeVector {
		t.Errorf("no gestionada = %q, want vector", got)
	}
}

// Un valor desconocido en el YAML (errata a mano) no debe sacar a la impresora
// del modo por defecto.
func TestPageModeOfUnknownValueIsVector(t *testing.T) {
	c := Config{Printers: []ManagedPrinter{{Name: "HP", PageMode: "imagen"}}}
	if got := c.PageModeOf("HP"); got != PageModeVector {
		t.Errorf("valor desconocido = %q, want vector", got)
	}
}
