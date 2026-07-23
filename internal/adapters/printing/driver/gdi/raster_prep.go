// Package gdi — preparación pura del raster para el blit por GDI. Sin build tag
// ni syscalls: sólo transforma imágenes y arma buffers de DIB, para poder
// testearlo desde cualquier SO. La parte Windows (StretchDIBits) vive en
// gdi_windows.go.
//
// Por qué existe: los drivers gráficos de Windows (sobre todo los host-based
// tipo Samsung/HP Laser) sólo digieren ~3–4 MB de bitmap por página en una
// StretchDIBits, y rechazan tanto un DIB grande como muchos pequeños
// (errno=158). La estrategia robusta —validada contra hardware real— es mandar
// UNA sola StretchDIBits por página con un DIB que quepa en ese presupuesto:
//
//   - Contenido en gris  -> 1 bpp (bilevel). Un A4 a 500 dpi son ~3 MB y en
//     láser se ve nítido: es el formato que el driver realmente quiere.
//   - Contenido a color   -> 24 bpp, reduciendo resolución hasta que quepa.
//
// El driver empieza a la resolución nativa y baja (backoff) si el driver
// concreto rechaza el DIB, así cada impresora recibe la mejor calidad que
// acepta sin conocer su límite de antemano.
package gdi

import (
	"image"
	"image/color"
	"math"
	"unsafe"
)

// firstDIBCap acota el tamaño del DIB del PRIMER intento para no construir ni
// enviar bitmaps absurdamente grandes (una página A4 color a 600 dpi son
// ~104 MB). 8 MB deja margen para drivers generosos; si el driver lo rechaza,
// el backoff baja desde ahí. No es el límite del driver, sólo el punto de
// arranque.
const firstDIBCap = 8 << 20

// minDIBBytes es el suelo: por debajo de esto la calidad sería inservible y
// preferimos fallar con un error claro a imprimir una mancha.
const minDIBBytes = 256 << 10

// dibBytes24 y dibBytes1 devuelven el tamaño en bytes de un DIB de w×h a 24bpp
// y 1bpp respectivamente (con stride alineado a DWORD como exige GDI).
func dibBytes24(w, h int) int { return stride24(w) * h }
func dibBytes1(w, h int) int  { return stride1(w) * h }

func stride24(w int) int { return ((w*3 + 3) / 4) * 4 }
func stride1(w int) int  { return ((w + 31) / 32) * 4 }

// initialScale calcula el factor de escala del primer intento para que el DIB
// resultante no supere firstDIBCap. Nunca amplía (máximo 1.0).
func initialScale(srcW, srcH int, mono bool) float64 {
	var native int
	if mono {
		native = dibBytes1(srcW, srcH)
	} else {
		native = dibBytes24(srcW, srcH)
	}
	if native <= firstDIBCap {
		return 1.0
	}
	return math.Sqrt(float64(firstDIBCap) / float64(native))
}

// scaledDims aplica un factor de escala a unas dimensiones, con mínimo de 1 px.
func scaledDims(srcW, srcH int, scale float64) (int, int) {
	w := int(float64(srcW) * scale)
	h := int(float64(srcH) * scale)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// isGrayscale reporta si la imagen no tiene color (todos los píxeles R=G=B).
// *image.Gray es gris por definición; para el resto escanea y corta al primer
// píxel con color. No es hot path (un job por impresión).
func isGrayscale(img image.Image) bool {
	if _, ok := img.(*image.Gray); ok {
		return true
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r != g || g != bl {
				return false
			}
		}
	}
	return true
}

// downscale reduce img a dstW×dstH promediando por cajas (area-average), que
// conserva bordes y texto mejor que el vecino más cercano. Si el destino no es
// menor devuelve la imagen tal cual (nunca amplía aquí; el escalado hacia
// arriba lo hace StretchDIBits en el destino). Conserva el tipo cuando puede:
// *image.Gray -> *image.Gray; el resto -> *image.RGBA.
func downscale(img image.Image, dstW, dstH int) image.Image {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if dstW >= sw && dstH >= sh {
		return img
	}
	if g, ok := img.(*image.Gray); ok {
		return downscaleGray(g, dstW, dstH)
	}
	return downscaleRGBA(img, dstW, dstH)
}

func downscaleGray(src *image.Gray, dstW, dstH int) *image.Gray {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewGray(image.Rect(0, 0, dstW, dstH))
	for dy := 0; dy < dstH; dy++ {
		y0, y1 := span(dy, dstH, sh)
		for dx := 0; dx < dstW; dx++ {
			x0, x1 := span(dx, dstW, sw)
			var sum, n uint32
			for yy := y0; yy < y1; yy++ {
				row := src.Pix[(b.Min.Y+yy)*src.Stride+(b.Min.X):]
				for xx := x0; xx < x1; xx++ {
					sum += uint32(row[xx])
					n++
				}
			}
			dst.Pix[dy*dst.Stride+dx] = uint8(sum / n)
		}
	}
	return dst
}

func downscaleRGBA(img image.Image, dstW, dstH int) *image.RGBA {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for dy := 0; dy < dstH; dy++ {
		y0, y1 := span(dy, dstH, sh)
		for dx := 0; dx < dstW; dx++ {
			x0, x1 := span(dx, dstW, sw)
			var sr, sg, sb, n uint32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					r, g, bl, _ := img.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
					sr += r >> 8
					sg += g >> 8
					sb += bl >> 8
					n++
				}
			}
			dst.Set(dx, dy, color.RGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 0xFF})
		}
	}
	return dst
}

// span devuelve el rango [lo,hi) de píxeles fuente que caen en el píxel destino
// idx de un eje que va de dstN a srcN, garantizando al menos un píxel.
func span(idx, dstN, srcN int) (int, int) {
	lo := idx * srcN / dstN
	hi := (idx + 1) * srcN / dstN
	if hi <= lo {
		hi = lo + 1
	}
	return lo, hi
}

// fitRect encaja un contenido srcW×srcH dentro de un área destino (printW×printH)
// conservando la proporción y centrándolo (letterbox). Devuelve el rectángulo
// destino en coordenadas del área imprimible (origen 0,0 = esquina imprimible).
func fitRect(srcW, srcH, printW, printH int) (x, y, w, h int) {
	if srcW <= 0 || srcH <= 0 {
		return 0, 0, printW, printH
	}
	scale := math.Min(float64(printW)/float64(srcW), float64(printH)/float64(srcH))
	w = int(float64(srcW) * scale)
	h = int(float64(srcH) * scale)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	x = (printW - w) / 2
	y = (printH - h) / 2
	return x, y, w, h
}

// ---- DIB monocromo (1 bpp) ----

// rgbQuad es RGBQUAD de Win32 (orden B,G,R,reservado) para la tabla de color.
type rgbQuad struct{ B, G, R, Reserved byte }

// bitmapInfo1bpp = BITMAPINFOHEADER + 2 entradas de paleta (índice 0 y 1). GDI
// exige la tabla de color contigua tras la cabecera para DIBs de 1 bpp.
type bitmapInfo1bpp struct {
	hdr    bitmapInfoHeader
	colors [2]rgbQuad
}

// packMono1bpp reempaqueta un MonoBitmap (stride (w+7)/8, bit=1 => negro) en un
// buffer de DIB top-down con stride alineado a DWORD, y devuelve el
// BITMAPINFO con la paleta {0:blanco, 1:negro} — así el bit=1=negro del
// MonoBitmap mapea al índice 1 = negro sin invertir nada.
func packMono1bpp(width, height int, bits []byte, srcStride int) ([]byte, unsafe.Pointer) {
	dibStride := stride1(width)
	buf := make([]byte, dibStride*height)
	for y := 0; y < height; y++ {
		copy(buf[y*dibStride:y*dibStride+srcStride], bits[y*srcStride:y*srcStride+srcStride])
	}
	bi := &bitmapInfo1bpp{
		hdr: bitmapInfoHeader{
			Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			Width:       int32(width),
			Height:      -int32(height), // negativo = top-down (orden natural del MonoBitmap)
			Planes:      1,
			BitCount:    1,
			Compression: bi_RGB,
			SizeImage:   uint32(len(buf)),
			ClrUsed:     2,
		},
		colors: [2]rgbQuad{{0xFF, 0xFF, 0xFF, 0}, {0, 0, 0, 0}},
	}
	return buf, unsafe.Pointer(bi)
}
