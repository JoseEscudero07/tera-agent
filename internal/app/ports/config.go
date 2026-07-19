package ports

import "time"

// Config holds the static configuration the Agent needs to start. The Token is
// issued by the Backend and only transmitted by the Agent; it is never
// generated or renewed here. Runtime identity (company, branch, ...) is received
// from the Backend after authentication.
type Config struct {
	// BackendURL is the secure (wss://) endpoint of the ERP Backend. Empty means
	// local mode (no backend): the Agent runs offline and printing is available
	// via the CLI.
	BackendURL string
	// Token is the registration Token issued by the Backend.
	Token string
	// AgentID is an optional local identifier for this installation.
	AgentID string
	// DefaultPrinter is used when a command omits the printer.
	DefaultPrinter string
	// HeartbeatInterval is a default; the Backend may override it at handshake.
	HeartbeatInterval time.Duration
	// LogLevel: debug|info|warn|error.
	LogLevel string
	// HTTPAddr, when set, makes `run` also expose the local HTTP print service
	// (e.g. "127.0.0.1:9100"). Empty disables it.
	HTTPAddr string
	// HTTPToken, when set, requires "Authorization: Bearer <token>" on the HTTP API.
	HTTPToken string
}

// ConfigStore loads and persists the Agent configuration. The file-based
// implementation lives in internal/adapters/config.
type ConfigStore interface {
	Load() (Config, error)
	Save(Config) error
}
