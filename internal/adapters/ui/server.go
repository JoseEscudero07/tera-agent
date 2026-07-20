// Package ui serves the local desktop web UI of the Agent: an embedded SPA plus
// a small JSON/WebSocket API backed by the running agent (state, printers, test
// print, config). It is a local, inbound adapter. Owner: UI Engineer.
package ui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/teraerp/tera-agent/internal/app/ports"
	appprint "github.com/teraerp/tera-agent/internal/app/print"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

//go:embed web/*
var webFS embed.FS

// Deps are the collaborators the UI reads from / acts on.
type Deps struct {
	Machine    *agent.Machine
	Info       *agent.Info
	Discovery  dp.Discovery
	Engine     *appprint.Engine
	Profiles   dp.ProfileCache
	Cfg        ports.Config
	Version    string
	DataDir    string
	Log        ports.Logger
	SaveConfig func(ports.Config) error // optional; persists config changes
}

// Server is the UI HTTP server.
type Server struct {
	d Deps
}

// New builds the UI server.
func New(d Deps) *Server { return &Server{d: d} }

// Run serves the UI until ctx is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/status", s.status)
	mux.HandleFunc("/api/printers", s.printers)
	mux.HandleFunc("/api/history", s.history)
	mux.HandleFunc("/api/logs", s.logs)
	mux.HandleFunc("/api/test-print", s.testPrint)
	mux.HandleFunc("/api/config", s.config)
	mux.HandleFunc("/api/register", s.register)
	mux.HandleFunc("/api/open-data", s.openData)
	mux.HandleFunc("/ws/ui", s.ws)

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	s.d.Log.Info("UI web listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	id, ts := s.d.Info.Snapshot()
	writeJSON(w, map[string]any{
		"registered": s.d.Cfg.Token != "",
		"state":      string(s.d.Machine.Current()),
		"empresa":    id.Empresa,
		"sede":       id.Sucursal,
		"equipo":     firstNonEmpty(id.Equipo, s.d.Cfg.AgentID),
		"lastSync":   humanSince(ts),
		"version":    s.d.Version,
		"os":         runtime.GOOS,
		"dataDir":    s.d.DataDir,
		"serverUrl":  s.d.Cfg.BackendURL,
	})
}

func (s *Server) printers(w http.ResponseWriter, r *http.Request) {
	list, err := s.d.Discovery.List(r.Context())
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, map[string]any{
			"name":    p.Name,
			"status":  string(p.Status),
			"default": p.Name == s.d.Cfg.DefaultPrinter,
		})
	}
	writeJSON(w, out)
}

func (s *Server) history(w http.ResponseWriter, _ *http.Request) {
	// A rich history store is a follow-up; return empty for now.
	writeJSON(w, []any{})
}

func (s *Server) logs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"text": ""})
}

func (s *Server) testPrint(w http.ResponseWriter, r *http.Request) {
	printer := s.d.Cfg.DefaultPrinter
	if printer == "" {
		if ps, _ := s.d.Discovery.List(r.Context()); len(ps) > 0 {
			printer = ps[0].Name
		}
	}
	if printer == "" {
		writeErr(w, "no hay impresora disponible")
		return
	}
	s.d.Profiles.Set(dp.PrinterProfile{
		PrinterID: printer, NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS},
		WidthDots: 576, DPI: 203, SupportsCut: true,
	})
	body := fmt.Sprintf("TERA AGENT\nPrueba de impresion\n%s\n", time.Now().Format("2006-01-02 15:04"))
	err := s.d.Engine.Print(r.Context(), dp.PrintJob{
		PrinterID: printer, Format: dp.FormatText, Content: []byte(body),
		Options: dp.Options{Copies: 1, Cut: true},
	})
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "printer": printer})
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, map[string]any{
			"nombre":  s.d.Cfg.AgentID,
			"url":     s.d.Cfg.BackendURL,
			"log":     s.d.Cfg.LogLevel,
			"dataDir": s.d.DataDir,
		})
		return
	}
	var body struct {
		Nombre string `json:"nombre"`
		URL    string `json:"url"`
		Log    string `json:"log"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.d.Cfg.AgentID = body.Nombre
	s.d.Cfg.BackendURL = body.URL
	if body.Log != "" {
		s.d.Cfg.LogLevel = body.Log
	}
	if s.d.SaveConfig != nil {
		if err := s.d.SaveConfig(s.d.Cfg); err != nil {
			writeErr(w, err.Error())
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true, "note": "los cambios de conexión aplican al reiniciar"})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Token == "" {
		writeErr(w, "token vacío")
		return
	}
	s.d.Cfg.Token = body.Token
	if s.d.SaveConfig != nil {
		if err := s.d.SaveConfig(s.d.Cfg); err != nil {
			writeErr(w, err.Error())
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true, "note": "reinicia el agente para conectar"})
}

func (s *Server) openData(w http.ResponseWriter, _ *http.Request) {
	dir := s.d.DataDir
	if dir == "" {
		writeErr(w, "sin carpeta de datos")
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	_ = cmd.Start()
	writeJSON(w, map[string]any{"ok": true})
}

var upgrader = gws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// ws pushes state changes to the UI (poll-based to avoid observer leaks).
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, e := c.ReadMessage(); e != nil {
				return
			}
		}
	}()

	t := time.NewTicker(time.Second)
	defer t.Stop()
	last := ""
	for {
		st := string(s.d.Machine.Current())
		if st != last {
			last = st
			_, ts := s.d.Info.Snapshot()
			if c.WriteJSON(map[string]any{"type": "state", "state": st, "lastSync": humanSince(ts)}) != nil {
				return
			}
		}
		select {
		case <-done:
			return
		case <-t.C:
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

func humanSince(ts time.Time) string {
	if ts.IsZero() {
		return "—"
	}
	d := time.Since(ts)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("hace %ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("hace %dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("hace %dh", int(d.Hours()))
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
