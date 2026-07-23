// Package websocket implements comms.Transport over a WebSocket connection using
// gorilla/websocket. Supports ws:// (no TLS) and wss://. Owner: Communication
// Engineer. It knows nothing about printers, devices or UI.
package websocket

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

// handshakeTimeout es lo que damos al servidor para completar el upgrade a
// WebSocket. Backends con cold start (gunicorn/uvicorn dormidos, proxies) a
// veces tardan >15s en el primer intento; 45s deja pasar esos casos sin que
// el usuario vea reintentos aparentes.
const handshakeTimeout = 45 * time.Second

// Transport is a WebSocket client transport. Supports ws:// and wss:// (TLS).
type Transport struct {
	url string
	log ports.Logger
	tls *tls.Config

	mu    sync.Mutex // guards writes (gorilla allows a single concurrent writer)
	conn  *gws.Conn
	inbox chan []byte
	done  chan struct{}
	once  sync.Once
}

// New returns a WebSocket transport dialing url. tlsCfg is used for wss:// (may
// be nil for defaults).
func New(url string, log ports.Logger, tlsCfg *tls.Config) *Transport {
	return &Transport{url: url, log: log, tls: tlsCfg, inbox: make(chan []byte, 32), done: make(chan struct{})}
}

// Connect dials the server and starts the read pump.
//
// Cuando el handshake falla, gorilla devuelve el genérico "bad handshake" y
// además la *http.Response del servidor. La aprovechamos para explicar en el
// log qué respondió de verdad (código HTTP, cuerpo corto) — es la única forma
// de distinguir 401 (token/URL malos), 404 (endpoint mal), 502 (backend
// caído), o un cold start que responde lento pero con éxito al retry.
func (t *Transport) Connect(ctx context.Context) error {
	dialer := gws.Dialer{HandshakeTimeout: handshakeTimeout, TLSClientConfig: t.tls}
	t.log.Debug("ws dial", "url", redactURL(t.url))
	conn, resp, err := dialer.DialContext(ctx, t.url, nil)
	if err != nil {
		return wrapDialError(err, resp, t.url)
	}
	t.conn = conn
	go t.readPump()
	return nil
}

// wrapDialError enriquece el error de gorilla con el estado HTTP y un extracto
// del cuerpo cuando el handshake falla contra un servidor que responde algo
// distinto de 101. El cuerpo se acota a 200 bytes: un error suele venir en un
// JSON corto o un HTML de proxy — más que eso ensucia el log.
func wrapDialError(err error, resp *http.Response, target string) error {
	if resp == nil {
		// Sin respuesta: falló el TCP/TLS antes del HTTP. err ya trae el motivo
		// (dial timeout, x509 error, connection refused).
		return fmt.Errorf("ws %s: %w", redactURL(target), err)
	}
	defer resp.Body.Close()
	// http.Response.Header también puede llevar pistas útiles (Server, WWW-Authenticate).
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	snippet := strings.TrimSpace(string(body))
	if snippet == "" {
		return fmt.Errorf("ws %s: %w (HTTP %d)", redactURL(target), err, resp.StatusCode)
	}
	return fmt.Errorf("ws %s: %w (HTTP %d: %s)", redactURL(target), err, resp.StatusCode, snippet)
}

// redactURL quita fragmentos que podrían contener secretos (token en la query,
// user info) antes de imprimir la URL. La ruta y el host se dejan porque son
// justo lo que necesitas para diagnosticar (¿estás dialando el endpoint
// correcto?).
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = nil
	if q := u.Query(); len(q) > 0 {
		q.Set("token", "REDACTED")
		u.RawQuery = q.Encode()
	}
	return u.String()
}


func (t *Transport) readPump() {
	defer close(t.inbox)
	for {
		_, msg, err := t.conn.ReadMessage()
		if err != nil {
			return
		}
		select {
		case t.inbox <- msg:
		case <-t.done:
			return
		}
	}
}

// Send writes one JSON frame.
func (t *Transport) Send(_ context.Context, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn.WriteMessage(gws.TextMessage, data)
}

// Receive returns the inbound frame channel.
func (t *Transport) Receive() <-chan []byte { return t.inbox }

// Close terminates the connection.
func (t *Transport) Close() error {
	t.once.Do(func() { close(t.done) })
	if t.conn != nil {
		return t.conn.Close()
	}
	return nil
}
