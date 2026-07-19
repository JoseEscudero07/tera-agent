//go:build linux || darwin

// Package platform selects the OS-specific printing adapters (drivers +
// discovery) so the rest of the system stays platform-agnostic. On Linux/macOS
// it uses CUPS (lp/lpstat). Owner: Printing Engineer.
package platform

import (
	"runtime"

	"github.com/teraerp/tera-agent/internal/adapters/printing/discovery"
	drivercups "github.com/teraerp/tera-agent/internal/adapters/printing/driver/cups"
	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Drivers returns the platform driver set (raw + native via CUPS).
func Drivers(log ports.Logger) []dp.Driver {
	return []dp.Driver{drivercups.NewRaw(log), drivercups.NewNative(log)}
}

// RawDriver returns the platform raw driver (for the `raw` command).
func RawDriver(log ports.Logger) dp.Driver { return drivercups.NewRaw(log) }

// NewDiscovery returns the platform printer discovery.
func NewDiscovery(log ports.Logger) dp.Discovery { return discovery.NewCUPS(log) }

// OSName is the running operating system.
func OSName() string { return runtime.GOOS }
