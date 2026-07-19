// Package device defines the domain entities and ports for auxiliary hardware
// (barcode scanner, cash drawer, scale, customer display, serial, USB HID,
// RFID). Drivers implement these ports in internal/adapters/device.
// Owner ports: Software Architect; implementations: Device Engineer.
// This package knows nothing about WebSocket, UI or ERP logic.
package device

import "context"

// Kind classifies a hardware device.
type Kind string

const (
	KindScanner      Kind = "BARCODE_SCANNER"
	KindCashDrawer   Kind = "CASH_DRAWER"
	KindScale        Kind = "SCALE"
	KindDisplay      Kind = "CUSTOMER_DISPLAY"
	KindSerial       Kind = "SERIAL"
	KindUSBHID       Kind = "USB_HID"
	KindRFID         Kind = "RFID"
)

// Device describes a connected peripheral.
type Device struct {
	ID   string
	Kind Kind
	Name string
}

// Driver is the port every peripheral implements. Command is opaque at the
// domain level; the concrete driver interprets it for its device.
type Driver interface {
	Kind() Kind
	Open(ctx context.Context) error
	Execute(ctx context.Context, command []byte) ([]byte, error)
	Close(ctx context.Context) error
}
