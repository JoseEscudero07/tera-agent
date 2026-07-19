// Package config provides a file-based ports.ConfigStore backed by JSON.
// Owner: Go Core Engineer. Secure storage of the Token is reviewed by the
// Security Engineer (never log it, never place it in URLs).
package config

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

// fileStore reads/writes the configuration as JSON at Path.
type fileStore struct {
	Path string
}

// New returns a ConfigStore backed by the JSON file at path.
func New(path string) ports.ConfigStore {
	return &fileStore{Path: path}
}

// wire is the on-disk representation. HeartbeatInterval is stored in seconds
// for human readability.
type wire struct {
	BackendURL          string `json:"backend_url"`
	Token               string `json:"token"`
	HeartbeatSeconds    int    `json:"heartbeat_seconds"`
	LogLevel            string `json:"log_level"`
}

func (f *fileStore) Load() (ports.Config, error) {
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return ports.Config{}, err
	}
	var w wire
	if err := json.Unmarshal(b, &w); err != nil {
		return ports.Config{}, err
	}
	if w.BackendURL == "" {
		return ports.Config{}, errors.New("config: backend_url is required")
	}
	return ports.Config{
		BackendURL:        w.BackendURL,
		Token:             w.Token,
		HeartbeatInterval: time.Duration(w.HeartbeatSeconds) * time.Second,
		LogLevel:          w.LogLevel,
	}, nil
}

func (f *fileStore) Save(c ports.Config) error {
	w := wire{
		BackendURL:       c.BackendURL,
		Token:            c.Token,
		HeartbeatSeconds: int(c.HeartbeatInterval / time.Second),
		LogLevel:         c.LogLevel,
	}
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.Path, b, 0o600)
}
