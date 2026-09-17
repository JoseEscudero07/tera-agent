package ui

import (
	"net"
	"net/http"
	"net/url"
)

// El panel escucha siempre en loopback (127.0.0.1:9180), pero eso NO basta para
// protegerlo del navegador: cualquier página web abierta en el mismo equipo
// puede lanzarle peticiones (CSRF), y un dominio que resuelva a 127.0.0.1 pasa a
// ser "same-origin" (DNS rebinding) y podría además leer las respuestas. Como el
// panel guarda la URL del Backend y el Token, un POST malicioso a /api/config
// bastaría para desviar el Token a un servidor del atacante.
//
// La defensa son dos comprobaciones que el navegador rellena y una web atacante
// no puede falsificar:
//   - La cabecera Host debe ser loopback con el puerto del panel. Corta el DNS
//     rebinding: la víctima llegaría con Host "atacante.com:9180", no loopback.
//   - En los métodos que modifican estado, el Origin debe ser el propio panel.
//     Corta el CSRF: el navegador pone el Origin real de la web atacante, que no
//     coincide, y una web no puede sobreescribirlo.

// loopbackHost indica si h (un hostname sin puerto) apunta al propio equipo.
func loopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// hostHeaderAllowed valida la cabecera Host: loopback y, si el panel conoce su
// puerto, el mismo puerto. Sin puerto conocido (addr no parseable) se exige solo
// que sea loopback.
func (s *Server) hostHeaderAllowed(host string) bool {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		// Host sin puerto: solo válido si el panel tampoco tiene puerto conocido.
		return s.port == "" && loopbackHost(host)
	}
	if s.port != "" && p != s.port {
		return false
	}
	return loopbackHost(h)
}

// originAllowed valida el Origin de una petición que modifica estado: debe ser
// http(s) hacia el propio panel (loopback + mismo puerto).
func (s *Server) originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if s.port != "" && u.Port() != s.port {
		return false
	}
	return loopbackHost(u.Hostname())
}

// guard envuelve el mux del panel con las comprobaciones anti-CSRF. Los métodos
// seguros (GET/HEAD) solo validan el Host; los que modifican estado exigen
// además un Origin del propio panel.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostHeaderAllowed(r.Host) {
			http.Error(w, "host no permitido", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			// Seguros: no cambian estado. El WebSocket entra por aquí (GET) y
			// valida su propio Origin en checkWSOrigin.
		default:
			if !s.originAllowed(r.Header.Get("Origin")) {
				http.Error(w, "origen no permitido", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// checkWSOrigin es el CheckOrigin del upgrade del WebSocket del panel. Un cliente
// no-navegador (sin Origin) se acepta; un navegador debe traer el Origin del
// propio panel. Antes se aceptaba cualquier origen, lo que permitía a una web
// abrir el WebSocket del panel de la víctima.
func (s *Server) checkWSOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	return s.originAllowed(o)
}
