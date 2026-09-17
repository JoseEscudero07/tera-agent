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

// FallbackProfileProvider resuelve el perfil de las acciones locales (probar,
// abrir cajón, POST /print) sin escribir la caché. Read side.
type FallbackProfileProvider interface {
	// ProfileOr devuelve el perfil cacheado de printerID si tiene formatos
	// nativos; si no, fallback con PrinterID = printerID. El resultado pasa por
	// la misma preparación de plataforma que Profile, así que un perfil de
	// respaldo imprime por el mismo camino que uno del Backend.
	ProfileOr(printerID string, fallback PrinterProfile) PrinterProfile
}

// ProfileCache adds the write side, used by the communication layer when the
// Backend pushes profiles (on authentication and on change). Solo guarda perfiles
// del Backend: las acciones locales usan ProfileOr y no escriben aquí, porque un
// perfil local instalado pisaba el del ERP hasta la siguiente sincronización.
type ProfileCache interface {
	ProfileProvider
	FallbackProfileProvider
	Set(PrinterProfile)      // upsert one profile
	SetAll([]PrinterProfile) // full sync
	// Forget descarta el perfil cacheado de una impresora concreta.
	Forget(printerID string)
}
