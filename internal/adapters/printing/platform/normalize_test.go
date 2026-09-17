package platform

import (
	"errors"
	"testing"

	"github.com/teraerp/tera-agent/internal/app/ports"
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

// --- Modo vectorial / imagen (NormalizeForWindows) ---

// Lo que envía el ERP para una impresora "normal" y lo que el resolver necesita
// en modo vectorial: PDF primero (pdftocairo) y raster detrás como red.
func TestNormalizeForWindows_VectorPutsPDFBeforeRaster(t *testing.T) {
	in := dp.PrinterProfile{
		PrinterID:     "HP LaserJet",
		NativeFormats: []dp.DeviceFormat{dp.DevicePDF},
		DPI:           203,
		WidthDots:     576,
	}
	out := NormalizeForWindows(in, ports.PageModeVector)
	assertFormats(t, out.NativeFormats, dp.DevicePDF, dp.DeviceGDIRaster)
	// Los mínimos raster se siguen aplicando: son los que usará el modo imagen
	// si pdftocairo falta o el trabajo es una imagen PNG/JPEG.
	if out.DPI != minRasterDPI || out.WidthDots != minRasterWidthDots {
		t.Errorf("mínimos raster no aplicados: dpi=%d width=%d", out.DPI, out.WidthDots)
	}
}

// El modo imagen es exactamente el comportamiento anterior.
func TestNormalizeForWindows_ImageIsTheRasterPath(t *testing.T) {
	in := dp.PrinterProfile{NativeFormats: []dp.DeviceFormat{dp.DevicePDF}, DPI: 203, WidthDots: 576}
	got := NormalizeForWindows(in, ports.PageModeImage)
	want := NormalizeForGDIRaster(in)
	assertFormats(t, got.NativeFormats, want.NativeFormats...)
	if got.DPI != want.DPI || got.WidthDots != want.WidthDots {
		t.Errorf("modo imagen distinto del raster de siempre: %+v vs %+v", got, want)
	}
}

// Un modo vacío (config antigua, CLI sin config) es vectorial.
func TestNormalizeForWindows_EmptyModeIsVector(t *testing.T) {
	out := NormalizeForWindows(dp.PrinterProfile{NativeFormats: []dp.DeviceFormat{dp.DevicePDF}}, "")
	assertFormats(t, out.NativeFormats, dp.DevicePDF, dp.DeviceGDIRaster)
}

// Un perfil que ya venía como gdi-raster (perfiles locales antiguos) también
// gana el camino vectorial en modo vector.
func TestNormalizeForWindows_VectorUpgradesRasterOnlyProfile(t *testing.T) {
	out := NormalizeForWindows(dp.PrinterProfile{NativeFormats: []dp.DeviceFormat{dp.DeviceGDIRaster}}, ports.PageModeVector)
	assertFormats(t, out.NativeFormats, dp.DevicePDF, dp.DeviceGDIRaster)
}

// Las térmicas no son impresoras de página: ningún modo las toca.
func TestNormalizeForWindows_LeavesThermalAlone(t *testing.T) {
	in := dp.PrinterProfile{NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS}, DPI: 203, WidthDots: 576}
	for _, mode := range []ports.PageMode{ports.PageModeVector, ports.PageModeImage} {
		out := NormalizeForWindows(in, mode)
		assertFormats(t, out.NativeFormats, dp.DeviceESCPOS)
		if out.DPI != 203 || out.WidthDots != 576 {
			t.Errorf("modo %s alteró la térmica: dpi=%d width=%d", mode, out.DPI, out.WidthDots)
		}
	}
}

// La prioridad entre formatos se conserva: térmica con respaldo de página.
func TestNormalizeForWindows_PreservesPriority(t *testing.T) {
	in := dp.PrinterProfile{NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS, dp.DevicePDF, dp.DeviceGDIRaster}}
	out := NormalizeForWindows(in, ports.PageModeVector)
	assertFormats(t, out.NativeFormats, dp.DeviceESCPOS, dp.DevicePDF, dp.DeviceGDIRaster)
}

// Cambiar el modo en el panel tiene que valer para el siguiente trabajo sin
// volver a sincronizar ni olvidar el perfil del Backend. Es la razón de que el
// cache normalice al leer.
func TestNormalizingProfileCache_ModeChangeAppliesOnNextRead(t *testing.T) {
	inner := &fakeCache{m: map[string]dp.PrinterProfile{}}
	mode := ports.PageModeVector
	cache := NormalizingProfileCache(inner, func(p dp.PrinterProfile) dp.PrinterProfile {
		return NormalizeForWindows(p, mode)
	})

	cache.SetAll([]dp.PrinterProfile{{PrinterID: "HP", NativeFormats: []dp.DeviceFormat{dp.DevicePDF}}})

	got, _ := cache.Profile("HP")
	assertFormats(t, got.NativeFormats, dp.DevicePDF, dp.DeviceGDIRaster)

	mode = ports.PageModeImage
	got, _ = cache.Profile("HP")
	assertFormats(t, got.NativeFormats, dp.DeviceGDIRaster)

	// El perfil almacenado sigue siendo el del Backend, sin traducir.
	assertFormats(t, inner.m["HP"].NativeFormats, dp.DevicePDF)
}

// Un perfil que no existe no se normaliza: el error tiene que llegar tal cual.
func TestNormalizingProfileCache_PropagatesMissingProfile(t *testing.T) {
	inner := &fakeCache{m: map[string]dp.PrinterProfile{}, err: errors.New("sin perfil")}
	called := false
	cache := NormalizingProfileCache(inner, func(p dp.PrinterProfile) dp.PrinterProfile { called = true; return p })
	if _, err := cache.Profile("X"); err == nil {
		t.Fatal("se perdió el error del cache interno")
	}
	if called {
		t.Error("se normalizó un perfil inexistente")
	}
}

func assertFormats(t *testing.T, got []dp.DeviceFormat, want ...dp.DeviceFormat) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("NativeFormats = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NativeFormats = %v, want %v", got, want)
		}
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

// fakeCache es un ProfileCache mínimo para tests: no traduce, sólo almacena. err,
// si no es nil, se devuelve para los perfiles que no existen.
type fakeCache struct {
	m   map[string]dp.PrinterProfile
	err error
}

func (c *fakeCache) Profile(id string) (dp.PrinterProfile, error) {
	if p, ok := c.m[id]; ok {
		return p, nil
	}
	return dp.PrinterProfile{}, c.err
}
func (c *fakeCache) Set(p dp.PrinterProfile) { c.m[p.PrinterID] = p }
func (c *fakeCache) SetAll(ps []dp.PrinterProfile) {
	for _, p := range ps {
		c.m[p.PrinterID] = p
	}
}
func (c *fakeCache) Forget(id string) { delete(c.m, id) }

// --- Nombres con tildes y Windows sin UTF-8 en manifiestos ---

func TestEffectivePageMode(t *testing.T) {
	cases := []struct {
		name       string
		configured ports.PageMode
		printer    string
		utf8ACP    bool
		want       ports.PageMode
	}{
		{"vectorial, nombre ASCII, Windows viejo", ports.PageModeVector, "HP LaserJet", false, ports.PageModeVector},
		{"vectorial, con tildes, Windows 10 1903+", ports.PageModeVector, "LÁSER Facturación Ñ", true, ports.PageModeVector},
		// pdftocairo fallaría con "Printer not found": mejor imagen que nada.
		{"vectorial, con tildes, Windows viejo", ports.PageModeVector, "LÁSER Facturación Ñ", false, ports.PageModeImage},
		{"por defecto (vacío), con tildes, Windows viejo", "", "Impresora Oficina Ñ", false, ports.PageModeImage},
		{"imagen elegida por el usuario siempre gana", ports.PageModeImage, "HP LaserJet", true, ports.PageModeImage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EffectivePageMode(c.configured, c.printer, c.utf8ACP); got != c.want {
				t.Errorf("EffectivePageMode = %q, want %q", got, c.want)
			}
		})
	}
}
