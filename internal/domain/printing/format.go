package printing

// SourceFormat is the format of the document the ERP sends (MIME-like).
type SourceFormat string

const (
	FormatPDF    SourceFormat = "application/pdf"
	FormatPNG    SourceFormat = "image/png"
	FormatJPEG   SourceFormat = "image/jpeg"
	FormatText   SourceFormat = "text/plain"
	FormatESCPOS SourceFormat = "application/vnd.escpos"
	FormatZPL    SourceFormat = "application/vnd.zpl"
)

// DeviceFormat is what a driver/printer consumes directly.
type DeviceFormat string

const (
	DeviceESCPOS DeviceFormat = "escpos"
	DeviceZPL    DeviceFormat = "zpl"
	DeviceEPL    DeviceFormat = "epl"
	DevicePDF    DeviceFormat = "pdf"
	DevicePNG    DeviceFormat = "png"
	DeviceRaw    DeviceFormat = "raw"
	// DeviceGDIRaster son páginas raster empaquetadas para el driver GDI de
	// Windows (láser/inyección/virtual PDF). El driver decodifica y pinta cada
	// página vía StretchDIBits; el formato del contenedor lo define el paquete
	// encoder/raster. No debe verse fuera del par encoder↔driver.
	DeviceGDIRaster DeviceFormat = "gdi-raster"
)

// deviceOfSource maps source formats that are already device-native, enabling a
// pass-through pipeline without any per-format conditional in the resolver.
var deviceOfSource = map[SourceFormat]DeviceFormat{
	FormatPDF:    DevicePDF,
	FormatPNG:    DevicePNG,
	FormatESCPOS: DeviceESCPOS,
	FormatZPL:    DeviceZPL,
}

// SourceIsDevice reports whether source is already the given device format
// (pass-through case).
func SourceIsDevice(source SourceFormat, device DeviceFormat) bool {
	d, ok := deviceOfSource[source]
	return ok && d == device
}
