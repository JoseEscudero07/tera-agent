package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

// readOnlyServer es un panel en modo servicio: la conexión (URL y Token) no debe
// poder cambiarse desde aquí, solo con `tera-agent register`.
func readOnlyServer(cfg ports.Config) (*Server, *ports.Config) {
	s, saved, _ := newTestServer(cfg)
	s.d.ConnReadOnly = true
	return s, saved
}

func TestServiceModeBlocksRegister(t *testing.T) {
	s, _ := readOnlyServer(ports.Config{})
	rec := httptest.NewRecorder()
	s.register(rec, httptest.NewRequest(http.MethodPost, "/api/register",
		strings.NewReader(`{"token":"t","url":"wss://x/ws/"}`)))
	if rec.Code == http.StatusOK {
		t.Fatalf("el registro por panel debe bloquearse en modo servicio, status = %d", rec.Code)
	}
}

func TestServiceModeBlocksUnregister(t *testing.T) {
	s, saved := readOnlyServer(ports.Config{BackendURL: "wss://x/ws/", Token: "tok"})
	rec := httptest.NewRecorder()
	s.unregister(rec, httptest.NewRequest(http.MethodPost, "/api/unregister", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("desvincular debe bloquearse en modo servicio, status = %d", rec.Code)
	}
	if saved.Token != "" {
		// saved solo cambia si se llamó SaveConfig; no debe haberse llamado.
		t.Errorf("no debe persistir nada: saved.Token = %q", saved.Token)
	}
}

func TestServiceModeBlocksTokenChange(t *testing.T) {
	s, saved := readOnlyServer(ports.Config{BackendURL: "wss://x/ws/", Token: "viejo"})
	rec := postConfig(t, s, `{"url":"wss://x/ws/","token":"nuevo"}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("cambiar el Token debe bloquearse en modo servicio, status = %d", rec.Code)
	}
	if saved.Token == "nuevo" {
		t.Errorf("el Token no debía cambiar")
	}
}

func TestServiceModeBlocksURLChange(t *testing.T) {
	s, _ := readOnlyServer(ports.Config{BackendURL: "wss://viejo/ws/", Token: "t"})
	rec := postConfig(t, s, `{"url":"wss://nuevo/ws/"}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("cambiar la URL debe bloquearse en modo servicio, status = %d", rec.Code)
	}
}

// La calibración SÍ se puede guardar en modo servicio: el panel manda la URL
// actual (sin cambio) y ningún token, así que el guardado debe pasar y persistir.
func TestServiceModeAllowsCalibrationSave(t *testing.T) {
	s, saved := readOnlyServer(ports.Config{BackendURL: "wss://x/ws/", Token: "t"})
	rec := postConfig(t, s, `{"url":"wss://x/ws/","cutFeedDots":200}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("guardar calibración debe funcionar en modo servicio, status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if saved.CutFeedDots != 200 {
		t.Errorf("CutFeedDots = %d; se esperaba 200", saved.CutFeedDots)
	}
	if saved.Token != "t" || saved.BackendURL != "wss://x/ws/" {
		t.Errorf("la conexión no debía tocarse: url=%q token=%q", saved.BackendURL, saved.Token)
	}
}
