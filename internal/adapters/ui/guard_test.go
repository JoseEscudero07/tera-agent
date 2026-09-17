package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gws "github.com/gorilla/websocket"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

const panelAddr = "127.0.0.1:9180"

// panelHandler monta la pila completa (guard + rutas) tal como la sirve Run.
func panelHandler(t *testing.T, s *Server) http.Handler {
	t.Helper()
	g, err := newGuard(panelAddr)
	if err != nil {
		t.Fatal(err)
	}
	return s.routes(g)
}

// panelRequest simula una petición del propio panel: Host del panel y, si cambia
// estado, Origin del panel y cuerpo JSON.
func panelRequest(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, "http://"+panelAddr+path, strings.NewReader(body))
	if !isSafeMethod(method) {
		r.Header.Set("Origin", "http://"+panelAddr)
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// El escenario del hallazgo: una web visitada en el POS hace
// fetch("http://127.0.0.1:9180/api/config", {method:"POST", mode:"no-cors", body})
// para apuntar el agente a un Backend suyo y recibir el Token al reconectar.
func TestGuardBlocksCrossSiteConfigChange(t *testing.T) {
	const original = "wss://erp.example.com/ws/agent/"
	s, saved, restarted := newTestServer(ports.Config{BackendURL: original, Token: "token-secreto"})
	h := panelHandler(t, s)

	r := httptest.NewRequest(http.MethodPost, "http://"+panelAddr+"/api/config",
		strings.NewReader(`{"url":"wss://evil.example/ws/"}`))
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Content-Type", "text/plain;charset=UTF-8") // lo que pone un fetch no-cors

	if rec := serve(h, r); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
	if saved.BackendURL != "" || s.d.Cfg.BackendURL != original || *restarted {
		t.Errorf("la petición cruzada cambió la config: guardado=%q memoria=%q reinicio=%v",
			saved.BackendURL, s.d.Cfg.BackendURL, *restarted)
	}
}

func TestGuardOriginAndContentType(t *testing.T) {
	cases := []struct {
		name, origin, contentType string
		want                      int
	}{
		{"panel", "http://" + panelAddr, "application/json", http.StatusOK},
		{"panel con charset", "http://" + panelAddr, "application/json; charset=utf-8", http.StatusOK},
		{"sin Origin", "", "application/json", http.StatusForbidden},
		{"Origin null", "null", "application/json", http.StatusForbidden},
		{"otra web", "https://evil.example", "application/json", http.StatusForbidden},
		{"mismo host por https", "https://" + panelAddr, "application/json", http.StatusForbidden},
		{"localhost contra Host 127.0.0.1", "http://localhost:9180", "application/json", http.StatusForbidden},
		{"otro puerto local", "http://127.0.0.1:3000", "application/json", http.StatusForbidden},
		{"sin Content-Type", "http://" + panelAddr, "", http.StatusUnsupportedMediaType},
		{"text/plain", "http://" + panelAddr, "text/plain;charset=UTF-8", http.StatusUnsupportedMediaType},
		{"formulario", "http://" + panelAddr, "application/x-www-form-urlencoded", http.StatusUnsupportedMediaType},
		{"multipart", "http://" + panelAddr, "multipart/form-data; boundary=x", http.StatusUnsupportedMediaType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestServer(ports.Config{})
			r := httptest.NewRequest(http.MethodPost, "http://"+panelAddr+"/api/config", strings.NewReader(`{"log":"debug"}`))
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.contentType != "" {
				r.Header.Set("Content-Type", tc.contentType)
			}
			rec := serve(panelHandler(t, s), r)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
			if changed := s.d.Cfg.LogLevel == "debug"; changed != (tc.want == http.StatusOK) {
				t.Errorf("config modificada = %v con status %d", changed, rec.Code)
			}
		})
	}
}

// DNS rebinding: el navegador manda el dominio del atacante en Host. Se corta
// en todo, estáticos incluidos, porque tras el rebinding también podría leer.
func TestGuardRejectsForeignHost(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{})
	h := panelHandler(t, s)
	for _, host := range []string{"evil.example:9180", "127.0.0.1:9181", "192.168.1.20:9180", "localhost", ""} {
		for _, path := range []string{"/", "/api/status", "/api/config"} {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.Host = host
			if rec := serve(h, r); rec.Code != http.StatusForbidden {
				t.Errorf("GET %s con Host %q = %d, want 403", path, host, rec.Code)
			}
		}
	}
	for _, host := range []string{"127.0.0.1:9180", "localhost:9180", "LOCALHOST:9180"} {
		r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		r.Host = host
		if rec := serve(h, r); rec.Code != http.StatusOK {
			t.Errorf("GET /api/status con Host %q = %d, want 200", host, rec.Code)
		}
	}
}

// Un GET no lleva Origin, así que guard no lo distingue del panel: las acciones
// tienen que exigir su método. Antes <img src=".../api/printers/drawer"> abría
// el cajón desde cualquier web.
func TestStateChangingRoutesRejectOtherMethods(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{BackendURL: "wss://erp.example.com/ws/", Token: "t"})
	h := panelHandler(t, s)
	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/printers/drawer"},
		{http.MethodGet, "/api/test-print"},
		{http.MethodGet, "/api/open-data"},
		{http.MethodGet, "/api/register"},
		{http.MethodGet, "/api/unregister"},
		{http.MethodGet, "/api/printers/manage"},
		{http.MethodGet, "/api/printers/default"},
		{http.MethodPost, "/api/printers"},
		{http.MethodPut, "/api/config"},
		{http.MethodDelete, "/api/config"},
		{http.MethodPost, "/api/status"},
		{http.MethodDelete, "/api/history"},
		{http.MethodPost, "/"},
	}
	// Un GET a una acción cae en los estáticos (GET /) y da 404; el resto de
	// métodos, 405. Lo que importa es que el handler no se ejecute: el del cajón o
	// el de prueba entrarían en pánico sin Engine, y register/open-data darían 502.
	for _, tc := range cases {
		rec := serve(h, panelRequest(tc.method, tc.path, "{}"))
		if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 405 o 404", tc.method, tc.path, rec.Code)
		}
	}
	if s.d.Cfg.Token != "t" {
		t.Error("una ruta con método incorrecto llegó a ejecutarse")
	}
}

// Las peticiones que hace el panel siguen funcionando con la pila completa.
func TestPanelRequestsStillWork(t *testing.T) {
	s, saved, _ := newTestServer(ports.Config{
		DefaultPrinter: "POS",
		Printers:       []ports.ManagedPrinter{{Name: "POS", Enabled: true}, {Name: "HP", Enabled: true}},
	})
	s.d.Profiles = &recordingProfiles{m: map[string]dp.PrinterProfile{}}
	s.d.Discovery = fakeDiscovery{names: []string{"POS", "HP"}}
	h := panelHandler(t, s)

	for _, r := range []*http.Request{
		panelRequest(http.MethodGet, "/", ""),
		panelRequest(http.MethodGet, "/api/printers", ""),
		panelRequest(http.MethodPost, "/api/printers/default", `{"name":"HP"}`),
		panelRequest(http.MethodPost, "/api/printers/manage", `{"name":"HP","enabled":true,"kind":"pdf"}`),
		panelRequest(http.MethodDelete, "/api/printers?name=POS", ""),
	} {
		if rec := serve(h, r); rec.Code != http.StatusOK {
			t.Errorf("%s %s = %d (body %s)", r.Method, r.URL, rec.Code, rec.Body.String())
		}
	}
	if saved.DefaultPrinter != "HP" || len(saved.Printers) != 1 || saved.Printers[0].Name != "HP" {
		t.Errorf("config guardada = default %q, impresoras %v", saved.DefaultPrinter, saved.Printers)
	}
}

// Sin estas cabeceras otra web puede cargar el panel en un iframe y hacer que el
// operador pulse "Desvincular" sin saberlo.
func TestGuardSetsAntiFramingHeaders(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{})
	rec := serve(panelHandler(t, s), panelRequest(http.MethodGet, "/", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}
	want := map[string]string{
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "frame-ancestors 'none'",
		"X-Content-Type-Options":  "nosniff",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestNewGuardHosts(t *testing.T) {
	cases := []struct {
		addr     string
		allow    []string
		disallow []string
	}{
		{"127.0.0.1:9180", []string{"127.0.0.1:9180", "localhost:9180"}, []string{"[::1]:9180", "127.0.0.1:9181"}},
		{"localhost:7000", []string{"127.0.0.1:7000", "localhost:7000"}, []string{"localhost:9180"}},
		{"[::1]:9180", []string{"[::1]:9180", "localhost:9180"}, []string{"[::2]:9180"}},
		// Escuchar en todas las interfaces no abre el panel a la red.
		{"0.0.0.0:9180", []string{"127.0.0.1:9180", "localhost:9180"}, []string{"0.0.0.0:9180", "192.168.1.20:9180"}},
	}
	for _, tc := range cases {
		g, err := newGuard(tc.addr)
		if err != nil {
			t.Fatalf("newGuard(%q): %v", tc.addr, err)
		}
		for _, h := range tc.allow {
			if !g.allowedHost(&http.Request{Host: h}) {
				t.Errorf("addr %s: Host %q rechazado", tc.addr, h)
			}
		}
		for _, h := range tc.disallow {
			if g.allowedHost(&http.Request{Host: h}) {
				t.Errorf("addr %s: Host %q aceptado", tc.addr, h)
			}
		}
	}
	for _, bad := range []string{"", "127.0.0.1", "sin-puerto"} {
		if _, err := newGuard(bad); err == nil {
			t.Errorf("newGuard(%q) = nil; se esperaba error", bad)
		}
	}
}

// El WebSocket del panel, con un servidor real: solo el propio panel conecta.
func TestWebSocketRequiresPanelOrigin(t *testing.T) {
	s, _, _ := newTestServer(ports.Config{})
	ts := httptest.NewUnstartedServer(nil)
	addr := ts.Listener.Addr().String()
	g, err := newGuard(addr)
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = s.routes(g)
	ts.Start()
	defer ts.Close()

	wsURL := "ws://" + addr + "/ws/ui"
	rejected := []http.Header{
		{},                                   // sin Origin
		{"Origin": {"https://evil.example"}}, // otra web
		{"Origin": {"null"}},                 // iframe sandbox / file://
		{"Origin": {"https://" + addr}},      // mismo host, otro esquema
		{"Origin": {"http://evil.example"}, "Host": {"evil.example"}}, // rebinding
	}
	for _, hdr := range rejected {
		c, resp, err := gws.DefaultDialer.Dial(wsURL, hdr)
		if err == nil {
			c.Close()
			t.Errorf("handshake con %v aceptado; se esperaba 403", hdr)
			continue
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Errorf("handshake con %v: resp=%v err=%v; se esperaba 403", hdr, resp, err)
		}
	}

	c, _, err := gws.DefaultDialer.Dial(wsURL, http.Header{"Origin": {"http://" + addr}})
	if err != nil {
		t.Fatalf("el panel no puede abrir su WebSocket: %v", err)
	}
	defer c.Close()
	var msg map[string]any
	if err := c.ReadJSON(&msg); err != nil || msg["type"] != "state" {
		t.Errorf("primer mensaje = %v, err %v; se esperaba type=state", msg, err)
	}
}
