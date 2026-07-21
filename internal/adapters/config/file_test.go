package config

import (
	"os"
	"path/filepath"
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
		Printers: []ports.ManagedPrinter{
			{Name: "POS80", Role: ports.RoleReceipt, Enabled: true},
			{Name: "Kitchen", Role: ports.RoleKitchen, Enabled: false},
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

	if len(got.Printers) != 2 {
		t.Fatalf("Printers len = %d, want 2 (%+v)", len(got.Printers), got.Printers)
	}
	if got.Printers[0] != orig.Printers[0] || got.Printers[1] != orig.Printers[1] {
		t.Errorf("managed printers mismatch: %+v vs %+v", got.Printers, orig.Printers)
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
}
