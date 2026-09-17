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

// recordingProfiles cuenta los Forget: un cambio de modo no debe tirar el perfil
// que envió el Backend.
type recordingProfiles struct {
	m       map[string]dp.PrinterProfile
	forgets []string
}

func (c *recordingProfiles) Profile(id string) (dp.PrinterProfile, error) { return c.m[id], nil }
func (c *recordingProfiles) Set(p dp.PrinterProfile)                      { c.m[p.PrinterID] = p }
func (c *recordingProfiles) SetAll(ps []dp.PrinterProfile) {
	for _, p := range ps {
		c.m[p.PrinterID] = p
	}
}
func (c *recordingProfiles) Forget(id string) { c.forgets = append(c.forgets, id) }

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
	if len(profiles.forgets) != 0 {
		t.Errorf("cambiar el modo olvidó perfiles %v; el cache ya aplica el modo al leer", profiles.forgets)
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
	s, _, _ := newTestServer(ports.Config{})
	profiles := &recordingProfiles{m: map[string]dp.PrinterProfile{}}
	s.d.Profiles = profiles
	s.pdfProfile("HP")
	got := profiles.m["HP"].NativeFormats
	if len(got) != 1 || got[0] != dp.DevicePDF {
		t.Errorf("NativeFormats = %v, want [pdf]", got)
	}
	if !profileIsPDF(profiles.m["HP"]) {
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
	if len(profiles.forgets) != 0 {
		t.Errorf("cambiar el DPI olvidó perfiles: %v", profiles.forgets)
	}
}

// Cambiar el TIPO sí debe olvidar el perfil: el anterior era de otra familia.
func TestKindChangeStillForgetsProfile(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{Printers: []ports.ManagedPrinter{
		{Name: "XP-80", Enabled: true, Kind: ports.KindPDF},
	}})
	profiles := &recordingProfiles{m: map[string]dp.PrinterProfile{}}
	s.d.Profiles = profiles
	manage(t, s, `{"name":"XP-80","enabled":true,"kind":"thermal"}`)
	if len(profiles.forgets) != 1 || profiles.forgets[0] != "XP-80" {
		t.Errorf("forgets = %v, want [XP-80]", profiles.forgets)
	}
}
