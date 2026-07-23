package printing

// PrinterProfile belongs to the ERP catalog (configured by the administrator).
// The Agent never discovers or persists it as business logic: it receives it
// from the Backend and keeps it in memory (see ProfileCache).
type PrinterProfile struct {
	PrinterID      string
	NativeFormats  []DeviceFormat // formats the device consumes, in priority order
	WidthDots      int            // e.g. 576 for 80mm @203dpi
	DPI            int
	SupportsCut    bool
	SupportsDrawer bool
	// Meta carries ERP catalog metadata (technology, logical driver, margins,
	// logo, custom config) without coupling the Core to concrete ERP fields.
	Meta map[string]string
}

// ProfileProvider returns the (cached) profile for a printer. Read side.
type ProfileProvider interface {
	Profile(printerID string) (PrinterProfile, error)
}

// ProfileCache adds the write side, used by the communication layer when the
// Backend pushes profiles (on authentication and on change).
type ProfileCache interface {
	ProfileProvider
	Set(PrinterProfile)      // upsert one profile
	SetAll([]PrinterProfile) // full sync
	// Forget descarta el perfil cacheado de una impresora concreta. La UI lo
	// usa cuando el usuario cambia el tipo local (thermal↔pdf) para evitar
	// que ensureProfile respete un perfil ya obsoleto.
	Forget(printerID string)
}
