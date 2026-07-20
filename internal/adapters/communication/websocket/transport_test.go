package websocket_test

import (
	"context"
	"crypto/tls"
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
