//go:build windows

// Package platform selects the OS-specific printing adapters. On Windows it uses
// the print spooler (winspool) for RAW printing and enumeration. There is no
// graphical/GDI driver path in the MVP: documents (PDF) go through the
// rasterizer -> ESC/POS pipeline like on the other platforms.
// Owner: Printing Engineer.
package platform

import (
	"github.com/teraerp/tera-agent/internal/adapters/printing/driver/gdi"
	spooler "github.com/teraerp/tera-agent/internal/adapters/printing/driver/spooler"
	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Drivers returns the platform driver set:
//   - spooler.NewRaw: térmicas / etiquetas (ESC/POS, ZPL, EPL, raw).
//   - gdi.New: láser / inyección / virtual PDF, vía GDI + spooler gráfico.
//
// El orden en el slice no importa: el resolver elige por Accepts(target).
func Drivers(log ports.Logger, bin dp.Binarizer) []dp.Driver {
	return []dp.Driver{spooler.NewRaw(log), gdi.New(log, bin)}
}

// RawDriver returns the platform raw driver (for the `raw` command).
func RawDriver(log ports.Logger) dp.Driver { return spooler.NewRaw(log) }

// NewDiscovery returns the platform printer discovery.
func NewDiscovery(log ports.Logger) dp.Discovery { return spooler.NewDiscovery(log) }

// OSName is the running operating system.
func OSName() string { return "windows" }

// ProfileNormalizerForOS devuelve el normalizador que este SO necesita: en
// Windows traduce DevicePDF → DeviceGDIRaster (rasterizar y pintar por GDI)
// y sube DPI/WidthDots al mínimo raster. La lógica pura vive en normalize.go
// para poder testearse desde cualquier SO.
func ProfileNormalizerForOS() ProfileNormalizer { return NormalizeForGDIRaster }
