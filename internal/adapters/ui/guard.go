package ui

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"strings"

	gws "github.com/gorilla/websocket"
)

// guard protege el panel local de las webs que se abren en el mismo equipo. El
// panel escucha en loopback y no tiene login, así que cualquier página que el
// navegador del POS visite puede lanzarle peticiones:
//
//   - CSRF: un fetch no-cors con Content-Type text/plain cambiaba la URL del
//     Backend y el agente mandaba el Token a otro servidor al reconectar.
//   - DNS rebinding: un dominio que resuelve a 127.0.0.1 pasa a ser "mismo
//     origen" para el navegador y puede leer y escribir la API.
//
// La cabecera Host frena el rebinding (el navegador manda el dominio del
// atacante); Origin y Content-Type JSON frenan el CSRF, porque el navegador los
// pone siempre en peticiones que no son GET/HEAD y un JSON cruzado exige un
// preflight que el panel nunca responde. Las acciones por GET (imprimir, abrir el
// cajón) se cierran aparte, con métodos por ruta en routes.
type guard struct {
	hosts map[string]struct{} // Host aceptados, en minúsculas y con puerto
}

// newGuard deriva la lista blanca de Host del puerto en el que escucha el panel:
// solo nombres de loopback. Aunque se arranque con --addr 0.0.0.0, el panel no
// tiene autenticación y no debe responder a la red.
func newGuard(addr string) (guard, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return guard{}, fmt.Errorf("dirección del panel inválida %q: %v", addr, err)
	}
	hosts := map[string]struct{}{
		net.JoinHostPort("localhost", port): {},
		net.JoinHostPort("127.0.0.1", port): {},
	}
	if isLoopbackHost(host) {
		hosts[strings.ToLower(net.JoinHostPort(host, port))] = struct{}{}
	}
	return guard{hosts: hosts}, nil
}

// wrap aplica las comprobaciones a todas las peticiones antes de enrutarlas.
func (g guard) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Sin esto, otra web puede cargar el panel en un iframe y hacer que el
		// operador pulse "Desvincular" o "Quitar" creyendo que pulsa otra cosa: la
		// petición saldría del propio panel y pasaría Host y Origin.
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")

		if !g.allowedHost(r) {
			writeErrStatus(w, http.StatusForbidden, "host no permitido")
			return
		}
		if !isSafeMethod(r.Method) {
			if !g.sameOrigin(r) {
				writeErrStatus(w, http.StatusForbidden, "origen no permitido")
				return
			}
			if !isJSON(r) {
				writeErrStatus(w, http.StatusUnsupportedMediaType, "se requiere Content-Type application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// upgrader acepta el WebSocket del panel solo desde el propio panel. El handshake
// es un GET, así que wrap no mira su Origin: lo hace CheckOrigin.
func (g guard) upgrader() gws.Upgrader {
	return gws.Upgrader{CheckOrigin: g.sameOrigin}
}

func (g guard) allowedHost(r *http.Request) bool {
	_, ok := g.hosts[strings.ToLower(r.Host)]
	return ok
}

// sameOrigin exige una cabecera Origin idéntica al origen del panel. Una petición
// sin Origin (o con "null") se rechaza: los navegadores la mandan siempre en
// POST, DELETE y en el handshake WebSocket.
func (g guard) sameOrigin(r *http.Request) bool {
	if !g.allowedHost(r) {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return strings.EqualFold(r.Header.Get("Origin"), scheme+"://"+r.Host)
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead
}

func isJSON(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

// isLoopbackHost indica si host (sin puerto ni corchetes) es el propio equipo.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
