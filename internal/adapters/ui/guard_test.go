package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// guardedServer devuelve un panel con el guard montado sobre un handler que
// responde 200, para observar qué peticiones deja pasar el middleware.
func guardedServer(port string) http.Handler {
	s := &Server{port: port}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return s.guard(ok)
}

func TestGuardAllowsSameOriginPost(t *testing.T) {
	h := guardedServer("9180")
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9180/api/config", nil)
	req.Host = "127.0.0.1:9180"
	req.Header.Set("Origin", "http://127.0.0.1:9180")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("un POST del propio panel debe pasar, status = %d", rec.Code)
	}
}

// El vector real: una web abierta en el navegador hace POST al panel. El Host es
// loopback (el navegador conecta a 127.0.0.1) pero el Origin es el de la web
// atacante. Debe rechazarse.
func TestGuardBlocksCrossOriginPost(t *testing.T) {
	h := guardedServer("9180")
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9180/api/config", nil)
	req.Host = "127.0.0.1:9180"
	req.Header.Set("Origin", "https://atacante.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("un POST con Origin ajeno debe dar 403, status = %d", rec.Code)
	}
}

// Un POST sin Origin (una web atacante puede lanzar peticiones "simples" sin que
// el navegador añada Origin en algunos casos, y curl no lo manda) tampoco debe
// modificar estado.
func TestGuardBlocksPostWithoutOrigin(t *testing.T) {
	h := guardedServer("9180")
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9180/api/config", nil)
	req.Host = "127.0.0.1:9180"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("un POST sin Origin debe dar 403, status = %d", rec.Code)
	}
}

// DNS rebinding: la víctima carga atacante.com, que resuelve a 127.0.0.1. El
// navegador conecta al panel pero manda Host "atacante.com:9180". Debe cortarse
// aunque sea un GET, porque de lo contrario el atacante leería /api/status.
func TestGuardBlocksRebindingHost(t *testing.T) {
	h := guardedServer("9180")
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9180/api/status", nil)
	req.Host = "atacante.example:9180"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("un Host no loopback debe dar 403, status = %d", rec.Code)
	}
}

func TestGuardAllowsGetNavigation(t *testing.T) {
	h := guardedServer("9180")
	// Cargar el panel es un GET sin Origin y con Host loopback: debe pasar.
	req := httptest.NewRequest(http.MethodGet, "http://localhost:9180/", nil)
	req.Host = "localhost:9180"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cargar el panel debe pasar, status = %d", rec.Code)
	}
}

// El puerto importa: una web servida en otro puerto del propio equipo no debe
// poder modificar el panel.
func TestGuardBlocksLoopbackOtherPort(t *testing.T) {
	h := guardedServer("9180")
	req := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:9180/api/printers?name=x", nil)
	req.Host = "127.0.0.1:9180"
	req.Header.Set("Origin", "http://127.0.0.1:3000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("un Origin loopback de otro puerto debe dar 403, status = %d", rec.Code)
	}
}

func TestCheckWSOrigin(t *testing.T) {
	s := &Server{port: "9180"}
	cases := []struct {
		origin string
		want   bool
	}{
		{"", true}, // cliente no-navegador (tests, herramientas)
		{"http://127.0.0.1:9180", true},
		{"http://localhost:9180", true},
		{"http://[::1]:9180", true},
		{"https://atacante.example", false},
		{"http://127.0.0.1:3000", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/ws/ui", nil)
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		if got := s.checkWSOrigin(req); got != c.want {
			t.Errorf("checkWSOrigin(%q) = %v; se esperaba %v", c.origin, got, c.want)
		}
	}
}
