// Package printing defines the domain of the print engine: entities, the
// intermediate artifacts, and the ports (Renderer, Encoder, Driver, Rasterizer,
// Binarizer, Resolver, ProfileProvider). It has no framework, transport or UI
// dependencies (image is stdlib). Adapters implement these ports.
//
// See docs/PRINT_ENGINE.md and docs/BINARIZATION.md for the approved design.
package printing

import "context"

// Status is the reported state of a physical printer.
type Status string

const (
	StatusUnknown Status = "UNKNOWN"
	StatusReady   Status = "READY"
	StatusBusy    Status = "BUSY"
	StatusOffline Status = "OFFLINE"
	StatusError   Status = "ERROR"
)

// Printer is a discovered printer (result of Discovery).
type Printer struct {
	ID     string
	Name   string
	Driver string
	Status Status
}

// PhysicalPrinter is what the Agent DETECTS and reports to the ERP. It carries
// only physical facts, never business logic; the ERP owns the PrinterProfile.
type PhysicalPrinter struct {
	Name   string
	Driver string
	Port   string
	Status Status
	DPI    int
}

// Discovery enumerates printers and reports their status.
type Discovery interface {
	List(ctx context.Context) ([]Printer, error)
	StatusOf(ctx context.Context, printerID string) (Status, error)
}
