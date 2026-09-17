// Package poppler implements domain/printing.Rasterizer using Poppler's
// `pdftoppm`. It is the only place that knows about Poppler; replace it with
// MuPDF/PDFium/Ghostscript without touching the rest of the system.
// Owner: Printing Engineer.
package poppler

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/teraerp/tera-agent/internal/adapters/printing/popplerbin"
	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// pdftoppmTimeout limita cuánto puede tardar la rasterización de un job. Un PDF
// patológico o un pdftoppm colgado no debe dejar la petición HTTP recargando
// indefinidamente: se cancela el proceso hijo y la UI recibe el error.
//
// 90s deja margen para PDFs multi-página a 1200 DPI (la calidad máxima que el
// panel elige para láser). Un A4 a esa densidad se rasteriza en 3-5s por
// página en un equipo típico; 90s cubre documentos de ~20 páginas o CPUs
// lentas sin cortar trabajos legítimos.
const pdftoppmTimeout = 90 * time.Second

// Rasterizer converts PDF bytes into raster pages via pdftoppm.
type Rasterizer struct{ log ports.Logger }

// New returns a Poppler-based rasterizer.
func New(log ports.Logger) *Rasterizer { return &Rasterizer{log: log} }

// Rasterize renders every PDF page to grayscale PNG at the requested width (in
// dots) and decodes them with the standard library.
func (r *Rasterizer) Rasterize(ctx context.Context, pdf []byte, opts dp.RasterOptions) ([]image.Image, error) {
	if len(pdf) == 0 {
		return nil, fmt.Errorf("poppler: empty pdf")
	}

	tmp, err := os.MkdirTemp("", "tera-raster-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	in := filepath.Join(tmp, "in.pdf")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		return nil, err
	}

	args := []string{"-png"}
	if opts.Gray {
		args = append(args, "-gray")
	}
	switch {
	case opts.WidthDots > 0:
		// Integer scaling to the native dot width keeps text and code modules crisp.
		args = append(args, "-scale-to-x", strconv.Itoa(opts.WidthDots), "-scale-to-y", "-1")
	case opts.DPI > 0:
		args = append(args, "-r", strconv.Itoa(opts.DPI))
	}
	prefix := filepath.Join(tmp, "page")
	args = append(args, in, prefix)

	runCtx, cancel := context.WithTimeout(ctx, pdftoppmTimeout)
	defer cancel()
	bin := resolvePdftoppm(r.log)
	// Command lanza sin ventana de consola: sin eso, el binario de la bandeja
	// (sin consola propia) hace que Windows abra una ventana negra por rasterizado.
	cmd := popplerbin.Command(runCtx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := popplerbin.Run(cmd); err != nil {
		return nil, popplerbin.RunError(bin, err, stderr.String())
	}

	files, err := filepath.Glob(prefix + "*.png")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("poppler: no pages produced")
	}

	pages := make([]image.Image, 0, len(files))
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		img, err := png.Decode(fh)
		fh.Close()
		if err != nil {
			return nil, fmt.Errorf("poppler: decode %s: %w", filepath.Base(f), err)
		}
		pages = append(pages, img)
	}
	r.log.Debug("rasterized pdf", "pages", len(pages), "width", opts.WidthDots)
	return pages, nil
}

var _ dp.Rasterizer = (*Rasterizer)(nil)

// resolvePdftoppm devuelve la ruta de pdftoppm (ver popplerbin.Find para el
// orden de búsqueda).
//
// Cached: printing on the hot path calls this per page. Recompute is cheap
// pero innecesario, y ensucia el log cuando ya hemos elegido una ruta.
func resolvePdftoppm(log ports.Logger) string {
	pdftoppmOnce.Do(func() {
		pdftoppmPath = popplerbin.Find("pdftoppm")
		// Una sola línea al primer uso: hace trivial diagnosticar futuros
		// "no lo encuentra" (¿está el .exe junto al Agent? ¿cayó al PATH?).
		if log != nil {
			log.Info("pdftoppm resolved", "path", pdftoppmPath)
		}
	})
	return pdftoppmPath
}

var (
	pdftoppmOnce sync.Once
	pdftoppmPath string
)
