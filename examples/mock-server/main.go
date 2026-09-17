// Command mock-server is a minimal reference WebSocket server that implements the
// SERVER side of the Tera Agent protocol (see docs/protocol/). Use it to test the
// Agent's WebSocket client before the Django backend implements it.
//
//	go run ./examples/mock-server --addr 127.0.0.1:8765 --pdf factura.pdf --printer KL200
//
// Point the Agent at it with config.yaml: server.url: "ws://127.0.0.1:8765"
//
// Para probar impresoras "normales" (láser/inyección) tal como las envía el ERP
// —native_formats ["pdf"], 576 dots, 203 dpi, ver apps/tera_agent/consumers.py—
// usa --format pdf. --pdf acepta varios ficheros separados por comas, que se
// envían en orden esperando el resultado de cada uno; --exit termina tras el
// último, que es lo que usa la verificación automática de tools/windows-vm.
//
// Por cada trabajo escribe en stdout una línea legible por scripts:
//
//	RESULT <fichero> <job_completed|job_failed> <segundos> <mensaje JSON>
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	gws "github.com/gorilla/websocket"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8765", "listen address")
	pdf := flag.String("pdf", "", "PDF file(s) to send as print jobs, comma separated (optional)")
	printer := flag.String("printer", "KL200", "target printer id")
	format := flag.String("format", "escpos", "printer profile: escpos (thermal) | pdf (laser, as the ERP sends it)")
	exit := flag.Bool("exit", false, "exit after the last job result instead of keeping the connection open")
	flag.Parse()

	if *format != "escpos" && *format != "pdf" {
		log.Fatalf("--format must be escpos or pdf, got %q", *format)
	}
	var files []string
	for _, f := range strings.Split(*pdf, ",") {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, f)
		}
	}

	up := gws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	done := make(chan struct{})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		serve(c, files, *printer, *format, *exit)
		if *exit {
			close(done)
		}
	})

	go func() {
		log.Printf("mock-server listening on ws://%s  (Ctrl-C to stop)", *addr)
		log.Fatal(http.ListenAndServe(*addr, nil))
	}()
	<-done
}

func serve(c *gws.Conn, files []string, printer, format string, exit bool) {
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

	send(map[string]any{"type": "profiles_sync", "version": 1, "profiles": []any{profileFor(printer, format)}})
	recv() // profiles_ack
	log.Printf("-> profiles_sync (printer=%s, format=%s)", printer, format)

	run := time.Now().Unix()
	for i, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			log.Printf("cannot read pdf: %v", err)
			fmt.Printf("RESULT %s read_error 0 %q\n", filepath.Base(f), err.Error())
			continue
		}
		id := fmt.Sprintf("mock-%d-%d", run, i)
		start := time.Now()
		send(map[string]any{"type": "job", "id": id, "job_type": "print", "payload": map[string]any{
			"printer_id": printer, "format": "application/pdf",
			"content": base64.StdEncoding.EncodeToString(data), "cut": format == "escpos",
		}})
		log.Printf("-> job %s (%d bytes pdf)", id, len(data))

		for {
			m := recv()
			if m == nil {
				fmt.Printf("RESULT %s connection_closed %.1f {}\n", filepath.Base(f), time.Since(start).Seconds())
				return
			}
			if t, _ := m["type"].(string); t == "job_completed" || t == "job_failed" {
				b, _ := json.Marshal(m)
				log.Printf("resultado del job: %v", m)
				fmt.Printf("RESULT %s %s %.1f %s\n", filepath.Base(f), t, time.Since(start).Seconds(), b)
				break
			}
		}
	}

	if exit {
		return
	}
	// Keep the connection open (drain heartbeats) so the Agent stays CONNECTED
	// and we do NOT resend the job on a reconnect.
	log.Printf("jobs done; keeping connection open (Ctrl-C to stop)")
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}

// profileFor devuelve el perfil que enviaría el ERP para la impresora.
func profileFor(printer, format string) map[string]any {
	if format == "pdf" {
		// Impresora "normal": el Agent en Windows la traduce a vectorial o imagen.
		return map[string]any{
			"printer_id": printer, "native_formats": []string{"pdf"}, "width_dots": 576,
			"dpi": 203, "supports_cut": false, "supports_drawer": false, "meta": map[string]any{"tipo": "normal"},
		}
	}
	return map[string]any{
		"printer_id": printer, "native_formats": []string{"escpos"}, "width_dots": 576,
		"dpi": 203, "supports_cut": true, "supports_drawer": false,
	}
}
