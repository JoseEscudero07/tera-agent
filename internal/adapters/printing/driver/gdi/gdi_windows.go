//go:build windows

// Package gdi imprime raster páginas en Windows a través del driver gráfico de
// la impresora (GDI + spooler). Es el camino correcto para láser / inyección /
// impresoras virtuales PDF: cede al driver del sistema la parte de PDL (PCL/PS)
// y sólo le entrega bitmaps.
//
// Estrategia (ver raster_prep.go para el porqué): UNA sola StretchDIBits por
// página, con un DIB que quepa en el presupuesto que el driver acepta. El
// contenido en gris va a 1 bpp (nítido en láser), el de color a 24 bpp; si el
// driver rechaza el DIB (errno=158) se baja resolución y se reintenta.
//
// Owner: Printing Engineer. La superficie de la API winspool/gdi32 se toca sólo
// aquí; el resto del código sigue viendo un dp.Driver anónimo.
package gdi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/teraerp/tera-agent/internal/adapters/printing/encoder/raster"
	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// ---------- Win32 bindings ----------

var (
	gdi32                 = syscall.NewLazyDLL("gdi32.dll")
	procCreateDCW         = gdi32.NewProc("CreateDCW")
	procDeleteDC          = gdi32.NewProc("DeleteDC")
	procStartDocW         = gdi32.NewProc("StartDocW")
	procStartPage         = gdi32.NewProc("StartPage")
	procEndPage           = gdi32.NewProc("EndPage")
	procEndDoc            = gdi32.NewProc("EndDoc")
	procAbortDoc          = gdi32.NewProc("AbortDoc")
	procGetDeviceCaps     = gdi32.NewProc("GetDeviceCaps")
	procStretchDIBits     = gdi32.NewProc("StretchDIBits")
	procSetStretchBltMode = gdi32.NewProc("SetStretchBltMode")
)

// GetDeviceCaps indices.
const (
	horzRes    = 8  // ancho del área imprimible (px)
	vertRes    = 10 // alto del área imprimible (px)
	logPixelsX = 88 // DPI lógico horizontal
	logPixelsY = 90 // DPI lógico vertical
)

// StretchBltMode: COLORONCOLOR es el default de Windows y el que aceptan sin
// quejarse todos los drivers conocidos. No requiere SetBrushOrgEx. La calidad
// del reescalado destino es suficiente porque mandamos el raster ya cerca de la
// resolución del dispositivo.
const colorOnColor = 3

// DIB / StretchDIBits constants.
const (
	diRGBColors  = 0
	bitBltSrcCpy = 0x00CC0020 // SRCCOPY
)

// errDIBReject señala que StretchDIBits rechazó el DIB (típicamente errno=158
// en drivers host-based por tamaño). Es reintentable bajando resolución.
var errDIBReject = errors.New("gdi: driver rechazó el DIB (reintentable)")

// DOCINFO datatype vacío = deja al driver elegir.
type docInfoW struct {
	cbSize      int32
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
	fwType      uint32
}

// ---------- Driver ----------

// Driver renderiza páginas raster contra el HDC de la impresora del spooler de
// Windows. Sólo acepta el contenedor privado DeviceGDIRaster. Usa un Binarizer
// para el camino monocromo (contenido en gris → 1 bpp nítido).
type Driver struct {
	log ports.Logger
	bin dp.Binarizer
}

// New devuelve el driver GDI. bin puede ser nil: sin él, el contenido en gris
// se imprime igualmente por el camino de color (24 bpp) en vez de 1 bpp.
func New(log ports.Logger, bin dp.Binarizer) dp.Driver { return &Driver{log: log, bin: bin} }

func (d *Driver) Accepts(f dp.DeviceFormat) bool { return f == dp.DeviceGDIRaster }

func (d *Driver) Send(_ context.Context, printerID string, data []byte) error {
	if printerID == "" {
		return fmt.Errorf("gdi: empty printer id")
	}
	pngs, err := raster.DecodeMultiPNG(data)
	if err != nil {
		return fmt.Errorf("gdi: decode container: %w", err)
	}
	if len(pngs) == 0 {
		return fmt.Errorf("gdi: no pages")
	}
	pages := make([]image.Image, len(pngs))
	for i, b := range pngs {
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			return fmt.Errorf("gdi: decode page %d: %w", i, err)
		}
		pages[i] = img
	}

	name, err := syscall.UTF16PtrFromString(printerID)
	if err != nil {
		return err
	}

	// Backoff adaptativo: intenta a la mejor resolución que quepa y baja si el
	// driver rechaza el DIB. Cada intento fallido hace AbortDoc y no gasta papel.
	scaleMul := 1.0
	for attempt := 0; attempt < 12; attempt++ {
		err := d.renderDoc(name, printerID, pages, scaleMul)
		if err == nil {
			d.log.Info("print job submitted (gdi)", "printer", printerID,
				"pages", len(pages), "scale", fmt.Sprintf("%.2f", scaleMul))
			return nil
		}
		if !errors.Is(err, errDIBReject) {
			return err
		}
		next := scaleMul * 0.8
		if d.belowFloor(pages, next) {
			return fmt.Errorf("gdi: la impresora %q rechaza el bitmap incluso a baja "+
				"resolución (errno 158); su driver no acepta este tamaño de página", printerID)
		}
		d.log.Debug("gdi DIB rechazado, bajando resolución", "printer", printerID,
			"scale", fmt.Sprintf("%.2f->%.2f", scaleMul, next))
		scaleMul = next
	}
	return fmt.Errorf("gdi: %q no aceptó la página tras varios intentos", printerID)
}

// belowFloor reporta si, al factor dado, la página más grande produciría un DIB
// por debajo del mínimo útil (no vale la pena seguir bajando).
func (d *Driver) belowFloor(pages []image.Image, scaleMul float64) bool {
	max := 0
	for _, pg := range pages {
		b := pg.Bounds()
		mono := isGrayscale(pg) && d.bin != nil
		s := initialScale(b.Dx(), b.Dy(), mono) * scaleMul
		w, h := scaledDims(b.Dx(), b.Dy(), s)
		n := dibBytes24(w, h)
		if mono {
			n = dibBytes1(w, h)
		}
		if n > max {
			max = n
		}
	}
	return max < minDIBBytes
}

// renderDoc abre el documento e imprime todas las páginas al factor dado.
// Devuelve errDIBReject (envuelto) si alguna StretchDIBits rechaza el DIB.
func (d *Driver) renderDoc(name *uint16, printerID string, pages []image.Image, scaleMul float64) error {
	hdc, _, e := procCreateDCW.Call(0, uintptr(unsafe.Pointer(name)), 0, 0)
	if hdc == 0 {
		return fmt.Errorf("gdi: CreateDC %q: %v", printerID, e)
	}
	defer procDeleteDC.Call(hdc)

	printableW := int32(deviceCap(hdc, horzRes))
	printableH := int32(deviceCap(hdc, vertRes))
	if printableW <= 0 || printableH <= 0 {
		return fmt.Errorf("gdi: printer reports empty printable area")
	}

	docName, _ := syscall.UTF16PtrFromString("Tera Agent PDF")
	di := docInfoW{cbSize: int32(unsafe.Sizeof(docInfoW{})), pDocName: docName}
	if job, _, e := procStartDocW.Call(hdc, uintptr(unsafe.Pointer(&di))); int32(job) <= 0 {
		return fmt.Errorf("gdi: StartDoc: %v", e)
	}
	success := false
	defer func() {
		if success {
			procEndDoc.Call(hdc)
		} else {
			procAbortDoc.Call(hdc)
		}
	}()
	procSetStretchBltMode.Call(hdc, colorOnColor)

	for _, pg := range pages {
		if err := d.drawPage(hdc, pg, scaleMul, printableW, printableH); err != nil {
			return err // errDIBReject u otro; el defer hace AbortDoc
		}
	}
	success = true
	return nil
}

// drawPage prepara el DIB de una página (1 bpp si es gris, 24 bpp si color) al
// factor pedido y lo pinta con UNA StretchDIBits, encajado y centrado en el
// área imprimible conservando la proporción.
func (d *Driver) drawPage(hdc uintptr, img image.Image, scaleMul float64, printableW, printableH int32) error {
	b := img.Bounds()
	mono := isGrayscale(img) && d.bin != nil
	scale := initialScale(b.Dx(), b.Dy(), mono) * scaleMul
	tw, th := scaledDims(b.Dx(), b.Dy(), scale)
	small := downscale(img, tw, th)

	var buf []byte
	var bi unsafe.Pointer
	var srcW, srcH int32
	if mono {
		m := d.bin.Binarize(small)
		buf, bi = packMono1bpp(m.Width, m.Height, m.Bits, m.Stride())
		srcW, srcH = int32(m.Width), int32(m.Height)
	} else {
		sb := small.Bounds()
		srcW, srcH = int32(sb.Dx()), int32(sb.Dy())
		var err error
		buf, bi, err = buildBandDIB(small, 0, srcH, srcW)
		if err != nil {
			return fmt.Errorf("gdi: build DIB: %w", err)
		}
	}
	if srcW <= 0 || srcH <= 0 {
		return fmt.Errorf("gdi: empty page image")
	}

	if r, _, e := procStartPage.Call(hdc); int32(r) <= 0 {
		return fmt.Errorf("gdi: StartPage: %v", e)
	}
	x, y, w, h := fitRect(int(srcW), int(srcH), int(printableW), int(printableH))
	r, _, e := procStretchDIBits.Call(
		hdc,
		uintptr(int32(x)), uintptr(int32(y)), uintptr(int32(w)), uintptr(int32(h)),
		0, 0, uintptr(srcW), uintptr(srcH),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(bi),
		diRGBColors,
		bitBltSrcCpy,
	)
	runtime.KeepAlive(bi)
	runtime.KeepAlive(buf)
	if int32(r) == 0 {
		// El driver rechazó el DIB. No hacemos EndPage: el AbortDoc de renderDoc
		// descarta la página. errno 158 (host-based por tamaño) es lo típico.
		return fmt.Errorf("%w: StretchDIBits %dx%d errno=%d", errDIBReject, srcW, srcH, errnoOf(e))
	}
	if r, _, e := procEndPage.Call(hdc); int32(r) <= 0 {
		return fmt.Errorf("gdi: EndPage: %v", e)
	}
	return nil
}

// deviceCap wraps GetDeviceCaps.
func deviceCap(hdc uintptr, idx int) int {
	r, _, _ := procGetDeviceCaps.Call(hdc, uintptr(idx))
	return int(int32(r))
}

// errnoOf extrae el código numérico Win32 de un error de syscall (0 si no lo es).
func errnoOf(err error) uintptr {
	if e, ok := err.(syscall.Errno); ok {
		return uintptr(e)
	}
	return 0
}
