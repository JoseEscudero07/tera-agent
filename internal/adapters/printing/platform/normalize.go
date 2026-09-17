// Lógica de normalización de perfiles independiente del SO. Vive fuera de los
// archivos con build tag para que se pueda ejercitar desde tests corriendo en
// Linux/mac sin cross-compilation. platform_windows.go la llama; en Linux/mac no
// hay normalizador (ProfileNormalizerForOS devuelve nil) y estas funciones no se
// usan en runtime — pero existen aquí para que la CI cubra el path Windows.
package platform

import (
	"github.com/teraerp/tera-agent/internal/app/ports"
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

// PageModeResolver devuelve el modo de impresión vigente de una impresora de
// página. Es una función y no un valor porque el panel puede cambiarlo en
// caliente.
type PageModeResolver func(printerID string) ports.PageMode

// NormalizeForWindows traduce un perfil pensado por el ERP para "PDF nativo"
// (las impresoras "normales" llegan como native_formats ["pdf"]) a lo que
// Windows sabe ejecutar, según el modo de la impresora:
//
//   - PageModeVector (defecto): [pdf, gdi-raster]. El driver vectorial imprime el
//     PDF tal cual. gdi-raster queda detrás por dos motivos: los trabajos PNG/JPEG
//     solo tienen ese camino, y si pdftocairo no está instalado el driver
//     vectorial no acepta pdf y el resolver cae al modo imagen al componer el
//     pipeline —antes de enviar nada, así que nunca se imprime dos veces.
//   - PageModeImage: [gdi-raster], el camino raster de siempre.
//
// Los perfiles que no son de página (térmicas, etiquetas) no se tocan. Función
// pura: testeable desde cualquier SO.
func NormalizeForWindows(p dp.PrinterProfile, mode ports.PageMode) dp.PrinterProfile {
	out := NormalizeForGDIRaster(p)
	if mode == ports.PageModeImage {
		return out
	}
	// NormalizeForGDIRaster ya dejó un único gdi-raster donde estaba el primer
	// pdf/gdi-raster: el PDF va justo delante, con la misma prioridad relativa
	// respecto a los demás formatos.
	formats := make([]dp.DeviceFormat, 0, len(out.NativeFormats)+1)
	for _, f := range out.NativeFormats {
		if f == dp.DeviceGDIRaster {
			formats = append(formats, dp.DevicePDF)
		}
		formats = append(formats, f)
	}
	out.NativeFormats = formats
	return out
}

// EffectivePageMode decide el modo con el que se imprime de verdad.
//
// El modo vectorial pasa el nombre de la impresora a pdftocairo, que lo usa con
// las funciones ANSI de Windows. El instalador le pone un manifiesto UTF-8 para
// que los nombres con tildes o ñ funcionen, pero Windows solo respeta ese
// manifiesto desde Windows 10 1903 (utf8ACP). En un Windows anterior, una
// impresora con nombre no ASCII fallaría con "Printer not found" en cada
// trabajo: mejor imprimirla en modo imagen que no imprimir.
func EffectivePageMode(configured ports.PageMode, printerID string, utf8ACP bool) ports.PageMode {
	if configured == ports.PageModeImage {
		return ports.PageModeImage
	}
	if !utf8ACP && !isASCII(printerID) {
		return ports.PageModeImage
	}
	return ports.PageModeVector
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// NormalizeForGDIRaster traduce un perfil pensado por el ERP para "PDF
// nativo" al camino raster de Windows (modo imagen): rasterizar con Poppler y
// pintar con GDI. La transformación:
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
