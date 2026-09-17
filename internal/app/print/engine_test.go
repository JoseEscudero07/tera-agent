package print

import (
	"context"
	"errors"
	"testing"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// recordingResolver resuelve siempre al mismo pipeline de fakes y apunta con qué
// perfil se le pidió resolver.
type recordingResolver struct{ profile dp.PrinterProfile }

func (r *recordingResolver) Resolve(_ dp.SourceFormat, p dp.PrinterProfile) (dp.Pipeline, error) {
	r.profile = p
	return dp.Pipeline{
		Renderer: fakeRenderer{src: dp.FormatText, produces: dp.ArtifactText},
		Encoder:  fakeEncoder{accepts: dp.ArtifactText, produces: dp.DeviceESCPOS},
		Driver:   fakeDriver{accepts: dp.DeviceESCPOS},
	}, nil
}

// failingProvider falla el test si alguien consulta la caché.
type failingProvider struct{ t *testing.T }

func (p failingProvider) Profile(id string) (dp.PrinterProfile, error) {
	p.t.Errorf("PrintWithProfile consultó la caché para %q", id)
	return dp.PrinterProfile{}, errors.New("no debería consultarse")
}

type nopLog struct{}

func (nopLog) Debug(string, ...any) {}
func (nopLog) Info(string, ...any)  {}
func (nopLog) Warn(string, ...any)  {}
func (nopLog) Error(string, ...any) {}

func TestEngine_PrintWithProfileUsesGivenProfileOnly(t *testing.T) {
	res := &recordingResolver{}
	e := NewEngine(res, failingProvider{t}, nopLog{})
	want := dp.PrinterProfile{PrinterID: "POS", NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS}, WidthDots: 384}

	if err := e.PrintWithProfile(context.Background(), dp.PrintJob{PrinterID: "POS", Format: dp.FormatText}, want); err != nil {
		t.Fatal(err)
	}
	if res.profile.WidthDots != 384 {
		t.Errorf("resolvió con %+v, want el perfil explícito", res.profile)
	}
}

// Print sigue leyendo el perfil de la caché (camino de los trabajos del ERP).
func TestEngine_PrintResolvesCachedProfile(t *testing.T) {
	res := &recordingResolver{}
	cached := dp.PrinterProfile{PrinterID: "POS", NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS}, WidthDots: 576}
	e := NewEngine(res, staticProvider{cached}, nopLog{})

	if err := e.Print(context.Background(), dp.PrintJob{PrinterID: "POS", Format: dp.FormatText}); err != nil {
		t.Fatal(err)
	}
	if res.profile.WidthDots != 576 {
		t.Errorf("resolvió con %+v, want el perfil cacheado", res.profile)
	}
}

type staticProvider struct{ p dp.PrinterProfile }

func (s staticProvider) Profile(string) (dp.PrinterProfile, error) { return s.p, nil }
