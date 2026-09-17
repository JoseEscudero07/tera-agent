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
	// vector | image. Vacío = vector.
	PageMode string `yaml:"page_mode,omitempty"`
}

// New returns a ConfigStore backed by the YAML file at path.
func New(path string) ports.ConfigStore { return &fileStore{Path: path} }

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
			PageMarginMM: p.PageMarginMM,
			PageMode:     ports.PageMode(p.PageMode),
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
	for _, p := range c.Printers {
		w.Printer.Managed = append(w.Printer.Managed, wireManagedPrinter{
			Name: p.Name, Role: string(p.Role), Enabled: p.Enabled,
			Kind:        string(p.Kind),
			CutFeedDots: p.CutFeedDots, TopMarginDots: p.TopMarginDots,
			PageMarginMM: p.PageMarginMM,
			PageMode:     string(p.PageMode),
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
