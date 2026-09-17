//go:build windows

// Package platform selects the OS-specific printing adapters. On Windows it uses
// the print spooler (winspool) for RAW printing and enumeration, pdftocairo for
// PDF on page printers (vector mode) and GDI for raster pages (image mode).
// Owner: Printing Engineer.
package platform

import (
	"github.com/teraerp/tera-agent/internal/adapters/printing/driver/gdi"
	"github.com/teraerp/tera-agent/internal/adapters/printing/driver/pdfvector"
	spooler "github.com/teraerp/tera-agent/internal/adapters/printing/driver/spooler"
	"github.com/teraerp/tera-agent/internal/adapters/printing/popplerbin"
	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
	"golang.org/x/sys/windows"
)

// Drivers returns the platform driver set:
//   - spooler.NewRaw: térmicas / etiquetas (ESC/POS, ZPL, EPL, raw).
//   - pdfvector.New: PDF en láser / inyección, dibujado en el driver (modo vectorial).
//   - gdi.New: páginas raster por GDI (modo imagen, y PNG/JPEG en impresoras de hoja).
//
// El orden en el slice no importa: el resolver elige por Accepts(target), y la
// prioridad entre pdf y gdi-raster la marca el perfil (NormalizeForWindows).
func Drivers(log ports.Logger, bin dp.Binarizer) []dp.Driver {
	return []dp.Driver{
		spooler.NewRaw(log),
		pdfvector.New(log, popplerbin.Find("pdftocairo")),
		gdi.New(log, bin),
	}
}

// RawDriver returns the platform raw driver (for the `raw` command).
func RawDriver(log ports.Logger) dp.Driver { return spooler.NewRaw(log) }

// NewDiscovery returns the platform printer discovery.
func NewDiscovery(log ports.Logger) dp.Discovery { return spooler.NewDiscovery(log) }

// OSName is the running operating system.
func OSName() string { return "windows" }

// ProfileNormalizerForOS devuelve el normalizador que este SO necesita: en
// Windows traduce los perfiles de página según el modo vigente de cada impresora
// (ver NormalizeForWindows). modeOf nil = todas en modo vectorial. La lógica
// pura vive en normalize.go para poder testearse desde cualquier SO.
func ProfileNormalizerForOS(modeOf PageModeResolver) ProfileNormalizer {
	utf8ACP := supportsUTF8ActiveCodePage()
	return func(p dp.PrinterProfile) dp.PrinterProfile {
		mode := ports.PageModeVector
		if modeOf != nil {
			mode = modeOf(p.PrinterID)
		}
		return NormalizeForWindows(p, EffectivePageMode(mode, p.PrinterID, utf8ACP))
	}
}

// supportsUTF8ActiveCodePage: Windows respeta activeCodePage=UTF-8 en el
// manifiesto de un ejecutable desde Windows 10 1903 (build 18362).
func supportsUTF8ActiveCodePage() bool {
	return windows.RtlGetVersion().BuildNumber >= 18362
}
