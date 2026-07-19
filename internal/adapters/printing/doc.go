// Package printing hosts the printing adapters that implement the ports in
// internal/domain/printing: CUPS (Linux), Windows Print API, ESC/POS, Zebra
// (ZPL) and PDF. Each backend is an interchangeable adapter. Platform-specific
// files use build tags. Owner: Printing Engineer.
//
// It must not import comms, ui or device packages.
package printing
