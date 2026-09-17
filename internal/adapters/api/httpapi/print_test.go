package httpapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appprint "github.com/teraerp/tera-agent/internal/app/print"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// profileStore es una caché de perfiles que cuenta las escrituras.
type profileStore struct {
	m      map[string]dp.PrinterProfile
	writes int
}

func (c *profileStore) Profile(id string) (dp.PrinterProfile, error) {
	if p, ok := c.m[id]; ok {
		return p, nil
	}
	return dp.PrinterProfile{}, fmt.Errorf("sin perfil para %q", id)
}
func (c *profileStore) ProfileOr(id string, fallback dp.PrinterProfile) dp.PrinterProfile {
	if p, ok := c.m[id]; ok && len(p.NativeFormats) > 0 {
		return p
	}
	fallback.PrinterID = id
	return fallback
}
func (c *profileStore) Set(p dp.PrinterProfile) { c.writes++; c.m[p.PrinterID] = p }
func (c *profileStore) SetAll(ps []dp.PrinterProfile) {
	c.writes++
	for _, p := range ps {
		c.m[p.PrinterID] = p
	}
}
func (c *profileStore) Forget(id string) { c.writes++; delete(c.m, id) }

// printSpy es un dp.Resolver que apunta con qué perfil se resolvió el último
// trabajo, sin imprimir nada.
type printSpy struct{ profile dp.PrinterProfile }

func (s *printSpy) Resolve(_ dp.SourceFormat, p dp.PrinterProfile) (dp.Pipeline, error) {
	s.profile = p
	return dp.Pipeline{Renderer: spyRenderer{}, Encoder: spyEncoder{}, Driver: spyDriver{}}, nil
}

type spyRenderer struct{}

func (spyRenderer) CanRender(dp.SourceFormat) bool { return true }
func (spyRenderer) Produces() dp.ArtifactKind      { return dp.ArtifactBytes }
func (spyRenderer) Render(context.Context, []byte, dp.RenderOptions) (dp.Artifact, error) {
	return dp.BytesArtifact{}, nil
}

type spyEncoder struct{}

func (spyEncoder) Accepts() dp.ArtifactKind  { return dp.ArtifactBytes }
func (spyEncoder) Produces() dp.DeviceFormat { return dp.DeviceRaw }
func (spyEncoder) Encode(context.Context, dp.Artifact, dp.EncodeOptions) ([]byte, error) {
	return nil, nil
}

type spyDriver struct{}

func (spyDriver) Accepts(dp.DeviceFormat) bool               { return true }
func (spyDriver) Send(context.Context, string, []byte) error { return nil }

type nopLog struct{}

func (nopLog) Debug(string, ...any) {}
func (nopLog) Info(string, ...any)  {}
func (nopLog) Warn(string, ...any)  {}
func (nopLog) Error(string, ...any) {}

// newPrintServer monta el servicio sobre un motor real que imprime en printSpy.
func newPrintServer(profiles *profileStore) (*Server, *appprint.Engine, *printSpy) {
	spy := &printSpy{}
	engine := appprint.NewEngine(spy, profiles, nopLog{})
	return New(engine, profiles, nil, nopLog{}, ""), engine, spy
}

func postPrint(t *testing.T, s *Server, printer string, paper int) {
	t.Helper()
	body := fmt.Sprintf(`{"printer":%q,"format":"application/pdf","paper":%d,"content":%q}`,
		printer, paper, base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")))
	req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handlePrint(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /print -> %d %s", rec.Code, rec.Body.String())
	}
}

// Regresión: POST /print instalaba un perfil ESC/POS en la caché compartida antes
// de cada trabajo, y una láser con perfil del ERP (native_formats ["pdf"]) pasaba a
// recibir ESC/POS crudo también en los trabajos del ERP.
func TestPrint_KeepsBackendPDFProfile(t *testing.T) {
	const laser = "HP LaserJet"
	profiles := &profileStore{m: map[string]dp.PrinterProfile{
		laser: {PrinterID: laser, NativeFormats: []dp.DeviceFormat{dp.DevicePDF}},
	}}
	s, engine, spy := newPrintServer(profiles)

	postPrint(t, s, laser, 80)
	if got := fmt.Sprint(spy.profile.NativeFormats); got != "[pdf]" {
		t.Errorf("POST /print imprimió con %s, want [pdf] (el perfil del ERP)", got)
	}
	if profiles.writes != 0 {
		t.Errorf("POST /print escribió %d veces en la caché de perfiles", profiles.writes)
	}

	// El siguiente trabajo del ERP a esa láser sigue saliendo por pdf.
	job := dp.PrintJob{PrinterID: laser, Format: dp.FormatPDF, Content: []byte("%PDF-1.4")}
	if err := engine.Print(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(spy.profile.NativeFormats); got != "[pdf]" {
		t.Errorf("el trabajo del ERP imprimió con %s, want [pdf]", got)
	}
}

// Sin perfil, paper/width describen un ESC/POS efímero que no queda en la caché:
// otra petición con otro papel no hereda el ancho de la anterior.
func TestPrint_WithoutProfileUsesRequestPaper(t *testing.T) {
	profiles := &profileStore{m: map[string]dp.PrinterProfile{}}
	s, _, spy := newPrintServer(profiles)

	for _, c := range []struct{ paper, width int }{{58, 384}, {80, 576}} {
		postPrint(t, s, "POS", c.paper)
		if fmt.Sprint(spy.profile.NativeFormats) != "[escpos]" || spy.profile.WidthDots != c.width {
			t.Errorf("paper %d imprimió con %+v, want escpos a %d dots", c.paper, spy.profile, c.width)
		}
	}
	if profiles.writes != 0 || len(profiles.m) != 0 {
		t.Errorf("POST /print dejó perfiles en la caché: %v", profiles.m)
	}
}
