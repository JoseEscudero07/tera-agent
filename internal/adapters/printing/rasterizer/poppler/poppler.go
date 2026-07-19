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
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

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

	args := []string{"-png", "-gray"}
	switch {
	case opts.WidthDots > 0:
		// Integer scaling to the native dot width keeps text and code modules crisp.
		args = append(args, "-scale-to-x", strconv.Itoa(opts.WidthDots), "-scale-to-y", "-1")
	case opts.DPI > 0:
		args = append(args, "-r", strconv.Itoa(opts.DPI))
	}
	prefix := filepath.Join(tmp, "page")
	args = append(args, in, prefix)

	cmd := exec.CommandContext(ctx, "pdftoppm", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("poppler: pdftoppm: %w: %s", err, stderr.String())
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
