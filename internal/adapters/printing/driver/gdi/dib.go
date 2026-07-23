// Package gdi — construcción de DIBs bottom-up de 24bpp BGR sin dependencia
// de la API de Windows. Aquí sólo hay layout de bytes; el driver GDI (en
// gdi_windows.go) los pasa a StretchDIBits. Vive fuera del build tag para
// que los tests puedan verificar el layout desde cualquier SO.
package gdi

import (
	"image"
	"unsafe"
)

// bitmapInfoHeader = BITMAPINFOHEADER de Win32. Se declara aquí para poder
// construir el DIB sin importar gdi32 desde tests; los offsets y tamaño de
// campos deben coincidir con la ABI x86_64 de Windows.
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// bi_RGB = BI_RGB (compresión: ninguna). Constante Win32; se replica aquí
// para no depender de la DLL en tests.
const bi_RGB = 0

// buildBandDIB arma un DIB de 24bpp BGR con sólo las filas
// [bandTop, bandTop+bandH) de la imagen fuente.
//
// Antes había un fast path de 8bpp con paleta gris (más eficiente en memoria).
// Se quitó porque el driver de la HP Universal Print Driver (y varios drivers
// PCL antiguos) rechazan DIBs de 8bpp devolviendo el genérico "el segmento
// está desbloqueado" (Win32 158). 24bpp es el formato universal aceptado por
// todos los drivers Windows y sigue siendo suficientemente rápido para el
// tamaño de bandas que usamos.
//
// El BITMAPINFOHEADER se devuelve como unsafe.Pointer para que el GC lo
// mantenga vivo hasta que el syscall lo consuma.
func buildBandDIB(img image.Image, bandTop, bandH, srcW int32) ([]byte, unsafe.Pointer, error) {
	// Fast path cuando la fuente ya es 8-bit gray (Poppler con -gray): evitar
	// image.At() en el hot loop es ~10x en tiempo de armado.
	if g, ok := img.(*image.Gray); ok {
		return buildBandDIB24FromGray(g, bandTop, bandH, srcW)
	}
	return buildBandDIB24FromAny(img, bandTop, bandH, srcW)
}

// buildBandDIB24FromGray expande gris 8bpp a BGR 24bpp copiando cada byte
// tres veces. Sigue siendo mucho más rápido que At()/RGBA().
func buildBandDIB24FromGray(g *image.Gray, bandTop, bandH, srcW int32) ([]byte, unsafe.Pointer, error) {
	stride := ((int(srcW)*3 + 3) / 4) * 4
	buf := make([]byte, stride*int(bandH))
	minX, minY := g.Rect.Min.X, g.Rect.Min.Y
	for y := int32(0); y < bandH; y++ {
		srcRow := bandTop + (bandH - 1 - y) // bottom-up
		off := (minY+int(srcRow)-g.Rect.Min.Y)*g.Stride + (minX - g.Rect.Min.X)
		src := g.Pix[off : off+int(srcW)]
		row := buf[int(y)*stride : int(y)*stride+stride]
		for x := 0; x < int(srcW); x++ {
			v := src[x]
			row[x*3+0] = v // B
			row[x*3+1] = v // G
			row[x*3+2] = v // R
		}
	}
	return buf, unsafe.Pointer(bmiHeader24(srcW, bandH, uint32(stride*int(bandH)))), nil
}

func buildBandDIB24FromAny(img image.Image, bandTop, bandH, srcW int32) ([]byte, unsafe.Pointer, error) {
	stride := ((int(srcW)*3 + 3) / 4) * 4
	buf := make([]byte, stride*int(bandH))
	bounds := img.Bounds()
	minX, minY := bounds.Min.X, bounds.Min.Y
	for y := int32(0); y < bandH; y++ {
		srcRow := bandTop + (bandH - 1 - y)
		row := buf[int(y)*stride : int(y)*stride+stride]
		for x := int32(0); x < srcW; x++ {
			cr, cg, cb, _ := img.At(minX+int(x), minY+int(srcRow)).RGBA()
			row[x*3+0] = uint8(cb >> 8)
			row[x*3+1] = uint8(cg >> 8)
			row[x*3+2] = uint8(cr >> 8)
		}
	}
	return buf, unsafe.Pointer(bmiHeader24(srcW, bandH, uint32(stride*int(bandH)))), nil
}

// bmiHeader24 crea un BITMAPINFOHEADER de 24bpp para un DIB bottom-up. Se
// factoriza para no repetir 8 líneas idénticas en cada fast path.
func bmiHeader24(width, height int32, sizeImage uint32) *bitmapInfoHeader {
	return &bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       width,
		Height:      height, // positivo = bottom-up
		Planes:      1,
		BitCount:    24,
		Compression: bi_RGB,
		SizeImage:   sizeImage,
	}
}
