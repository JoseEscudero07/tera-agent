// Package httpapi is an inbound adapter: a small local HTTP service that lets a
// backend (e.g. Django) submit print jobs to the Agent over HTTP. It drives the
// same print Engine as the CLI. This complements the future agent-initiated
// WebSocket transport; it is intended for LAN/local integration and testing.
// Owner: Communication Engineer.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/teraerp/tera-agent/internal/app/ports"
	appprint "github.com/teraerp/tera-agent/internal/app/print"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

const maxBody = 32 << 20 // 32 MiB

// Server exposes /health, /printers and /print over HTTP.
type Server struct {
	engine   *appprint.Engine
	profiles dp.FallbackProfileProvider // solo lectura: nunca escribe la caché
	disc     dp.Discovery
	log      ports.Logger
	token    string

	mu sync.Mutex // serializes prints (printers are sequential)
}

// New builds the HTTP print service. If token is non-empty, requests must carry
// "Authorization: Bearer <token>".
func New(engine *appprint.Engine, profiles dp.FallbackProfileProvider, disc dp.Discovery, log ports.Logger, token string) *Server {
	return &Server{engine: engine, profiles: profiles, disc: disc, log: log, token: token}
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/printers", s.auth(s.handlePrinters))
	mux.HandleFunc("/print", s.auth(s.handlePrint))
	return mux
}

// Run starts the server and shuts it down when ctx is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	s.log.Info("print web service listening", "addr", addr, "auth", s.token != "")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// auth wraps a handler with optional bearer-token checking.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && r.Header.Get("Authorization") != "Bearer "+s.token {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handlePrinters(w http.ResponseWriter, r *http.Request) {
	printers, err := s.disc.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "discovery_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"printers": printers})
}

// printRequest is the JSON body of POST /print.
type printRequest struct {
	Printer string `json:"printer"`
	Format  string `json:"format"`  // MIME; default application/pdf
	Content []byte `json:"content"` // base64 in JSON
	Paper   int    `json:"paper"`   // 58 | 80 (mm)
	Width   int    `json:"width"`   // dots; overrides paper
	Cut     *bool  `json:"cut"`     // default true
	Drawer  bool   `json:"drawer"`
}

func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	req, err := parseRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Printer == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "printer is required")
		return
	}
	if len(req.Content) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "empty content")
		return
	}

	width := req.Width
	if width == 0 {
		width = 576
		if req.Paper == 58 {
			width = 384
		}
	}
	cut := true
	if req.Cut != nil {
		cut = *req.Cut
	}
	format := dp.SourceFormat(req.Format)
	if format == "" {
		format = dp.FormatPDF
	}

	// paper/width solo describen el perfil de respaldo: si la impresora ya tiene
	// perfil (el del Backend), manda ese. Antes se instalaba este ESC/POS en la
	// caché compartida antes de cada trabajo, y una láser pasaba a recibir ESC/POS
	// crudo también en los trabajos del ERP.
	profile := s.profiles.ProfileOr(req.Printer, dp.PrinterProfile{
		PrinterID:      req.Printer,
		NativeFormats:  []dp.DeviceFormat{dp.DeviceESCPOS},
		WidthDots:      width,
		DPI:            203,
		SupportsCut:    true,
		SupportsDrawer: true,
	})

	s.mu.Lock()
	err = s.engine.PrintWithProfile(r.Context(), dp.PrintJob{
		PrinterID: req.Printer,
		Format:    format,
		Content:   req.Content,
		Options:   dp.Options{Copies: 1, Cut: cut, OpenDrawer: req.Drawer},
	}, profile)
	s.mu.Unlock()

	if err != nil {
		s.log.Error("http print failed", "printer", req.Printer, "err", err)
		writeErr(w, http.StatusBadGateway, "print_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "printed", "printer": req.Printer})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, code int, key, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]string{"code": key, "message": msg}})
}
