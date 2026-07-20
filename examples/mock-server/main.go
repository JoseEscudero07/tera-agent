// Command mock-server is a minimal reference WebSocket server that implements the
// SERVER side of the Tera Agent protocol (see docs/protocol/). Use it to test the
// Agent's WebSocket client before the Django backend implements it.
//
//	go run ./examples/mock-server --addr 127.0.0.1:8765 --pdf factura.pdf --printer KL200
//
// Point the Agent at it with config.yaml: server.url: "ws://127.0.0.1:8765"
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"

	gws "github.com/gorilla/websocket"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8765", "listen address")
	pdf := flag.String("pdf", "", "PDF file to send as a print job (optional)")
	printer := flag.String("printer", "KL200", "target printer id")
	flag.Parse()

	up := gws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		serve(c, *pdf, *printer)
	})

	log.Printf("mock-server listening on ws://%s  (Ctrl-C to stop)", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func serve(c *gws.Conn, pdf, printer string) {
	send := func(v any) { b, _ := json.Marshal(v); _ = c.WriteMessage(gws.TextMessage, b) }
	recv := func() map[string]any {
		_, raw, err := c.ReadMessage()
		if err != nil {
			return nil
		}
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		log.Printf("<- %v", m["type"])
		return m
	}

	recv() // hello
	recv() // authenticate
	send(map[string]any{"type": "authenticated", "agent_id": "mock", "company_id": "1", "branch_id": "1"})
	recv() // register
	recv() // capabilities

	send(map[string]any{"type": "profiles_sync", "version": 1, "profiles": []any{map[string]any{
		"printer_id": printer, "native_formats": []string{"escpos"}, "width_dots": 576,
		"dpi": 203, "supports_cut": true, "supports_drawer": false,
	}}})
	recv() // profiles_ack
	log.Printf("-> profiles_sync (printer=%s)", printer)

	if pdf != "" {
		data, err := os.ReadFile(pdf)
		if err != nil {
			log.Printf("cannot read pdf: %v", err)
			return
		}
		send(map[string]any{"type": "job", "id": "mock-1", "job_type": "print", "payload": map[string]any{
			"printer_id": printer, "format": "application/pdf",
			"content": base64.StdEncoding.EncodeToString(data), "cut": true,
		}})
		log.Printf("-> job mock-1 (%d bytes pdf)", len(data))
	}

	for {
		m := recv()
		if m == nil {
			return
		}
		if t, _ := m["type"].(string); t == "job_completed" || t == "job_failed" {
			log.Printf("resultado del job: %v", m)
			return
		}
	}
}
