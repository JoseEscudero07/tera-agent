// Package printing defines the domain entities and ports for the printing
// subsystem. Adapters (CUPS, Windows, ESC/POS, Zebra, PDF) implement these
// interfaces in internal/adapters/printing. Owner ports: Software Architect;
// implementations: Printing Engineer. This package knows nothing about
// WebSocket, UI or ERP logic.
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

// Printer describes a discovered printer.
type Printer struct {
	ID     string
	Name   string
	Driver string
	Status Status
}

// Document is a ready-to-print payload plus the target printer.
type Document struct {
	PrinterID string
	Data      []byte // bytes already formatted for the target (ESC/POS, ZPL, PDF, ...)
	// Raw sends the bytes to the device without driver/filter processing. Use it
	// for ESC/POS and ZPL; leave false for PDF/text the driver should render.
	Raw bool
}

// Request is the wire contract for a PRINT job payload dispatched by the
// Backend. Data is base64-encoded automatically by encoding/json.
type Request struct {
	PrinterID string `json:"printer_id"`
	Data      []byte `json:"data"`
	Raw       bool   `json:"raw"`
}

// Port is the driven port the core uses to print. Each backend provides its
// own implementation; they are interchangeable behind this interface.
type Port interface {
	// Print sends a document to a printer, honoring context cancellation.
	Print(ctx context.Context, doc Document) error
}

// Discovery enumerates printers and reports their status.
type Discovery interface {
	List(ctx context.Context) ([]Printer, error)
	StatusOf(ctx context.Context, printerID string) (Status, error)
}
