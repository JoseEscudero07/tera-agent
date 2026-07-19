// Package config provides a YAML-backed ports.ConfigStore.
// Owner: Go Core Engineer. Secure storage of the Token is reviewed by the
// Security Engineer (never log it, never place it in URLs).
package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

// fileStore reads/writes the configuration as YAML at Path.
type fileStore struct{ Path string }

// New returns a ConfigStore backed by the YAML file at path.
func New(path string) ports.ConfigStore { return &fileStore{Path: path} }

// wire is the on-disk YAML representation.
type wire struct {
	Server struct {
		URL   string `yaml:"url"`
		Token string `yaml:"token"`
	} `yaml:"server"`
	Agent struct {
		ID string `yaml:"id"`
	} `yaml:"agent"`
	Printer struct {
		Default string `yaml:"default"`
	} `yaml:"printer"`
	Heartbeat struct {
		Seconds int `yaml:"seconds"`
	} `yaml:"heartbeat"`
	Log struct {
		Level string `yaml:"level"`
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
	return ports.Config{
		BackendURL:        w.Server.URL,
		Token:             w.Server.Token,
		AgentID:           w.Agent.ID,
		DefaultPrinter:    w.Printer.Default,
		HeartbeatInterval: time.Duration(w.Heartbeat.Seconds) * time.Second,
		LogLevel:          level,
	}, nil
}

func (f *fileStore) Save(c ports.Config) error {
	var w wire
	w.Server.URL = c.BackendURL
	w.Server.Token = c.Token
	w.Agent.ID = c.AgentID
	w.Printer.Default = c.DefaultPrinter
	w.Heartbeat.Seconds = int(c.HeartbeatInterval / time.Second)
	w.Log.Level = c.LogLevel

	b, err := yaml.Marshal(&w)
	if err != nil {
		return err
	}
	return os.WriteFile(f.Path, b, 0o600)
}
