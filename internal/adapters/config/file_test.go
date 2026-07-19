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
