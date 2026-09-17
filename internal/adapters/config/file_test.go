package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

func TestLoad_ParsesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
server:
  url: "wss://erp.example.com/ws/agent/"
  token: "secret-token"
agent:
  id: "agt-1"
printer:
  default: "XP-80"
heartbeat:
  seconds: 45
log:
  level: "debug"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := New(path).Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.BackendURL != "wss://erp.example.com/ws/agent/" {
		t.Errorf("BackendURL = %q", cfg.BackendURL)
	}
	if cfg.Token != "secret-token" {
		t.Errorf("Token = %q", cfg.Token)
	}
	if cfg.DefaultPrinter != "XP-80" {
		t.Errorf("DefaultPrinter = %q", cfg.DefaultPrinter)
	}
	if cfg.HeartbeatInterval != 45*time.Second {
		t.Errorf("HeartbeatInterval = %v", cfg.HeartbeatInterval)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q", cfg.LogLevel)
	}
}

func TestLoad_DefaultsLogLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  url: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := New(path).Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default = %q, want info", cfg.LogLevel)
	}
	if cfg.BackendURL != "" {
		t.Errorf("BackendURL = %q, want empty (local mode)", cfg.BackendURL)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	store := New(path)
	orig := ports.Config{
		BackendURL:        "wss://x/ws/",
		Token:             "tok",
		AgentID:           "agt-9",
		DefaultPrinter:    "XP-58",
		HeartbeatInterval: 30 * time.Second,
		LogLevel:          "warn",
	}
	if err := store.Save(orig); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got.DefaultPrinter != orig.DefaultPrinter || got.HeartbeatInterval != orig.HeartbeatInterval {
		t.Errorf("round trip mismatch: %+v vs %+v", got, orig)
	}
}

func TestSaveLoad_ManagedPrintersAndPreservedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	store := New(path)
	orig := ports.Config{
		DefaultPrinter: "POS80",
		CutFeedDots:    232,
		TopMarginDots:  16,
		Printers: []ports.ManagedPrinter{
			{Name: "POS80", Role: ports.RoleFacturacion, Enabled: true, Kind: ports.KindThermal, CutFeedDots: 248, TopMarginDots: 8},
			{Name: "Kitchen", Role: ports.RoleCocina, Enabled: false, Kind: ports.KindThermal},
			{Name: "HP LaserJet", Role: ports.RoleFacturacion, Enabled: true, Kind: ports.KindPDF},
			{Name: "Samsung M2020", Role: ports.RoleOficina, Enabled: true, Kind: ports.KindPDF, PageMode: ports.PageModeImage},
		},
		// Fields that Save used to drop silently.
		DataDir:            "/var/lib/tera",
		InsecureSkipVerify: true,
		LogFile:            "/var/log/tera.log",
		LogMaxSizeMB:       7,
		LogMaxBackups:      4,
	}
	if err := store.Save(orig); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(got.Printers) != 4 {
		t.Fatalf("Printers len = %d, want 4 (%+v)", len(got.Printers), got.Printers)
	}
	for i := range orig.Printers {
		if got.Printers[i] != orig.Printers[i] {
			t.Errorf("managed printers[%d] mismatch: %+v vs %+v", i, got.Printers[i], orig.Printers[i])
		}
	}
	if got.Printers[2].Kind != ports.KindPDF {
		t.Errorf("HP LaserJet Kind = %q, want %q", got.Printers[2].Kind, ports.KindPDF)
	}
	if got.Printers[3].PageMode != ports.PageModeImage {
		t.Errorf("Samsung PageMode = %q, want image (modo imagen perdido al guardar)", got.Printers[3].PageMode)
	}
	if got.DataDir != orig.DataDir {
		t.Errorf("DataDir not preserved: %q", got.DataDir)
	}
	if !got.InsecureSkipVerify {
		t.Errorf("InsecureSkipVerify not preserved")
	}
	if got.LogFile != orig.LogFile || got.LogMaxSizeMB != orig.LogMaxSizeMB || got.LogMaxBackups != orig.LogMaxBackups {
		t.Errorf("log fields not preserved: file=%q size=%d backups=%d", got.LogFile, got.LogMaxSizeMB, got.LogMaxBackups)
	}
	if got.CutFeedDots != 232 || got.TopMarginDots != 16 {
		t.Errorf("global cut tuning not preserved: feed=%d top=%d", got.CutFeedDots, got.TopMarginDots)
	}
	if got.Printers[0].CutFeedDots != 248 || got.Printers[0].TopMarginDots != 8 {
		t.Errorf("per-printer cut tuning not preserved: %+v", got.Printers[0])
	}
}

func TestPrinterTuning_PerPrinterOverridesGlobal(t *testing.T) {
	cfg := ports.Config{
		CutFeedDots:   232,
		TopMarginDots: 16,
		Printers: []ports.ManagedPrinter{
			{Name: "XPrinter", CutFeedDots: 248}, // overrides feed, inherits top margin
			{Name: "POS80"},                      // inherits both globals
		},
	}
	// Per-printer feed override wins, top margin falls back to global.
	if feed, top := cfg.PrinterTuning("XPrinter"); feed != 248 || top != 16 {
		t.Errorf("XPrinter tuning = (%d,%d), want (248,16)", feed, top)
	}
	// Printer without overrides inherits globals.
	if feed, top := cfg.PrinterTuning("POS80"); feed != 232 || top != 16 {
		t.Errorf("POS80 tuning = (%d,%d), want (232,16)", feed, top)
	}
	// Unknown printer inherits globals.
	if feed, top := cfg.PrinterTuning("Otra"); feed != 232 || top != 16 {
		t.Errorf("unknown printer tuning = (%d,%d), want (232,16)", feed, top)
	}
}

// Un config.yaml escrito cuando existía el ajuste de DPI (render_dpi) tiene que
// seguir cargando: el campo se ignora y desaparece al volver a guardar.
func TestLoad_IgnoresRemovedRenderDPI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	old := `printer:
  render_dpi: 450
  page_margin_mm: 4
  managed:
    - name: HP
      enabled: true
      kind: pdf
      render_dpi: 600
      page_mode: image
`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(path)
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("un config.yaml con render_dpi ya no carga: %v", err)
	}
	if cfg.PageMarginMM != 4 || len(cfg.Printers) != 1 || cfg.Printers[0].PageMode != ports.PageModeImage {
		t.Errorf("config mal leída: %+v", cfg)
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(path)
	if strings.Contains(string(saved), "render_dpi") {
		t.Errorf("render_dpi sigue en el YAML guardado:\n%s", saved)
	}
}

// LogFileWithin es la barrera que impide que un log.file manipulado saque la
// escritura del servicio (LocalSystem) fuera de la carpeta de datos protegida.
func TestLogFileWithin(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "logs")

	ok := []string{
		"", // sin fichero: solo stderr
		filepath.Join(dir, "tera-agent.log"),
		filepath.Join(sub, "tera-agent.log"),
	}
	for _, lf := range ok {
		if err := LogFileWithin(lf, dir); err != nil {
			t.Errorf("LogFileWithin(%q, %q) = %v; se esperaba válida", lf, dir, err)
		}
	}

	bad := []string{
		filepath.Join(filepath.Dir(dir), "fuera.log"),        // hermano de data_dir
		filepath.Join(dir, "..", "escape.log"),               // sube por encima
		"relativo.log",                                        // no absoluta
	}
	for _, lf := range bad {
		if err := LogFileWithin(lf, dir); err == nil {
			t.Errorf("LogFileWithin(%q, %q) = nil; se esperaba error", lf, dir)
		}
	}
}

// EnsureDefault crea la config inicial una sola vez, con log.file dentro de
// data_dir, y no la pisa en llamadas posteriores (actualización / rearranque).
func TestEnsureDefault(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "TeraAgent")
	path := filepath.Join(dataDir, "config.yaml")

	created, err := EnsureDefault(path, dataDir)
	if err != nil || !created {
		t.Fatalf("EnsureDefault inicial: created=%v err=%v", created, err)
	}
	// El log por defecto debe quedar dentro de data_dir (barrera de seguridad).
	cfg, err := New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := LogFileWithin(cfg.LogFile, dataDir); err != nil {
		t.Errorf("el log por defecto debe estar dentro de data_dir: %v", err)
	}
	if cfg.DataDir != dataDir {
		t.Errorf("data_dir = %q; se esperaba %q", cfg.DataDir, dataDir)
	}

	// Marca el fichero y comprueba que una segunda llamada NO lo pisa.
	if err := os.WriteFile(path, []byte("server:\n  token: 'YA-REGISTRADO'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err = EnsureDefault(path, dataDir)
	if err != nil || created {
		t.Fatalf("segunda EnsureDefault: created=%v err=%v (no debía recrear)", created, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "YA-REGISTRADO") {
		t.Error("EnsureDefault pisó una config existente")
	}
}

// El config.yaml inicial trae ya la URL del ERP: dar de alta un equipo es pegar
// el Token y nada más. El Token, en cambio, tiene que quedar vacío.
func TestEnsureDefaultTraeLaURLDelERP(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "config.yaml")
	if _, err := EnsureDefault(path, dataDir); err != nil {
		t.Fatal(err)
	}
	cfg, err := New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BackendURL != ports.DefaultBackendURL {
		t.Errorf("server.url = %q; se esperaba %q", cfg.BackendURL, ports.DefaultBackendURL)
	}
	if err := ValidateBackendURL(cfg.BackendURL); err != nil {
		t.Errorf("la URL por defecto no pasa la validación: %v", err)
	}
	if cfg.Token != "" {
		t.Errorf("Token = %q; el config inicial no debe traer credenciales", cfg.Token)
	}
}
