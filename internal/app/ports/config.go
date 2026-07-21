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
	// Printers is the agent-local list of managed printers: which discovered
	// printers this installation actually uses, plus a local routing role. These
	// are hints owned by the Agent, not the ERP-owned PrinterProfile.
	Printers []ManagedPrinter
	// CutFeedDots is the global default paper feed (ESC J) before the cut, in
	// dots (1mm ≈ 8 dots @203dpi). 0 = the encoder's built-in default. A printer
	// entry can override it. Lets a client tune the cut without recompiling.
	CutFeedDots int
	// TopMarginDots is the global default of blank top rows to keep. 0 = the
	// encoder's built-in default. A printer entry can override it.
	TopMarginDots int
	// HeartbeatInterval is a default; the Backend may override it at handshake.
	HeartbeatInterval time.Duration
	// LogLevel: debug|info|warn|error.
	LogLevel string
	// LogFile, when set, writes rotating logs to this path (in addition to stderr).
	LogFile string
	// LogMaxSizeMB and LogMaxBackups control log rotation (defaults 5 and 3).
	LogMaxSizeMB  int
	LogMaxBackups int
	// HTTPAddr, when set, makes `run` also expose the local HTTP print service
	// (e.g. "127.0.0.1:9100"). Empty disables it.
	HTTPAddr string
	// HTTPToken, when set, requires "Authorization: Bearer <token>" on the HTTP API.
	HTTPToken string
	// InsecureSkipVerify disables TLS certificate verification for wss:// (dev
	// only, e.g. self-signed certificates). Default false.
	InsecureSkipVerify bool
	// DataDir is where the Agent persists runtime state (processed job ids,
	// pending results). Empty = OS-appropriate default next to the config.
	DataDir string
}

// PrinterRole is the agent-local routing hint for a managed printer. It tells
// the Agent which physical printer plays which functional role; it is not a
// device capability (that is the ERP-owned PrinterProfile).
type PrinterRole string

const (
	RoleReceipt PrinterRole = "receipt" // ticket / recibo (POS)
	RoleKitchen PrinterRole = "kitchen" // comanda de cocina
	RoleA4      PrinterRole = "a4"      // documento A4 / factura
	RoleLabel   PrinterRole = "label"   // etiquetas
)

// ManagedPrinter is a discovered printer the user has chosen to manage: whether
// it is enabled for this agent, the local role it plays and its physical cut
// calibration (blade distance varies by model, hence per-printer overrides).
type ManagedPrinter struct {
	Name    string
	Role    PrinterRole
	Enabled bool
	// CutFeedDots overrides the paper fed before the cut for this printer. 0 =
	// use the global default (Config.CutFeedDots) or the built-in default.
	CutFeedDots int
	// TopMarginDots overrides how many blank top rows to keep. 0 = use the global
	// default (Config.TopMarginDots) or the built-in default.
	TopMarginDots int
}

// PrinterTuning resolves the cut calibration for a printer: the per-printer
// override wins, else the global default, else 0 (encoder built-in default).
func (c Config) PrinterTuning(printerID string) (cutFeedDots, topMarginDots int) {
	cutFeedDots, topMarginDots = c.CutFeedDots, c.TopMarginDots
	for _, p := range c.Printers {
		if p.Name == printerID {
			if p.CutFeedDots != 0 {
				cutFeedDots = p.CutFeedDots
			}
			if p.TopMarginDots != 0 {
				topMarginDots = p.TopMarginDots
			}
			break
		}
	}
	return cutFeedDots, topMarginDots
}

// ConfigStore loads and persists the Agent configuration. The file-based
// implementation lives in internal/adapters/config.
type ConfigStore interface {
	Load() (Config, error)
	Save(Config) error
}
