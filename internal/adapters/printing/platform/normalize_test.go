package platform

import (
	"testing"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Tests aquí ejercen la lógica de normalización que en runtime sólo corre en
// Windows. Al vivir en un helper puro, se puede probar en cualquier CI sin
// cross-compilation. Si algún día se rompen estos tests, el binario de
// Windows tendrá el mismo bug.

func TestNormalize_TranslatesPDFToGDIRaster(t *testing.T) {
	in := dp.PrinterProfile{
		PrinterID:     "HP LaserJet",
		NativeFormats: []dp.DeviceFormat{dp.DevicePDF},
		DPI:           300,
		WidthDots:     2480,
	}
	out := NormalizeForGDIRaster(in)

	if len(out.NativeFormats) != 1 || out.NativeFormats[0] != dp.DeviceGDIRaster {
		t.Fatalf("NativeFormats = %v, want [%s]", out.NativeFormats, dp.DeviceGDIRaster)
	}
	if out.DPI != minRasterDPI {
		t.Errorf("DPI = %d, want at least %d (raster mínimo)", out.DPI, minRasterDPI)
	}
	if out.WidthDots != minRasterWidthDots {
		t.Errorf("WidthDots = %d, want at least %d", out.WidthDots, minRasterWidthDots)
	}
	if out.PrinterID != in.PrinterID {
		t.Errorf("PrinterID = %q, want %q", out.PrinterID, in.PrinterID)
	}
}

func TestNormalize_PreservesHigherDPI(t *testing.T) {
	// Si el ERP catalogó la impresora como 2400 dpi (Xerox / Canon de gama
	// alta), respetamos ese valor: no bajamos calidad.
	in := dp.PrinterProfile{
		NativeFormats: []dp.DeviceFormat{dp.DeviceGDIRaster},
		DPI:           2400,
		WidthDots:     19840,
	}
	out := NormalizeForGDIRaster(in)
	if out.DPI != 2400 {
		t.Errorf("DPI reducido: %d, want 2400", out.DPI)
	}
	if out.WidthDots != 19840 {
		t.Errorf("WidthDots reducido: %d, want 19840", out.WidthDots)
	}
}

func TestNormalize_LeavesESCPOSAlone(t *testing.T) {
	// Térmicas: no tocar. El pipeline ESC/POS no pasa por GDI raster.
	in := dp.PrinterProfile{
		PrinterID:     "POS-80C",
		NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS},
		DPI:           203,
		WidthDots:     576,
	}
	out := NormalizeForGDIRaster(in)
	if out.NativeFormats[0] != dp.DeviceESCPOS {
		t.Errorf("ESCPOS mutado a %v", out.NativeFormats)
	}
	if out.DPI != 203 || out.WidthDots != 576 {
		t.Errorf("valores de térmica alterados: dpi=%d width=%d", out.DPI, out.WidthDots)
	}
}

func TestNormalize_DedupesAfterTranslation(t *testing.T) {
	// Perfil raro pero posible: el Backend lista PDF y GDI-raster; tras
	// traducir el primero queda un duplicado que hay que colapsar.
	in := dp.PrinterProfile{
		NativeFormats: []dp.DeviceFormat{dp.DevicePDF, dp.DeviceGDIRaster},
	}
	out := NormalizeForGDIRaster(in)
	if len(out.NativeFormats) != 1 || out.NativeFormats[0] != dp.DeviceGDIRaster {
		t.Fatalf("dedup fallido: %v", out.NativeFormats)
	}
}

func TestNormalize_PreservesFormatOrder(t *testing.T) {
	// El orden es la prioridad para el resolver: térmica con fallback PDF.
	// Debe quedar térmica primero, gdi-raster segundo.
	in := dp.PrinterProfile{
		NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS, dp.DevicePDF},
	}
	out := NormalizeForGDIRaster(in)
	want := []dp.DeviceFormat{dp.DeviceESCPOS, dp.DeviceGDIRaster}
	if len(out.NativeFormats) != 2 {
		t.Fatalf("NativeFormats len = %d, want 2 (%v)", len(out.NativeFormats), out.NativeFormats)
	}
	for i, f := range want {
		if out.NativeFormats[i] != f {
			t.Errorf("NativeFormats[%d] = %s, want %s", i, out.NativeFormats[i], f)
		}
	}
}

func TestNormalize_EmptyProfile(t *testing.T) {
	// No debe explotar con perfil vacío (que hoy no debería llegar del
	// Backend, pero blindar cuesta 3 líneas y evita un panic en prod).
	out := NormalizeForGDIRaster(dp.PrinterProfile{PrinterID: "X"})
	if len(out.NativeFormats) != 0 {
		t.Errorf("empty in, non-empty out: %v", out.NativeFormats)
	}
	if out.PrinterID != "X" {
		t.Errorf("PrinterID perdido")
	}
}

// TestNormalizingProfileCache_EndToEnd emula lo que hace el lifecycle cuando
// llega un ProfilesSync del Backend: pasa el perfil por el cache envuelto y
// verifica que lo que sale por Profile() es lo que el resolver necesita.
// Este test es el más importante — cubre el bug que reportó el usuario ("no
// hay driver para pdf"). Se pasa NormalizeForGDIRaster explícito para que el
// test corra igual en Linux (donde ProfileNormalizerForOS es nil) y verifique
// exactamente lo que ocurrirá en Windows.
func TestNormalizingProfileCache_EndToEnd(t *testing.T) {
	inner := &fakeCache{m: map[string]dp.PrinterProfile{}}
	cache := NormalizingProfileCache(inner, NormalizeForGDIRaster)

	// El Backend envía el perfil como el ERP lo catalogó (DevicePDF).
	cache.SetAll([]dp.PrinterProfile{{
		PrinterID:     "HP LaserJet Pro",
		NativeFormats: []dp.DeviceFormat{dp.DevicePDF},
		DPI:           300,
		WidthDots:     2480,
	}})

	got, err := cache.Profile("HP LaserJet Pro")
	if err != nil {
		t.Fatalf("Profile error: %v", err)
	}
	if len(got.NativeFormats) != 1 || got.NativeFormats[0] != dp.DeviceGDIRaster {
		t.Fatalf("NativeFormats = %v, want [%s] (traducción PDF→GDI-raster)",
			got.NativeFormats, dp.DeviceGDIRaster)
	}
	if got.DPI < minRasterDPI {
		t.Errorf("DPI = %d, want ≥ %d", got.DPI, minRasterDPI)
	}
	if got.WidthDots < minRasterWidthDots {
		t.Errorf("WidthDots = %d, want ≥ %d", got.WidthDots, minRasterWidthDots)
	}
}

// TestNormalizingProfileCache_NilNormalizer verifica que pasar nil como
// normalizador equivale al cache pelado (que es lo que hace Linux/mac en DI).
func TestNormalizingProfileCache_NilNormalizer(t *testing.T) {
	inner := &fakeCache{m: map[string]dp.PrinterProfile{}}
	cache := NormalizingProfileCache(inner, nil)
	if cache != dp.ProfileCache(inner) {
		t.Fatal("nil normalizer debe devolver el inner tal cual, sin wrapping")
	}
}

// TestNormalizingProfileCache_ForgetPasses verifica que Forget llega al cache
// interno; el bug sería que la traducción rompa la interfaz.
func TestNormalizingProfileCache_ForgetPasses(t *testing.T) {
	inner := &fakeCache{m: map[string]dp.PrinterProfile{
		"POS": {PrinterID: "POS", NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS}},
	}}
	cache := NormalizingProfileCache(inner, NormalizeForGDIRaster)
	cache.Forget("POS")
	if _, ok := inner.m["POS"]; ok {
		t.Fatal("Forget no propagó al cache interno")
	}
}

// fakeCache es un ProfileCache mínimo para tests: no traduce, sólo almacena.
type fakeCache struct{ m map[string]dp.PrinterProfile }

func (c *fakeCache) Profile(id string) (dp.PrinterProfile, error) {
	if p, ok := c.m[id]; ok {
		return p, nil
	}
	return dp.PrinterProfile{}, nil
}
func (c *fakeCache) Set(p dp.PrinterProfile)       { c.m[p.PrinterID] = p }
func (c *fakeCache) SetAll(ps []dp.PrinterProfile) { for _, p := range ps { c.m[p.PrinterID] = p } }
func (c *fakeCache) Forget(id string)              { delete(c.m, id) }
