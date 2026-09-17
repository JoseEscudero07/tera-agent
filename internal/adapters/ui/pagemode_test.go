package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

type fakeDiscovery struct{ names []string }

func (f fakeDiscovery) List(context.Context) ([]dp.Printer, error) {
	out := make([]dp.Printer, 0, len(f.names))
	for _, n := range f.names {
		out = append(out, dp.Printer{ID: n, Name: n, Status: dp.Status("READY")})
	}
	return out, nil
}
func (fakeDiscovery) StatusOf(context.Context, string) (dp.Status, error) { return "READY", nil }

// recordingProfiles es una caché de perfiles que cuenta las escrituras: ni un
// cambio en el panel ni "Probar" deben tocar el perfil que envió el Backend.
type recordingProfiles struct {
	m      map[string]dp.PrinterProfile
	writes int
}

func (c *recordingProfiles) Profile(id string) (dp.PrinterProfile, error) { return c.m[id], nil }
func (c *recordingProfiles) Set(p dp.PrinterProfile)                      { c.writes++; c.m[p.PrinterID] = p }
func (c *recordingProfiles) SetAll(ps []dp.PrinterProfile) {
	c.writes++
	for _, p := range ps {
		c.m[p.PrinterID] = p
	}
}
func (c *recordingProfiles) ProfileOr(id string, fallback dp.PrinterProfile) dp.PrinterProfile {
	if p, ok := c.m[id]; ok && len(p.NativeFormats) > 0 {
		return p
	}
	fallback.PrinterID = id
	return fallback
}

func manage(t *testing.T, s *Server, body string) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.printersManage(rec, httptest.NewRequest(http.MethodPost, "/api/printers/manage", strings.NewReader(body)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("manage %s -> %d %s", body, rec.Code, rec.Body.String())
	}
}

func TestPrintersManagePersistsPageMode(t *testing.T) {
	s, saved, _ := newTestServer(ports.Config{})
	profiles := &recordingProfiles{m: map[string]dp.PrinterProfile{}}
	s.d.Profiles = profiles
	applied := ports.Config{}
	s.d.ApplyTuning = func(c ports.Config) { applied = c }

	manage(t, s, `{"name":"Samsung M2020","enabled":true,"kind":"pdf","pageMode":"image"}`)
	if got := saved.PageModeOf("Samsung M2020"); got != ports.PageModeImage {
		t.Errorf("guardado = %q, want image", got)
	}
	// ApplyTuning es lo que reinstala el resolutor de modo en el motor en caliente.
	if got := applied.PageModeOf("Samsung M2020"); got != ports.PageModeImage {
		t.Errorf("ApplyTuning recibió modo %q, want image", got)
	}

	manage(t, s, `{"name":"Samsung M2020","enabled":true,"kind":"pdf","pageMode":"vector"}`)
	if got := saved.PageModeOf("Samsung M2020"); got != ports.PageModeVector {
		t.Errorf("tras volver a vectorial = %q", got)
	}
	if profiles.writes != 0 {
		t.Errorf("cambiar el modo escribió %d veces en la caché; el cache ya aplica el modo al leer", profiles.writes)
	}
}

// Un valor desconocido (panel antiguo, petición a mano) cae al modo por defecto.
func TestPrintersManageUnknownPageModeIsVector(t *testing.T) {
	s, saved, _ := newTestServer(ports.Config{})
	s.d.Profiles = &recordingProfiles{m: map[string]dp.PrinterProfile{}}
	manage(t, s, `{"name":"HP","enabled":true,"kind":"pdf","pageMode":"bitmap"}`)
	if got := saved.PageModeOf("HP"); got != ports.PageModeVector {
		t.Errorf("modo resultante = %q, want vector", got)
	}
	for _, p := range saved.Printers {
		if p.Name == "HP" && p.PageMode != "" {
			t.Errorf("PageMode guardado = %q; el modo por defecto se guarda vacío", p.PageMode)
		}
	}
}

func TestPrintersListExposesEffectivePageMode(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{Printers: []ports.ManagedPrinter{
		{Name: "Samsung M2020", Enabled: true, Kind: ports.KindPDF, PageMode: ports.PageModeImage},
	}})
	s.d.Discovery = fakeDiscovery{names: []string{"Samsung M2020", "HP LaserJet"}}

	rec := httptest.NewRecorder()
	s.printers(rec, httptest.NewRequest(http.MethodGet, "/api/printers", nil))
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	modes := map[string]any{}
	for _, p := range list {
		modes[p["name"].(string)] = p["pageMode"]
	}
	if modes["Samsung M2020"] != "image" || modes["HP LaserJet"] != "vector" {
		t.Errorf("pageMode expuesto = %v", modes)
	}
}

// El perfil local de una láser declara pdf, como el del Backend: así pasa por la
// misma traducción de modo y "Probar" imprime igual que un trabajo del ERP.
func TestPDFProfileDeclaresPDF(t *testing.T) {
	p := pdfProfile(ports.Config{}, "HP")
	if len(p.NativeFormats) != 1 || p.NativeFormats[0] != dp.DevicePDF {
		t.Errorf("NativeFormats = %v, want [pdf]", p.NativeFormats)
	}
	if !profileIsPDF(p) {
		t.Error("profileIsPDF no reconoce el perfil local de página")
	}
}

// Regresión (go test -race): el panel reescribía Config.Printers en su sitio
// mientras los resolutores instalados en el motor —que comparten ese array— lo
// leían desde los trabajos en curso.
func TestPrintersManageDoesNotRaceWithRunningJobs(t *testing.T) {
	cfg := ports.Config{Printers: []ports.ManagedPrinter{
		{Name: "HP", Enabled: true, Kind: ports.KindPDF},
		{Name: "POS", Enabled: true, Kind: ports.KindThermal},
	}}
	s, _, _ := newTestServer(cfg)
	s.d.Profiles = &recordingProfiles{m: map[string]dp.PrinterProfile{}}
	s.d.Discovery = fakeDiscovery{}

	// Lo que ApplyTuning instaló al arrancar: resolutores sobre una copia de Config
	// que comparte el array de impresoras con el panel.
	modeOf, tuning := s.d.Cfg.PageModeOf, s.d.Cfg.PrinterTuning

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			_ = modeOf("HP")
			_, _ = tuning("POS")
		}
	}()
	for i := 0; i < 50; i++ {
		mode := "image"
		if i%2 == 0 {
			mode = "vector"
		}
		manage(t, s, `{"name":"HP","enabled":true,"kind":"pdf","pageMode":"`+mode+`"}`)
	}
	rec := httptest.NewRecorder()
	s.printers(rec, httptest.NewRequest(http.MethodDelete, "/api/printers?name=POS", nil))
	<-done
}

// Regresión: cambiar el DPI olvidaba el perfil de la impresora, incluido el que
// envió el Backend, y los trabajos del ERP fallaban hasta la siguiente sync.
func TestRenderDPIChangeKeepsBackendProfile(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{Printers: []ports.ManagedPrinter{
		{Name: "HP", Enabled: true, Kind: ports.KindPDF},
	}})
	profiles := &recordingProfiles{m: map[string]dp.PrinterProfile{
		"HP": {PrinterID: "HP", NativeFormats: []dp.DeviceFormat{dp.DevicePDF}},
	}}
	s.d.Profiles = profiles

	manage(t, s, `{"name":"HP","enabled":true,"kind":"pdf","pageMode":"image","renderDPI":450}`)
	if rec := postConfig(t, s, `{"renderDPI":300}`); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/config -> %d %s", rec.Code, rec.Body.String())
	}
	if profiles.writes != 0 {
		t.Errorf("cambiar el DPI escribió %d veces en la caché", profiles.writes)
	}
}

// Cambiar el TIPO tampoco olvida el perfil. La caché solo guarda los del Backend
// (el perfil por defecto de las acciones locales es efímero), así que olvidarlo
// dejaba los trabajos del ERP sin perfil hasta la siguiente sincronización. El
// tipo nuevo vale al instante para las impresoras sin perfil del Backend.
func TestKindChangeKeepsBackendProfile(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{Printers: []ports.ManagedPrinter{
		{Name: "XP-80", Enabled: true, Kind: ports.KindPDF},
		{Name: "Sin perfil", Enabled: true, Kind: ports.KindPDF},
	}})
	backend := dp.PrinterProfile{PrinterID: "XP-80", NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS}, WidthDots: 576}
	profiles := &recordingProfiles{m: map[string]dp.PrinterProfile{"XP-80": backend}}
	s.d.Profiles = profiles

	manage(t, s, `{"name":"XP-80","enabled":true,"kind":"thermal"}`)
	manage(t, s, `{"name":"Sin perfil","enabled":true,"kind":"thermal"}`)

	if profiles.writes != 0 {
		t.Errorf("cambiar el tipo escribió %d veces en la caché", profiles.writes)
	}
	if got := localProfile(s.d.Profiles, s.d.Cfg, "XP-80"); got.WidthDots != backend.WidthDots {
		t.Errorf("XP-80 usa %+v, want el perfil del Backend", got)
	}
	if got := localProfile(s.d.Profiles, s.d.Cfg, "Sin perfil"); profileIsPDF(got) {
		t.Errorf("tras pasar a térmica el perfil por defecto sigue siendo de página: %v", got.NativeFormats)
	}
}
