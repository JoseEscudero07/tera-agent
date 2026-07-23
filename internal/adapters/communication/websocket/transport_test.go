package websocket_test

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/teraerp/tera-agent/internal/adapters/communication/websocket"
	"github.com/teraerp/tera-agent/internal/adapters/logger"
)

// TestTransport_ConnectsOverWSS verifies the TLS (wss://) path works end to end.
func TestTransport_ConnectsOverWSS(t *testing.T) {
	upgrader := gws.Upgrader{}
	got := make(chan string, 1)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		if _, msg, err := c.ReadMessage(); err == nil {
			got <- string(msg)
		}
	}))
	defer srv.Close()

	url := strings.Replace(srv.URL, "https", "wss", 1)
	tr := websocket.New(url, logger.New("error"), &tls.Config{InsecureSkipVerify: true})

	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("wss connect failed: %v", err)
	}
	defer tr.Close()

	if err := tr.Send(context.Background(), []byte("hola-tls")); err != nil {
		t.Fatalf("send over wss failed: %v", err)
	}

	select {
	case m := <-got:
		if m != "hola-tls" {
			t.Fatalf("server received %q over wss, want hola-tls", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server received nothing over wss")
	}
}

// TestTransport_BadHandshakeSurfacesHTTPStatus verifica que cuando el servidor
// responde algo distinto de 101, Connect devuelve un error que incluye el
// código HTTP y un extracto del cuerpo — sin eso el usuario ve el genérico
// "bad handshake" de gorilla y no puede diagnosticar (401 vs 404 vs 502).
func TestTransport_BadHandshakeSurfacesHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid token"}`)
	}))
	defer srv.Close()

	url := strings.Replace(srv.URL, "http", "ws", 1)
	tr := websocket.New(url, logger.New("error"), nil)

	err := tr.Connect(context.Background())
	if err == nil {
		t.Fatal("expected handshake error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "401") {
		t.Errorf("error should mention HTTP status 401, got: %q", msg)
	}
	if !strings.Contains(msg, "invalid token") {
		t.Errorf("error should include response body, got: %q", msg)
	}
}

// TestTransport_RedactURLHidesToken verifica que la URL que aparece en los
// errores oculta cualquier ?token=... por si un dev copia y pega el log a un
// canal público. Se prueba a través del error real: no queremos exportar la
// función.
func TestTransport_RedactURLHidesToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	url := strings.Replace(srv.URL, "http", "ws", 1) + "/?token=SECRET-XYZ"
	tr := websocket.New(url, logger.New("error"), nil)
	err := tr.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SECRET-XYZ") {
		t.Errorf("token leaked into error: %q", err.Error())
	}
}
