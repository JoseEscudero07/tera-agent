// Lógica de normalización de perfiles independiente del SO. Vive fuera de los
// archivos con build tag para que se pueda ejercitar desde tests corriendo en
// Linux/mac sin cross-compilation. platform_windows.go la llama; en Linux/mac
// NormalizeProfile es identidad y esta función no se usa en runtime — pero
// existe para que la CI cubra el path Windows.
package platform

import (
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Umbrales del path raster de Windows. 600 DPI es el punto donde el texto
// deja de verse borroso y el DIB sigue por debajo del límite interno de
// ancho de origen de los drivers HP y PCL comunes (>~5000 wide → errno=158
// en StretchDIBits). Si el Backend envía un perfil con valores mayores se
// respetan, pero por debajo de este piso se suben para no imprimir borroso.
const (
	minRasterDPI       = 600
	minRasterWidthDots = 4960
)

// NormalizeForGDIRaster traduce un perfil pensado por el ERP para "PDF
// nativo" al camino real que sabemos ejecutar en Windows: rasterizar con
// Poppler y pintar con GDI. La transformación:
//
//   - Cada DevicePDF en NativeFormats se sustituye por DeviceGDIRaster.
//   - Duplicados eliminados preservando la prioridad (primer aparecido gana).
//   - Si el perfil resultante apunta a GDIRaster, se sube el DPI a
//     minRasterDPI y el WidthDots a minRasterWidthDots cuando venían por
//     debajo, para que la impresora reciba resolución nativa y no un bitmap
//     bajo que después se ve interpolado.
//
// Función pura: no toca el receptor, ni consulta variables globales. Testeable
// desde cualquier SO.
func NormalizeForGDIRaster(p dp.PrinterProfile) dp.PrinterProfile {
	if len(p.NativeFormats) == 0 {
		return p
	}
	out := make([]dp.DeviceFormat, 0, len(p.NativeFormats))
	seen := make(map[dp.DeviceFormat]bool, len(p.NativeFormats))
	rasterTarget := false
	for _, f := range p.NativeFormats {
		if f == dp.DevicePDF {
			f = dp.DeviceGDIRaster
		}
		if f == dp.DeviceGDIRaster {
			rasterTarget = true
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	p.NativeFormats = out
	if rasterTarget {
		if p.DPI < minRasterDPI {
			p.DPI = minRasterDPI
		}
		if p.WidthDots < minRasterWidthDots {
			p.WidthDots = minRasterWidthDots
		}
	}
	return p
}
