package ui

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teraerp/tera-agent/internal/app/ports"
	appprint "github.com/teraerp/tera-agent/internal/app/print"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// printSpy es un dp.Resolver que apunta con qué perfil se resolvió el último
// trabajo y qué documento llegó al renderer, sin imprimir nada.
type printSpy struct {
	calls   int
	profile dp.PrinterProfile
	format  dp.SourceFormat
	content []byte
}

func (s *printSpy) Resolve(f dp.SourceFormat, p dp.PrinterProfile) (dp.Pipeline, error) {
	s.calls++
	s.profile, s.format = p, f
	return dp.Pipeline{Renderer: spyRenderer{s}, Encoder: spyEncoder{}, Driver: spyDriver{}}, nil
}

type spyRenderer struct{ s *printSpy }

func (spyRenderer) CanRender(dp.SourceFormat) bool { return true }
func (spyRenderer) Produces() dp.ArtifactKind      { return dp.ArtifactBytes }
func (r spyRenderer) Render(_ context.Context, content []byte, _ dp.RenderOptions) (dp.Artifact, error) {
	r.s.content = content
	return dp.BytesArtifact{Data: content}, nil
}

type spyEncoder struct{}

func (spyEncoder) Accepts() dp.ArtifactKind  { return dp.ArtifactBytes }
func (spyEncoder) Produces() dp.DeviceFormat { return dp.DeviceRaw }
func (spyEncoder) Encode(_ context.Context, a dp.Artifact, _ dp.EncodeOptions) ([]byte, error) {
	return a.(dp.BytesArtifact).Data, nil
}

type spyDriver struct{}

func (spyDriver) Accepts(dp.DeviceFormat) bool               { return true }
func (spyDriver) Send(context.Context, string, []byte) error { return nil }

type nopLog struct{}

func (nopLog) Debug(string, ...any) {}
func (nopLog) Info(string, ...any)  {}
func (nopLog) Warn(string, ...any)  {}
func (nopLog) Error(string, ...any) {}

// withSpyEngine conecta al server la caché dada y un motor real sobre printSpy.
func withSpyEngine(s *Server, profiles *recordingProfiles) *printSpy {
	spy := &printSpy{}
	s.d.Profiles = profiles
	s.d.Engine = appprint.NewEngine(spy, profiles, nopLog{})
	return spy
}

func assertDeviceFormats(t *testing.T, what string, p dp.PrinterProfile, want ...dp.DeviceFormat) {
	t.Helper()
	if fmt.Sprint(p.NativeFormats) != fmt.Sprint(want) {
		t.Errorf("%s imprimió con NativeFormats %v, want %v", what, p.NativeFormats, want)
	}
}

// Regresión: "Probar" (bandeja o panel) instalaba un perfil ESC/POS en la caché
// compartida y los trabajos del ERP a una láser salían como ESC/POS crudo hasta la
// siguiente sincronización. La láser no está gestionada en el panel, así que su
// tipo local cae a térmica: aun así manda el perfil del ERP.
func TestProbarKeepsBackendPDFProfile(t *testing.T) {
	const laser = "HP LaserJet"
	probar := map[string]func(*Server) error{
		"bandeja": func(s *Server) error {
			return PrintTestPage(context.Background(), s.d.Engine, s.d.Profiles, s.d.Cfg, laser)
		},
		"panel": func(s *Server) error {
			rec := httptest.NewRecorder()
			s.testPrint(rec, httptest.NewRequest(http.MethodPost, "/api/test-print", strings.NewReader(`{"name":"`+laser+`"}`)))
			if rec.Code != http.StatusOK {
				return fmt.Errorf("POST /api/test-print -> %d %s", rec.Code, rec.Body.String())
			}
			return nil
		},
	}
	for name, run := range probar {
		t.Run(name, func(t *testing.T) {
			s, _, _ := newTestServer(ports.Config{})
			profiles := &recordingProfiles{m: map[string]dp.PrinterProfile{
				laser: {PrinterID: laser, NativeFormats: []dp.DeviceFormat{dp.DevicePDF}},
			}}
			spy := withSpyEngine(s, profiles)

			if err := run(s); err != nil {
				t.Fatal(err)
			}
			if spy.format != dp.FormatPDF || !bytes.Equal(spy.content, testPagePDF) {
				t.Errorf("Probar envió %q; a una láser le toca la página PDF de prueba", spy.format)
			}
			assertDeviceFormats(t, "Probar", spy.profile, dp.DevicePDF)
			if n := profiles.writes(); n != 0 {
				t.Errorf("Probar escribió %d veces en la caché de perfiles", n)
			}

			// El siguiente trabajo del ERP a esa láser sigue saliendo por pdf.
			job := dp.PrintJob{PrinterID: laser, Format: dp.FormatPDF, Content: []byte("%PDF-1.4")}
			if err := s.d.Engine.Print(context.Background(), job); err != nil {
				t.Fatal(err)
			}
			assertDeviceFormats(t, "el trabajo del ERP", spy.profile, dp.DevicePDF)
		})
	}
}

// Regresión: en una térmica de 58 mm con perfil del ERP, "Probar" instalaba encima
// el ESC/POS por defecto de 80 mm (576 dots).
func TestProbarKeepsBackendThermalWidth(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{})
	profiles := &recordingProfiles{m: map[string]dp.PrinterProfile{
		"POS-58": {PrinterID: "POS-58", NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS}, WidthDots: 384, DPI: 203, SupportsCut: true},
	}}
	spy := withSpyEngine(s, profiles)

	if err := PrintTestPage(context.Background(), s.d.Engine, s.d.Profiles, s.d.Cfg, "POS-58"); err != nil {
		t.Fatal(err)
	}
	if spy.format != dp.FormatText || spy.profile.WidthDots != 384 {
		t.Errorf("Probar imprimió %q a %d dots, want texto a 384", spy.format, spy.profile.WidthDots)
	}
	if n := profiles.writes(); n != 0 || profiles.m["POS-58"].WidthDots != 384 {
		t.Errorf("el perfil del ERP cambió (%d escrituras): %+v", n, profiles.m["POS-58"])
	}
}

// Sin perfil del Backend, "Probar" usa un perfil efímero según el tipo declarado
// en el panel y no lo deja en la caché.
func TestProbarWithoutProfileUsesDeclaredKind(t *testing.T) {
	cfg := ports.Config{Printers: []ports.ManagedPrinter{{Name: "HP", Enabled: true, Kind: ports.KindPDF}}}
	cases := []struct {
		printer string
		format  dp.SourceFormat
		device  dp.DeviceFormat
	}{
		{"HP", dp.FormatPDF, dp.DevicePDF},      // gestionada como pdf
		{"POS", dp.FormatText, dp.DeviceESCPOS}, // sin gestionar: térmica
	}
	for _, c := range cases {
		t.Run(c.printer, func(t *testing.T) {
			s, _, _ := newTestServer(cfg)
			profiles := &recordingProfiles{m: map[string]dp.PrinterProfile{}}
			spy := withSpyEngine(s, profiles)

			if err := PrintTestPage(context.Background(), s.d.Engine, s.d.Profiles, s.d.Cfg, c.printer); err != nil {
				t.Fatal(err)
			}
			if spy.format != c.format || spy.profile.PrinterID != c.printer {
				t.Errorf("Probar imprimió %q con %+v", spy.format, spy.profile)
			}
			assertDeviceFormats(t, "Probar", spy.profile, c.device)
			if len(profiles.m) != 0 || profiles.writes() != 0 {
				t.Errorf("el perfil por defecto quedó en la caché: %v", profiles.m)
			}
		})
	}
}

// El cajón tampoco escribe la caché: usa el perfil del ERP si existe (una láser no
// tiene cajón) o el ESC/POS efímero.
func TestDrawerDoesNotWriteProfiles(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{})
	profiles := &recordingProfiles{m: map[string]dp.PrinterProfile{
		"HP LaserJet": {PrinterID: "HP LaserJet", NativeFormats: []dp.DeviceFormat{dp.DevicePDF}},
	}}
	spy := withSpyEngine(s, profiles)
	drawer := func(name string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.printersDrawer(rec, httptest.NewRequest(http.MethodPost, "/api/printers/drawer", strings.NewReader(`{"name":"`+name+`"}`)))
		return rec
	}

	if rec := drawer("HP LaserJet"); rec.Code == http.StatusOK || spy.calls != 0 {
		t.Errorf("abrir cajón en una láser: %d, %d impresiones; want error sin imprimir", rec.Code, spy.calls)
	}
	if rec := drawer("POS"); rec.Code != http.StatusOK {
		t.Fatalf("abrir cajón en POS -> %d %s", rec.Code, rec.Body.String())
	}
	assertDeviceFormats(t, "el cajón", spy.profile, dp.DeviceESCPOS)
	if profiles.writes() != 0 || len(profiles.m) != 1 {
		t.Errorf("el cajón escribió en la caché: %v", profiles.m)
	}
}
