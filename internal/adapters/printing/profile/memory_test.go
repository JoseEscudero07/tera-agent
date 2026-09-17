package profile

import (
	"testing"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

var escposFallback = dp.PrinterProfile{
	NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS}, WidthDots: 576, DPI: 203,
}

func TestProfileOr_BackendProfileWins(t *testing.T) {
	c := NewMemoryCache()
	c.Set(dp.PrinterProfile{PrinterID: "HP", NativeFormats: []dp.DeviceFormat{dp.DevicePDF}})

	got := c.ProfileOr("HP", escposFallback)
	if len(got.NativeFormats) != 1 || got.NativeFormats[0] != dp.DevicePDF {
		t.Fatalf("NativeFormats = %v, want [pdf] (el perfil del Backend)", got.NativeFormats)
	}
}

// Sin perfil, el respaldo sale con el PrinterID pedido (el normalizador de
// Windows resuelve el modo por impresora) y NO queda guardado.
func TestProfileOr_MissingUsesFallbackWithoutStoringIt(t *testing.T) {
	c := NewMemoryCache()

	got := c.ProfileOr("POS", escposFallback)
	if got.PrinterID != "POS" || got.WidthDots != 576 {
		t.Fatalf("perfil = %+v, want el respaldo con PrinterID POS", got)
	}
	if _, err := c.Profile("POS"); err == nil {
		t.Fatal("ProfileOr guardó el respaldo en la caché")
	}
}

// Un perfil sin formatos no puede resolver ningún pipeline: cuenta como ausente.
func TestProfileOr_ProfileWithoutFormatsUsesFallback(t *testing.T) {
	c := NewMemoryCache()
	c.Set(dp.PrinterProfile{PrinterID: "POS"})

	if got := c.ProfileOr("POS", escposFallback); len(got.NativeFormats) != 1 || got.NativeFormats[0] != dp.DeviceESCPOS {
		t.Fatalf("NativeFormats = %v, want [escpos]", got.NativeFormats)
	}
}
