// Pruebas internas: originHeader no es exportada, así que viven en el paquete
// (el resto de las pruebas del transporte están en package websocket_test).
package websocket

import (
	"strings"
	"testing"
)

// El upgrade debe llevar Origin derivado del propio host del Backend: Django
// Channels, tras AllowedHostsOriginValidator, responde 403 a cualquier handshake
// que no la traiga — antes incluso de resolver la ruta.
func TestOriginHeader(t *testing.T) {
	cases := map[string]string{
		"wss://api.grupotera.cloud/ws/agent/": "https://api.grupotera.cloud",
		"ws://localhost:8000/ws/agent/":       "http://localhost:8000",
		"wss://erp.example.com:8443/ws/":      "https://erp.example.com:8443",
		"http://localhost:9000/ws/":           "http://localhost:9000",
	}
	for in, want := range cases {
		h := originHeader(in)
		if h == nil {
			t.Errorf("originHeader(%q) = nil; se esperaba Origin %q", in, want)
			continue
		}
		if got := h.Get("Origin"); got != want {
			t.Errorf("originHeader(%q) Origin = %q; se esperaba %q", in, got, want)
		}
	}
}

// Sin host no se inventa una cabecera: se deja que el dial falle con su propio
// error, que es más claro que un Origin absurdo.
func TestOriginHeaderWithoutHost(t *testing.T) {
	for _, bad := range []string{"", "://roto", "no-es-una-url"} {
		if h := originHeader(bad); h != nil && h.Get("Origin") != "" {
			t.Errorf("originHeader(%q) devolvió Origin %q; se esperaba ninguna", bad, h.Get("Origin"))
		}
	}
}

// El Origin nunca debe arrastrar el token, ni aunque alguien lo ponga en la query
// de la URL del Backend: solo esquema y host.
func TestOriginHeaderCarriesNoSecret(t *testing.T) {
	h := originHeader("wss://erp.example.com/ws/agent/?token=supersecreto")
	if h == nil {
		t.Fatal("originHeader = nil")
	}
	got := h.Get("Origin")
	if strings.Contains(got, "supersecreto") || strings.Contains(got, "token") {
		t.Errorf("Origin = %q; no debe incluir la query ni el token", got)
	}
	if got != "https://erp.example.com" {
		t.Errorf("Origin = %q; se esperaba https://erp.example.com", got)
	}
}
