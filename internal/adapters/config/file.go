// Package config provides a YAML-backed ports.ConfigStore.
// Owner: Go Core Engineer. Secure storage of the Token is reviewed by the
// Security Engineer (never log it, never place it in URLs).
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

// ValidateBackendURL comprueba que raw sea un endpoint WebSocket usable
// (wss://, o ws:// en desarrollo) con host. La usa el registro por consola
// (`tera-agent register`) para no persistir una URL con una errata que dejaría
// al Agent en un bucle de reconexión sin explicación.
func ValidateBackendURL(raw string) error {
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

// LogFileWithin comprueba que log.file, si se indica, viva dentro de data_dir.
//
// No es cosmético: en modo servicio el Agent corre como LocalSystem y abre el
// fichero de log en O_CREATE|O_APPEND (adapters/logger/rotate.go). Si un usuario
// sin privilegios pudiera fijar log.file a una ruta arbitraria, LocalSystem
// crearía/abriría ese fichero por él — una primitiva conocida de escalada de
// privilegios y de DoS (llenar una partición, corromper un fichero del sistema).
// Encerrando el log dentro de data_dir —cuya ACL ya restringimos— el destino
// hereda esa misma protección.
//
// Vacío = solo stderr, sin fichero: válido. En otro caso la ruta debe ser
// absoluta y quedar bajo data_dir.
func LogFileWithin(logFile, dataDir string) error {
	if logFile == "" {
		return nil
	}
	if !filepath.IsAbs(logFile) {
		return fmt.Errorf("log.file debe ser una ruta absoluta: %q", logFile)
	}
	lf := filepath.Clean(logFile)
	dd := filepath.Clean(dataDir)
	rel, err := filepath.Rel(dd, lf)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("log.file (%q) debe estar dentro de data_dir (%q)", logFile, dataDir)
	}
	return nil
}

// fileStore reads/writes the configuration as YAML at Path.
type fileStore struct{ Path string }

// wireManagedPrinter is the on-disk form of a ports.ManagedPrinter.
type wireManagedPrinter struct {
	Name          string `yaml:"name"`
	Role          string `yaml:"role"`
	Enabled       bool   `yaml:"enabled"`
	Kind          string `yaml:"kind,omitempty"`
	CutFeedDots   int    `yaml:"cut_feed_dots"`
	TopMarginDots int    `yaml:"top_margin_dots"`
	// Solo para kind=pdf. omitempty para no ensuciar el YAML de las térmicas.
	PageMarginMM float64 `yaml:"page_margin_mm,omitempty"`
	RenderDPI    int     `yaml:"render_dpi,omitempty"`
	// vector | image. Vacío = vector.
	PageMode string `yaml:"page_mode,omitempty"`
}

// New returns a ConfigStore backed by the YAML file at path.
func New(path string) ports.ConfigStore { return &fileStore{Path: path} }

// LogSubdir es la subcarpeta de datos donde viven los logs. Se separa de la
// configuración porque tiene permisos distintos: en modo servicio los logs son
// legibles por el grupo Usuarios (soporte), pero el config con el Token no.
const LogSubdir = "logs"

// yamlStr envuelve un valor en comillas SIMPLES de YAML, que lo vuelven literal.
// Con comillas dobles, YAML interpreta los escapes y una ruta de Windows como
// C:\ProgramData\TeraAgent (con \P, \T) es YAML inválido: el Agent moría al
// arrancar con "found unknown escape character". Antes esto lo hacía el script
// Pascal del instalador; ahora el config lo escribe el propio Agent.
func yamlStr(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// DefaultYAML genera la configuración inicial para dataDir. server.url y el
// Token quedan vacíos: se rellenan al registrar el equipo (panel en modo usuario,
// `tera-agent register` en modo servicio). El log va a la subcarpeta logs\.
func DefaultYAML(dataDir string) []byte {
	logFile := filepath.Join(dataDir, LogSubdir, "tera-agent.log")
	lines := []string{
		"# Tera Agent - configuracion. Registra el equipo desde el panel:",
		"#   http://127.0.0.1:9180",
		"server:",
		"  url: '' # wss://tu-erp/ws/agent/ ; vacio = modo local",
		"  token: '' # lo emite el Backend; se rellena al registrar",
		"printer:",
		"  default: ''",
		"http:",
		"  addr: " + yamlStr("127.0.0.1:9100"),
		"  token: ''",
		"data_dir: " + yamlStr(dataDir),
		"log:",
		"  level: " + yamlStr("info"),
		"  file: " + yamlStr(logFile),
		"",
	}
	return []byte(strings.Join(lines, "\n"))
}

// EnsureDefault crea dataDir (y su subcarpeta de logs) y escribe la config por
// defecto si el fichero aún no existe. Devuelve created=true solo si la creó, de
// modo que una actualización o un segundo arranque NUNCA pisen el Token ni las
// impresoras ya calibradas. dirPerm aplica solo en POSIX; en Windows los
// permisos los fija fsacl.
func EnsureDefault(path, dataDir string) (created bool, err error) {
	if err := os.MkdirAll(filepath.Join(dataDir, LogSubdir), 0o755); err != nil {
		return false, err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return false, nil // ya existe: no lo tocamos
	} else if !os.IsNotExist(statErr) {
		return false, statErr
	}
	if err := os.WriteFile(path, DefaultYAML(dataDir), 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// wire is the on-disk YAML representation.
type wire struct {
	Server struct {
		URL                string `yaml:"url"`
		Token              string `yaml:"token"`
		InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
	} `yaml:"server"`
	Agent struct {
		ID string `yaml:"id"`
	} `yaml:"agent"`
	DataDir string `yaml:"data_dir"`
	Printer struct {
		Default       string               `yaml:"default"`
		CutFeedDots   int                  `yaml:"cut_feed_dots"`
		TopMarginDots int                  `yaml:"top_margin_dots"`
		PageMarginMM  float64              `yaml:"page_margin_mm"`
		RenderDPI     int                  `yaml:"render_dpi"`
		Managed       []wireManagedPrinter `yaml:"managed"`
	} `yaml:"printer"`
	Heartbeat struct {
		Seconds int `yaml:"seconds"`
	} `yaml:"heartbeat"`
	HTTP struct {
		Addr  string `yaml:"addr"`
		Token string `yaml:"token"`
	} `yaml:"http"`
	Log struct {
		Level      string `yaml:"level"`
		File       string `yaml:"file"`
		MaxSizeMB  int    `yaml:"max_size_mb"`
		MaxBackups int    `yaml:"max_backups"`
	} `yaml:"log"`
}

func (f *fileStore) Load() (ports.Config, error) {
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return ports.Config{}, err
	}
	var w wire
	if err := yaml.Unmarshal(b, &w); err != nil {
		return ports.Config{}, err
	}
	level := w.Log.Level
	if level == "" {
		level = "info"
	}
	printers := make([]ports.ManagedPrinter, 0, len(w.Printer.Managed))
	for _, p := range w.Printer.Managed {
		printers = append(printers, ports.ManagedPrinter{
			Name: p.Name, Role: ports.PrinterRole(p.Role), Enabled: p.Enabled,
			Kind:        ports.PrinterKind(p.Kind),
			CutFeedDots: p.CutFeedDots, TopMarginDots: p.TopMarginDots,
			PageMarginMM: p.PageMarginMM, RenderDPI: p.RenderDPI,
			PageMode: ports.PageMode(p.PageMode),
		})
	}
	return ports.Config{
		BackendURL:         w.Server.URL,
		Token:              w.Server.Token,
		AgentID:            w.Agent.ID,
		DefaultPrinter:     w.Printer.Default,
		Printers:           printers,
		CutFeedDots:        w.Printer.CutFeedDots,
		TopMarginDots:      w.Printer.TopMarginDots,
		PageMarginMM:       w.Printer.PageMarginMM,
		RenderDPI:          w.Printer.RenderDPI,
		HeartbeatInterval:  time.Duration(w.Heartbeat.Seconds) * time.Second,
		LogLevel:           level,
		LogFile:            w.Log.File,
		LogMaxSizeMB:       w.Log.MaxSizeMB,
		LogMaxBackups:      w.Log.MaxBackups,
		HTTPAddr:           w.HTTP.Addr,
		HTTPToken:          w.HTTP.Token,
		InsecureSkipVerify: w.Server.InsecureSkipVerify,
		DataDir:            w.DataDir,
	}, nil
}

func (f *fileStore) Save(c ports.Config) error {
	var w wire
	w.Server.URL = c.BackendURL
	w.Server.Token = c.Token
	w.Server.InsecureSkipVerify = c.InsecureSkipVerify
	w.Agent.ID = c.AgentID
	w.DataDir = c.DataDir
	w.Printer.Default = c.DefaultPrinter
	w.Printer.CutFeedDots = c.CutFeedDots
	w.Printer.TopMarginDots = c.TopMarginDots
	w.Printer.PageMarginMM = c.PageMarginMM
	w.Printer.RenderDPI = c.RenderDPI
	for _, p := range c.Printers {
		w.Printer.Managed = append(w.Printer.Managed, wireManagedPrinter{
			Name: p.Name, Role: string(p.Role), Enabled: p.Enabled,
			Kind:        string(p.Kind),
			CutFeedDots: p.CutFeedDots, TopMarginDots: p.TopMarginDots,
			PageMarginMM: p.PageMarginMM, RenderDPI: p.RenderDPI,
			PageMode: string(p.PageMode),
		})
	}
	w.Heartbeat.Seconds = int(c.HeartbeatInterval / time.Second)
	w.HTTP.Addr = c.HTTPAddr
	w.HTTP.Token = c.HTTPToken
	w.Log.Level = c.LogLevel
	w.Log.File = c.LogFile
	w.Log.MaxSizeMB = c.LogMaxSizeMB
	w.Log.MaxBackups = c.LogMaxBackups

	b, err := yaml.Marshal(&w)
	if err != nil {
		return err
	}
	return os.WriteFile(f.Path, b, 0o600)
}
