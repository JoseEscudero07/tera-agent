// Package poppler implements domain/printing.Rasterizer using Poppler's
// `pdftoppm`. It is the only place that knows about Poppler; replace it with
// MuPDF/PDFium/Ghostscript without touching the rest of the system.
// Owner: Printing Engineer.
package poppler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

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
	cmd := exec.CommandContext(runCtx, bin, args...)
	// Sin esto, el binario de la bandeja (sin consola propia) hace que Windows
	// abra una ventana negra en cada rasterizado.
	hideConsole(cmd)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, runError(bin, err, stderr.String())
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

// dllNotFoundExit es STATUS_DLL_NOT_FOUND (0xC0000135) visto como código de
// salida: Windows no llegó a ejecutar el binario porque le falta una DLL.
const dllNotFoundExit = -1073741515

// runError explica por qué falló pdftoppm.
//
// El caso 0xC0000135 merece mensaje propio: el proceso no arranca siquiera, así
// que stderr viene vacío y el error crudo ("exit status 0xc0000135") no dice nada
// a quien da soporte en el equipo de un cliente. En la práctica siempre significa
// lo mismo: falta el runtime de Visual C++ que pdftoppm y varias DLLs de poppler
// importan, y que no viene incluido en el bundle de poppler.
func runError(bin string, err error, stderr string) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == dllNotFoundExit {
		return dllNotFoundError(bin)
	}
	if stderr == "" {
		return fmt.Errorf("poppler: pdftoppm (%s): %w", bin, err)
	}
	return fmt.Errorf("poppler: pdftoppm (%s): %w: %s", bin, err, stderr)
}

// dllNotFoundError redacta el diagnóstico de 0xC0000135 con la acción concreta a
// tomar, que es lo que necesita quien está delante del equipo del cliente.
func dllNotFoundError(bin string) error {
	return fmt.Errorf("poppler: %s no pudo arrancar: falta una DLL requerida (0xC0000135). "+
		"Normalmente es el runtime de Visual C++: comprueba que msvcp140.dll, "+
		"vcruntime140.dll y vcruntime140_1.dll estén junto a pdftoppm.exe, o instala "+
		"el Microsoft Visual C++ Redistributable (x64)", bin)
}

// resolvePdftoppm finds the pdftoppm binary. Order:
//  1. TERA_PDFTOPPM env var (absolute path). Useful for tests and admins.
//  2. Same directory as the running executable — allows shipping pdftoppm.exe
//     next to tera-agent.exe on Windows without editing PATH (services run as
//     LocalSystem and don't see per-user PATH).
//  3. A ./poppler/bin/ subfolder next to the executable — convenient for the
//     Windows installer, which drops the poppler bundle there.
//  4. exec.LookPath sobre el PATH del proceso. En Windows, cuando LookPath
//     resuelve por el CWD, Go 1.19+ marca ErrDot y devuelve la ruta igual:
//     la aceptamos convertida a absoluta, así funciona aunque el usuario
//     lance el Agent desde la carpeta donde vive pdftoppm.exe.
//
// Cached: printing on the hot path calls this per page. Recompute is cheap
// pero innecesario, y ensucia el log cuando ya hemos elegido una ruta.
func resolvePdftoppm(log ports.Logger) string {
	pdftoppmOnce.Do(func() {
		pdftoppmPath = findPdftoppm()
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

func findPdftoppm() string {
	bin := "pdftoppm"
	if runtime.GOOS == "windows" {
		bin = "pdftoppm.exe"
	}

	if p := os.Getenv("TERA_PDFTOPPM"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(dir, bin),
			filepath.Join(dir, "poppler", "bin", bin),
			filepath.Join(dir, "bin", bin),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}

	// PATH del proceso. Aceptamos ErrDot (resolución vía CWD) porque el
	// nombre "pdftoppm" no es ambiguo: el usuario lo instaló a propósito y
	// que el binario esté en el mismo directorio desde el que se lanzó el
	// Agent es un caso legítimo. Convertimos a ruta absoluta para que
	// exec.CommandContext no vuelva a chocar con la misma protección.
	if p, err := exec.LookPath(bin); p != "" && (err == nil || errors.Is(err, exec.ErrDot)) {
		if abs, aerr := filepath.Abs(p); aerr == nil {
			return abs
		}
		return p
	}

	return bin
}
