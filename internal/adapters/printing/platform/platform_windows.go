//go:build windows

// Package platform selects the OS-specific printing adapters. On Windows it uses
// the print spooler (winspool) for RAW printing and enumeration. There is no
// graphical/GDI driver path in the MVP: documents (PDF) go through the
// rasterizer -> ESC/POS pipeline like on the other platforms.
// Owner: Printing Engineer.
package platform

import (
	spooler "github.com/teraerp/tera-agent/internal/adapters/printing/driver/spooler"
	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Drivers returns the platform driver set (spooler RAW).
func Drivers(log ports.Logger) []dp.Driver {
	return []dp.Driver{spooler.NewRaw(log)}
}

// RawDriver returns the platform raw driver (for the `raw` command).
func RawDriver(log ports.Logger) dp.Driver { return spooler.NewRaw(log) }

// NewDiscovery returns the platform printer discovery.
func NewDiscovery(log ports.Logger) dp.Discovery { return spooler.NewDiscovery(log) }

// OSName is the running operating system.
func OSName() string { return "windows" }
