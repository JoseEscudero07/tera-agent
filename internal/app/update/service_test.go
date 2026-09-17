package update

import (
	"context"
	"errors"
	"testing"

	du "github.com/teraerp/tera-agent/internal/domain/update"
)

type fakeSource struct {
	m   du.Manifest
	err error
}

func (f fakeSource) Fetch(context.Context, string, string, string) (du.Manifest, error) {
	return f.m, f.err
}

type fakeApplier struct {
	called bool
	err    error
}

func (a *fakeApplier) Apply(context.Context, du.Manifest) error { a.called = true; return a.err }

type nopLog struct{}

func (nopLog) Debug(string, ...any) {}
func (nopLog) Info(string, ...any)  {}
func (nopLog) Warn(string, ...any)  {}
func (nopLog) Error(string, ...any) {}

func TestCheckAvailable(t *testing.T) {
	s := New("1.2.0", fakeSource{m: du.Manifest{Latest: "1.3.0", Notes: "nuevo"}}, &fakeApplier{}, nopLog{})
	st, err := s.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Available || st.Latest != "1.3.0" || st.Current != "1.2.0" {
		t.Fatalf("status inesperado: %+v", st)
	}
	if got := s.Cached(); got.Latest != "1.3.0" {
		t.Errorf("cache no actualizada: %+v", got)
	}
}

func TestMandatoryByMinSupported(t *testing.T) {
	s := New("1.0.0", fakeSource{m: du.Manifest{Latest: "1.3.0", MinSupported: "1.2.0"}}, &fakeApplier{}, nopLog{})
	st, _ := s.Check(context.Background())
	if !st.Mandatory {
		t.Error("1.0.0 < min_supported 1.2.0 debe ser mandatory")
	}
}

func TestApplyNothingNewer(t *testing.T) {
	ap := &fakeApplier{}
	s := New("1.3.0", fakeSource{m: du.Manifest{Latest: "1.3.0"}}, ap, nopLog{})
	if err := s.Apply(context.Background()); err == nil {
		t.Error("aplicar con misma versión debe fallar")
	}
	if ap.called {
		t.Error("no debió llamar al Applier")
	}
}

func TestApplyRequiresHash(t *testing.T) {
	ap := &fakeApplier{}
	// Hay versión nueva pero sin SHA256 → no se aplica (verificación obligatoria).
	s := New("1.2.0", fakeSource{m: du.Manifest{Latest: "1.3.0", URL: "https://x/bin"}}, ap, nopLog{})
	if err := s.Apply(context.Background()); err == nil {
		t.Error("aplicar sin SHA256 debe fallar")
	}
	if ap.called {
		t.Error("no debió llamar al Applier sin hash")
	}
}

func TestApplyOK(t *testing.T) {
	ap := &fakeApplier{}
	s := New("1.2.0", fakeSource{m: du.Manifest{Latest: "1.3.0", URL: "https://x/bin", SHA256: "abc"}}, ap, nopLog{})
	if err := s.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !ap.called {
		t.Error("debió llamar al Applier")
	}
}

func TestCheckErrorReturnsCached(t *testing.T) {
	s := New("1.2.0", fakeSource{err: errors.New("red caída")}, &fakeApplier{}, nopLog{})
	if _, err := s.Check(context.Background()); err == nil {
		t.Error("error de red debe propagarse")
	}
}
