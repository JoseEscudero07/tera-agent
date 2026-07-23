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

// Lista canónica de roles funcionales. Debe coincidir exactamente con
// `apps.tera_agent.roles.Rol` del backend Django: los strings viajan por JSON
// como clave y cualquier divergencia rompe el ruteo por rol.
const (
	RoleCaja        PrinterRole = "caja"        // recibos de caja, cierre Z, arqueos
	RoleFacturacion PrinterRole = "facturacion" // tickets 80mm y facturas A4 de venta
	RoleCocina      PrinterRole = "cocina"      // comandas, delivery, KDS impreso
	RoleBodega      PrinterRole = "bodega"      // remisiones, órdenes de despacho, ingresos
	RoleOficina     PrinterRole = "oficina"     // informes internos, listados
	RoleEtiqueta    PrinterRole = "etiqueta"    // precios, códigos de barra
)

// PrinterKind is what physical family of printer this is, from the Agent's
// point of view. It decides which local default profile install (ESC/POS vs
// PDF/raster) when the Backend hasn't sent one. Empty = thermal (backward
// compat with configs written before this field existed).
type PrinterKind string

const (
	KindThermal PrinterKind = "thermal" // POS 58/80 mm ESC/POS
	KindPDF     PrinterKind = "pdf"     // láser / inyección / virtual PDF (GDI)
)

// ManagedPrinter is a discovered printer the user has chosen to manage: whether
// it is enabled for this agent, the local role it plays and its physical cut
// calibration (blade distance varies by model, hence per-printer overrides).
type ManagedPrinter struct {
	Name    string
	Role    PrinterRole
	Enabled bool
	// Kind marca la familia física (thermal|pdf). Vacío = thermal (compat).
	Kind PrinterKind
	// CutFeedDots overrides the paper fed before the cut for this printer. 0 =
	// use the global default (Config.CutFeedDots) or the built-in default.
	CutFeedDots int
	// TopMarginDots overrides how many blank top rows to keep. 0 = use the global
	// default (Config.TopMarginDots) or the built-in default.
	TopMarginDots int
}

// KindOf resolves the declared kind for a printer, or thermal if unmanaged /
// unset. Helper used by the UI and profile bootstrap so the default doesn't
// leak in multiple places.
func (c Config) KindOf(printerID string) PrinterKind {
	for _, p := range c.Printers {
		if p.Name == printerID {
			if p.Kind != "" {
				return p.Kind
			}
			return KindThermal
		}
	}
	return KindThermal
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
