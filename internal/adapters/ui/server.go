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
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/teraerp/tera-agent/internal/app/ports"
	appprint "github.com/teraerp/tera-agent/internal/app/print"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

//go:embed web/*
var webFS embed.FS

//go:embed assets/testpage.pdf
var testPagePDF []byte

// Deps are the collaborators the UI reads from / acts on.
type Deps struct {
	Machine   *agent.Machine
	Info      *agent.Info
	Discovery dp.Discovery
	Engine    *appprint.Engine
	// Profiles es de solo lectura a propósito: el panel nunca escribe la caché de
	// perfiles, que solo guarda los del Backend (ver localProfile).
	Profiles   dp.FallbackProfileProvider
	Cfg        ports.Config
	Version    string
	DataDir    string
	Log        ports.Logger
	SaveConfig func(ports.Config) error // optional; persists config changes
	// ApplyTuning, if set, re-applies the cut calibration to the running engine
	// so panel changes take effect immediately (no restart needed).
	ApplyTuning func(ports.Config)
	// OnRegistered, if set, is called after a successful graphical registration
	// so the agent can apply the new credentials (e.g. restart/reconnect).
	OnRegistered func()
	// RunMode describes how this process was started ("servicio" / "app de
	// usuario"). El panel lo muestra como texto de solo lectura: quién controla el
	// arranque lo decide el instalador, no la configuración, así que un
	// interruptor aquí solo podría mentir.
	RunMode string
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
	mux.HandleFunc("/api/printers/manage", s.printersManage)
	mux.HandleFunc("/api/printers/default", s.printersDefault)
	mux.HandleFunc("/api/printers/drawer", s.printersDrawer)
	mux.HandleFunc("/api/history", s.history)
	mux.HandleFunc("/api/logs", s.logs)
	mux.HandleFunc("/api/test-print", s.testPrint)
	mux.HandleFunc("/api/config", s.config)
	mux.HandleFunc("/api/register", s.register)
	mux.HandleFunc("/api/unregister", s.unregister)
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
	// Un equipo se considera registrado solo si tiene Token *y* URL del servidor:
	// un config con token viejo pero sin URL (o vice versa) muestra la pantalla
	// de registro en lugar de un panel vacío.
	writeJSON(w, map[string]any{
		"registered": s.d.Cfg.Token != "" && s.d.Cfg.BackendURL != "",
		"state":      string(s.d.Machine.Current()),
		"empresa":    id.Empresa,
		"sede":       id.Sucursal,
		"equipo":     firstNonEmpty(id.Equipo, s.d.Cfg.AgentID),
		"agentId":    s.d.Cfg.AgentID,
		"lastSync":   humanSince(ts),
		"version":    s.d.Version,
		"os":         runtime.GOOS,
		"dataDir":    s.d.DataDir,
		"serverUrl":  s.d.Cfg.BackendURL,
		// Defaults globales de calibración (0 = usar el default interno del agente).
		"cutFeedDots":   s.d.Cfg.CutFeedDots,
		"topMarginDots": s.d.Cfg.TopMarginDots,
	})
}

// printers lists discovered printers merged with their agent-local managed
// state (GET) or removes a printer from the managed list (DELETE ?name=).
func (s *Server) printers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		name := r.URL.Query().Get("name")
		if name == "" {
			writeErr(w, "falta el nombre de la impresora")
			return
		}
		s.d.Cfg.Printers = removeManaged(s.d.Cfg.Printers, name)
		if name == s.d.Cfg.DefaultPrinter {
			s.d.Cfg.DefaultPrinter = ""
		}
		if err := s.persist(); err != nil {
			writeErr(w, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	list, err := s.d.Discovery.List(r.Context())
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		mp, managed := findManaged(s.d.Cfg.Printers, p.Name)
		enabled := true // discovered-but-unmanaged printers are usable by default
		role := ""
		if managed {
			enabled = mp.Enabled
			role = string(mp.Role)
		}
		kind := ""
		if managed {
			kind = string(mp.Kind)
		}
		if kind == "" {
			kind = string(ports.KindThermal)
		}
		out = append(out, map[string]any{
			"name":          p.Name,
			"status":        string(p.Status),
			"default":       p.Name == s.d.Cfg.DefaultPrinter,
			"managed":       managed,
			"enabled":       enabled,
			"role":          role,
			"kind":          kind,
			"cutFeedDots":   mp.CutFeedDots,   // 0 = usar default
			"topMarginDots": mp.TopMarginDots, // 0 = usar default
			"pageMarginMM":  mp.PageMarginMM,  // 0 = usar default (impresoras de hoja)
			"renderDPI":     mp.RenderDPI,     // 0 = usar default
			// vector | image. Se resuelve con el mismo helper que usa la impresión,
			// para que el panel muestre el modo con el que de verdad se imprime.
			"pageMode": string(s.d.Cfg.PageModeOf(p.Name)),
		})
	}
	writeJSON(w, out)
}

// printersManage upserts one printer's managed state (enabled, role, kind and
// the per-printer cut calibration).
func (s *Server) printersManage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string  `json:"name"`
		Role          string  `json:"role"`
		Enabled       bool    `json:"enabled"`
		Kind          string  `json:"kind"`
		CutFeedDots   int     `json:"cutFeedDots"`
		TopMarginDots int     `json:"topMarginDots"`
		PageMarginMM  float64 `json:"pageMarginMM"`
		RenderDPI     int     `json:"renderDPI"`
		PageMode      string  `json:"pageMode"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		writeErr(w, "falta el nombre de la impresora")
		return
	}
	if body.CutFeedDots < 0 {
		body.CutFeedDots = 0
	}
	if body.CutFeedDots > 2000 {
		body.CutFeedDots = 2000
	}
	if body.TopMarginDots < 0 {
		body.TopMarginDots = 0
	}
	// Un margen mayor que media hoja dejaría un área de impresión nula; acotarlo
	// aquí evita que el driver tenga que decidir qué hacer con un absurdo.
	body.PageMarginMM = clampFloat(body.PageMarginMM, 0, maxPageMarginMM)
	// Fuera de este rango, o el texto es ilegible o el bitmap no cabe en el
	// presupuesto de DIB de los drivers host-based y el backoff lo reduce igual.
	if body.RenderDPI != 0 {
		body.RenderDPI = clamp(body.RenderDPI, minRenderDPI, maxRenderDPI)
	}
	kind := ports.PrinterKind(body.Kind)
	// Rechazar valores desconocidos y caer al default (thermal) evita meter
	// basura al YAML si el panel manda algo inesperado.
	if kind != ports.KindThermal && kind != ports.KindPDF {
		kind = ports.KindThermal
	}
	// Mismo criterio para el modo: solo "image" se guarda; cualquier otra cosa es
	// el modo por defecto (vectorial) y se guarda vacío, para no ensuciar el YAML
	// de las térmicas con un modo que no les aplica. No hace falta olvidar el
	// perfil: el cache aplica el modo al leerlo, así que vale desde el siguiente
	// trabajo y el perfil que envió el Backend se conserva.
	var pageMode ports.PageMode
	if ports.PageMode(body.PageMode) == ports.PageModeImage {
		pageMode = ports.PageModeImage
	}
	// Ni el tipo ni el DPI olvidan el perfil cacheado. La caché solo guarda
	// perfiles del Backend —el perfil por defecto de las acciones locales es
	// efímero (localProfile)—, así que olvidarlo solo tiraba el del ERP y sus
	// trabajos fallaban con "no profile" hasta la siguiente sincronización. El
	// tipo nuevo vale al instante para las impresoras sin perfil del Backend.
	s.d.Cfg.Printers = upsertManaged(s.d.Cfg.Printers, ports.ManagedPrinter{
		Name: body.Name, Role: ports.PrinterRole(body.Role), Enabled: body.Enabled,
		Kind:        kind,
		CutFeedDots: body.CutFeedDots, TopMarginDots: body.TopMarginDots,
		PageMarginMM: body.PageMarginMM, RenderDPI: body.RenderDPI,
		PageMode: pageMode,
	})
	if err := s.persist(); err != nil {
		writeErr(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// printersDefault sets the default printer.
func (s *Server) printersDefault(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		writeErr(w, "falta el nombre de la impresora")
		return
	}
	s.d.Cfg.DefaultPrinter = body.Name
	if err := s.persist(); err != nil {
		writeErr(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// printersDrawer opens the cash drawer wired to the given printer (ESC p).
func (s *Server) printersDrawer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	printer := firstNonEmpty(body.Name, s.d.Cfg.DefaultPrinter)
	if printer == "" {
		writeErr(w, "no hay impresora disponible")
		return
	}
	prof := localProfile(s.d.Profiles, s.d.Cfg, printer)
	if !prof.SupportsDrawer {
		writeErr(w, "la impresora no tiene cajón (solo impresoras térmicas con cajón)")
		return
	}
	err := s.d.Engine.PrintWithProfile(r.Context(), dp.PrintJob{
		PrinterID: printer, Format: dp.FormatText, Content: []byte(""),
		Options: dp.Options{Copies: 1, OpenDrawer: true},
	}, prof)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "printer": printer})
}

func (s *Server) history(w http.ResponseWriter, _ *http.Request) {
	// A rich history store is a follow-up; return empty for now.
	writeJSON(w, []any{})
}

func (s *Server) logs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"text": ""})
}

func (s *Server) testPrint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	printer := firstNonEmpty(body.Name, s.d.Cfg.DefaultPrinter)
	if printer == "" {
		if ps, _ := s.d.Discovery.List(r.Context()); len(ps) > 0 {
			printer = ps[0].Name
		}
	}
	if printer == "" {
		writeErr(w, "no hay impresora disponible")
		return
	}
	if err := PrintTestPage(r.Context(), s.d.Engine, s.d.Profiles, s.d.Cfg, printer); err != nil {
		writeErr(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "printer": printer})
}

// PrintTestPage imprime una prueba en printer: la página PDF si es una impresora
// de página (láser / inyección) o un ticket de texto si es térmica. La comparten
// el "Probar" del panel y el de la bandeja para que se comporten igual.
//
// No escribe la caché de perfiles. Antes instalaba un perfil ESC/POS en ella y
// una láser con perfil del ERP pasaba a recibir ESC/POS crudo en los trabajos del
// ERP hasta la siguiente sincronización.
func PrintTestPage(ctx context.Context, engine *appprint.Engine, profiles dp.FallbackProfileProvider, cfg ports.Config, printer string) error {
	prof := localProfile(profiles, cfg, printer)
	job := dp.PrintJob{PrinterID: printer, Options: dp.Options{Copies: 1}}
	if profileIsPDF(prof) {
		job.Format, job.Content = dp.FormatPDF, testPagePDF
	} else {
		job.Format = dp.FormatText
		job.Content = []byte(fmt.Sprintf("TERA AGENT\nPrueba de impresion\n%s\n", time.Now().Format("2006-01-02 15:04")))
		job.Options.Cut = true
	}
	return engine.PrintWithProfile(ctx, job, prof)
}

// localProfile devuelve el perfil con el que imprime una acción local: el del
// Backend si existe; si no, uno por defecto según el tipo declarado en el panel
// (thermal → ESC/POS, pdf → PDF). El por defecto solo vive para ese trabajo y la
// caché lo prepara igual que uno del Backend (en Windows, el modo vectorial o
// imagen de la impresora).
func localProfile(profiles dp.FallbackProfileProvider, cfg ports.Config, printer string) dp.PrinterProfile {
	fallback := escposProfile(printer)
	if cfg.KindOf(printer) == ports.KindPDF {
		fallback = pdfProfile(cfg, printer)
	}
	return profiles.ProfileOr(printer, fallback)
}

// profileIsPDF reports whether the printer consumes PDF natively (láser /
// inyección / virtual PDF). Reconoce también el perfil por defecto pdfProfile.
func profileIsPDF(p dp.PrinterProfile) bool {
	for _, f := range p.NativeFormats {
		if f == dp.DevicePDF || f == dp.DeviceGDIRaster {
			return true
		}
	}
	return false
}

// escposProfile is a sensible default ESC/POS profile so the local test / drawer
// actions work without a Backend-provided profile.
func escposProfile(printer string) dp.PrinterProfile {
	return dp.PrinterProfile{
		PrinterID: printer, NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS},
		WidthDots: 576, DPI: 203, SupportsCut: true, SupportsDrawer: true,
	}
}

// pdfProfile es el perfil por defecto de las impresoras láser / inyección /
// virtuales PDF. Declara DevicePDF, igual que el perfil que envía el Backend para
// una impresora "normal": el cache de perfiles lo traduce en Windows según el
// modo de la impresora (vectorial con pdftocairo, o imagen por GDI raster).
//
// El DPI solo tendría sentido en modo imagen y hoy no llega a aplicarse:
// NormalizeForGDIRaster sube todo perfil raster al mínimo de 600 DPI / 4960 dots
// y el rasterizador prioriza el ancho sobre el DPI. Se sigue rellenando para no
// cambiar el formato del perfil; ver la nota de RenderDPI en ports.Config.
func pdfProfile(cfg ports.Config, printer string) dp.PrinterProfile {
	_, dpi := cfg.PageTuning(printer)
	return dp.PrinterProfile{
		PrinterID: printer, NativeFormats: []dp.DeviceFormat{dp.DevicePDF},
		// WidthDots: 0 a propósito. Antes se fijaba a 4960 (ancho de A4 a 600dpi),
		// lo que forzaba CUALQUIER documento al ancho de un A4: un Letter o un A5
		// salían estirados. Con 0, el rasterizador respeta el tamaño real de la
		// página del PDF y sólo aplica el DPI pedido.
		WidthDots: 0, DPI: dpi,
		SupportsCut:    false,
		SupportsDrawer: false,
	}
}

// persist writes the current config through SaveConfig and re-applies the cut
// calibration to the running engine so panel changes take effect immediately.
func (s *Server) persist() error {
	if s.d.SaveConfig != nil {
		if err := s.d.SaveConfig(s.d.Cfg); err != nil {
			return err
		}
	}
	if s.d.ApplyTuning != nil {
		s.d.ApplyTuning(s.d.Cfg)
	}
	return nil
}

func findManaged(list []ports.ManagedPrinter, name string) (ports.ManagedPrinter, bool) {
	for _, p := range list {
		if p.Name == name {
			return p, true
		}
	}
	return ports.ManagedPrinter{}, false
}

// upsertManaged y removeManaged devuelven SIEMPRE un slice nuevo y nunca tocan
// el array de list. Los resolutores que ApplyTuning instala en el motor (corte,
// margen, modo de página) capturan una copia de Config cuyo Printers apunta al
// mismo array que el panel: modificarlo en su sitio era una carrera de datos con
// los trabajos que imprimían en ese momento, y removeManaged, al compactar,
// podía hacerle ver a un trabajo la configuración de otra impresora.
func upsertManaged(list []ports.ManagedPrinter, mp ports.ManagedPrinter) []ports.ManagedPrinter {
	out := make([]ports.ManagedPrinter, 0, len(list)+1)
	replaced := false
	for _, p := range list {
		if p.Name == mp.Name {
			p, replaced = mp, true
		}
		out = append(out, p)
	}
	if !replaced {
		out = append(out, mp)
	}
	return out
}

func removeManaged(list []ports.ManagedPrinter, name string) []ports.ManagedPrinter {
	out := make([]ports.ManagedPrinter, 0, len(list))
	for _, p := range list {
		if p.Name != name {
			out = append(out, p)
		}
	}
	return out
}

// maxCutFeedDots acota el avance de corte configurable (~25cm de papel). Un
// valor absurdo desperdiciaría un rollo entero por ticket.
const maxCutFeedDots = 2000

// Límites de los ajustes de las impresoras de hoja.
const (
	// maxPageMarginMM: más de 50mm por lado deja un A4 sin sitio útil.
	maxPageMarginMM = 50.0
	// Por debajo de 150 dpi el texto pequeño es ilegible; por encima de 600 el
	// bitmap no cabe en el presupuesto de DIB de los drivers host-based y el
	// backoff acaba reduciéndolo igual, así que sólo se gana lentitud.
	minRenderDPI = 150
	maxRenderDPI = 600
)

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// El Token NUNCA se devuelve al navegador: se informa sólo de si existe y
		// de sus últimos caracteres, lo justo para que el operador reconozca cuál
		// está puesto sin exponer la credencial.
		writeJSON(w, map[string]any{
			"nombre":        s.d.Cfg.AgentID,
			"url":           s.d.Cfg.BackendURL,
			"tokenSet":      s.d.Cfg.Token != "",
			"tokenHint":     tokenHint(s.d.Cfg.Token),
			"log":           s.d.Cfg.LogLevel,
			"dataDir":       s.d.DataDir,
			"runMode":       s.d.RunMode,
			"cutFeedDots":   s.d.Cfg.CutFeedDots,
			"topMarginDots": s.d.Cfg.TopMarginDots,
			"pageMarginMM":  s.d.Cfg.PageMarginMM,
			"renderDPI":     s.d.Cfg.RenderDPI,
		})
		return
	}

	// Todos los campos son punteros: lo que el panel no envía, no se toca. Antes
	// eran valores planos y guardar el formulario con la URL vacía borraba la URL
	// del Backend, desregistrando el equipo sin querer.
	var body struct {
		Nombre        *string  `json:"nombre"`
		URL           *string  `json:"url"`
		Token         *string  `json:"token"`
		Log           *string  `json:"log"`
		CutFeedDots   *int     `json:"cutFeedDots"`
		TopMarginDots *int     `json:"topMarginDots"`
		PageMarginMM  *float64 `json:"pageMarginMM"`
		RenderDPI     *int     `json:"renderDPI"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, "petición inválida: "+err.Error())
		return
	}

	// Un cambio de conexión (URL o Token) invalida la sesión actual con el
	// Backend, así que hay que reconectar; el resto se aplica en vivo.
	connChanged := false

	if body.URL != nil {
		url := strings.TrimSpace(*body.URL)
		if url != "" {
			if err := validateBackendURL(url); err != nil {
				writeErr(w, err.Error())
				return
			}
		}
		if url != s.d.Cfg.BackendURL {
			s.d.Cfg.BackendURL = url
			connChanged = true
		}
	}
	// Token vacío = "no lo cambies". Es la única forma de tener un formulario que
	// se pueda guardar sin reescribir la credencial en cada guardado. Para
	// borrarlo de verdad está /api/unregister.
	if body.Token != nil {
		if tok := strings.TrimSpace(*body.Token); tok != "" && tok != s.d.Cfg.Token {
			s.d.Cfg.Token = tok
			connChanged = true
		}
	}
	if body.Nombre != nil {
		s.d.Cfg.AgentID = strings.TrimSpace(*body.Nombre)
	}
	if body.Log != nil && *body.Log != "" {
		s.d.Cfg.LogLevel = *body.Log
	}
	if body.CutFeedDots != nil {
		s.d.Cfg.CutFeedDots = clamp(*body.CutFeedDots, 0, maxCutFeedDots)
	}
	if body.TopMarginDots != nil {
		s.d.Cfg.TopMarginDots = clamp(*body.TopMarginDots, 0, maxCutFeedDots)
	}
	if body.PageMarginMM != nil {
		s.d.Cfg.PageMarginMM = clampFloat(*body.PageMarginMM, 0, maxPageMarginMM)
	}
	if body.RenderDPI != nil {
		dpi := *body.RenderDPI
		if dpi != 0 { // 0 = usar el default interno
			dpi = clamp(dpi, minRenderDPI, maxRenderDPI)
		}
		// Sin Forget, por la misma razón que en printersManage: olvidar perfiles
		// tiraba los del Backend.
		s.d.Cfg.RenderDPI = dpi
	}

	if s.d.ApplyTuning != nil {
		s.d.ApplyTuning(s.d.Cfg)
	}
	if s.d.SaveConfig != nil {
		if err := s.d.SaveConfig(s.d.Cfg); err != nil {
			writeErr(w, err.Error())
			return
		}
	}

	if connChanged {
		s.applyConnectionChange(w, "Datos de conexión guardados; reconectando…")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "note": "Cambios guardados y aplicados."})
}

// unregister desvincula el equipo: borra Token y URL y reinicia, de modo que el
// panel vuelve a la pantalla de registro. Es la acción explícita que antes sólo
// se conseguía por accidente (vaciando el campo URL y guardando).
func (s *Server) unregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, "usa POST")
		return
	}
	s.d.Cfg.Token = ""
	s.d.Cfg.BackendURL = ""
	if s.d.SaveConfig != nil {
		if err := s.d.SaveConfig(s.d.Cfg); err != nil {
			writeErr(w, err.Error())
			return
		}
	}
	s.applyConnectionChange(w, "Equipo desvinculado; reiniciando…")
}

// applyConnectionChange responde y luego dispara el reinicio del Agent. El orden
// importa: si reiniciáramos antes de escribir la respuesta, el navegador vería
// una conexión cortada en vez del resultado.
func (s *Server) applyConnectionChange(w http.ResponseWriter, note string) {
	writeJSON(w, map[string]any{"ok": true, "note": note, "restarting": true})
	if s.d.OnRegistered == nil {
		return
	}
	go func() {
		time.Sleep(700 * time.Millisecond)
		s.d.OnRegistered()
	}()
}

// validateBackendURL comprueba que la URL del Backend sea un endpoint WebSocket
// utilizable. Antes no se validaba nada: un valor con una errata se guardaba tal
// cual y el agente quedaba en un bucle de reconexión sin explicación clara.
func validateBackendURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("la URL del servidor no es válida: %w", err)
	}
	switch u.Scheme {
	case "wss", "ws":
	default:
		return fmt.Errorf("la URL debe empezar por wss:// (o ws:// en desarrollo), no %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("la URL del servidor no incluye el host")
	}
	return nil
}

// tokenHint devuelve los últimos 4 caracteres del Token para que el operador
// distinga qué credencial está puesta. Con tokens muy cortos no devuelve nada:
// más vale no mostrar pista que filtrar el secreto entero.
func tokenHint(token string) string {
	if len(token) < 8 {
		return ""
	}
	return token[len(token)-4:]
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Token = strings.TrimSpace(body.Token)
	body.URL = strings.TrimSpace(body.URL)
	if body.Token == "" {
		writeErr(w, "token vacío")
		return
	}
	// La URL puede llegar en el body o venir ya en el config; exigimos ambas para
	// no dejar el agente en un estado a medio registrar (token sin destino).
	if body.URL != "" {
		if err := validateBackendURL(body.URL); err != nil {
			writeErr(w, err.Error())
			return
		}
		s.d.Cfg.BackendURL = body.URL
	}
	if s.d.Cfg.BackendURL == "" {
		writeErr(w, "falta la URL del servidor")
		return
	}
	s.d.Cfg.Token = body.Token
	if s.d.SaveConfig != nil {
		if err := s.d.SaveConfig(s.d.Cfg); err != nil {
			writeErr(w, err.Error())
			return
		}
	}
	s.applyConnectionChange(w, "aplicando registro…")
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
