// Package escpos encodes intermediate artifacts into ESC/POS command bytes.
// It knows the device language but nothing about the transport.
// Owner: Printing Engineer.
package escpos

import (
	"bytes"
	"context"
	"fmt"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// bandRows bounds the height of each GS v 0 raster block so low-end printers
// with small buffers accept the image.
const bandRows = 128

// Defaults used when the print job carries no per-printer calibration
// (EncodeOptions.CutFeedDots / TopMarginDots == 0). These are now configurable
// per printer in the agent config (printer.cut_feed_dots / top_margin_dots), so
// a client can tune the cut for any printer model WITHOUT recompiling — see
// docs/CORTE.md. 1mm ≈ 8 dots @203dpi.
const (
	// defaultCutFeedDots feeds paper (ESC J) before cutting so the last line
	// clears the blade (the blade sits ~10-18mm above the print head).
	defaultCutFeedDots = 232 // ~29mm
	// defaultTopMarginDots is how many blank top rows to keep; the rest is
	// trimmed so the receipt starts close to the content.
	defaultTopMarginDots = 16 // ~2mm
	maxFeedDots          = 255 // ESC J takes a single byte
)

// ESC/POS control sequences.
var (
	cmdInit     = []byte{0x1B, 0x40}                   // ESC @  (initialize)
	cmdFullCut  = []byte{0x1D, 0x56, 0x00}             // GS V 0 (full cut)
	cmdDrawer   = []byte{0x1B, 0x70, 0x00, 0x19, 0xFA} // ESC p 0 (kick cash drawer)
	rasterStart = []byte{0x1D, 0x76, 0x30, 0x00}       // GS v 0 m=0
)

// escJFeed returns "ESC J n" (feed n dots), resolving 0 to the default and
// clamping to the one-byte maximum.
func escJFeed(dots int) []byte {
	if dots <= 0 {
		dots = defaultCutFeedDots
	}
	if dots > maxFeedDots {
		dots = maxFeedDots
	}
	return []byte{0x1B, 0x4A, byte(dots)}
}

// resolveTopMargin resolves 0 to the default top margin.
func resolveTopMargin(dots int) int {
	if dots <= 0 {
		return defaultTopMarginDots
	}
	return dots
}

// Raster encodes raster pages as ESC/POS bit images using a Binarizer.
type Raster struct{ bin dp.Binarizer }

// NewRaster returns an ESC/POS raster encoder using the given binarizer.
func NewRaster(bin dp.Binarizer) dp.Encoder { return &Raster{bin: bin} }

func (Raster) Accepts() dp.ArtifactKind  { return dp.ArtifactRaster }
func (Raster) Produces() dp.DeviceFormat { return dp.DeviceESCPOS }

func (e *Raster) Encode(_ context.Context, a dp.Artifact, opts dp.EncodeOptions) ([]byte, error) {
	ra, ok := a.(dp.RasterArtifact)
	if !ok {
		return nil, fmt.Errorf("escpos raster: expected raster artifact, got %s", a.Kind())
	}

	var buf bytes.Buffer
	buf.Write(cmdInit)
	for i, page := range ra.Pages {
		// Trim trailing blank rows so we don't feed (and cut after) the empty
		// bottom margin of the document — the main cause of wasted paper.
		mb := trimTrailingBlank(e.bin.Binarize(page))
		// Trim the blank top margin of the first page down to the configured
		// number of dots so the receipt starts close to the content.
		if i == 0 {
			mb = trimLeadingBlankTo(mb, resolveTopMargin(opts.TopMarginDots))
		}
		if mb.Height == 0 {
			continue
		}
		writeRaster(&buf, mb)
	}
	if opts.OpenDrawer {
		buf.Write(cmdDrawer)
	}
	if opts.Cut {
		buf.Write(escJFeed(opts.CutFeedDots)) // clear the cutter, then cut below content
		buf.Write(cmdFullCut)
	}
	return buf.Bytes(), nil
}

// trimLeadingBlankTo removes fully-blank rows from the top until at most `keep`
// blank rows remain, so the receipt starts close to the content without cropping
// it. If there is already less blank than `keep`, the bitmap is unchanged.
func trimLeadingBlankTo(mb *dp.MonoBitmap, keep int) *dp.MonoBitmap {
	if keep < 0 {
		keep = 0
	}
	stride := mb.Stride()
	blank := 0
	for blank < mb.Height {
		allWhite := true
		for _, b := range mb.Bits[blank*stride : (blank+1)*stride] {
			if b != 0 {
				allWhite = false
				break
			}
		}
		if !allWhite {
			break
		}
		blank++
	}
	if blank <= keep {
		return mb
	}
	remove := blank - keep
	return &dp.MonoBitmap{Width: mb.Width, Height: mb.Height - remove, Bits: mb.Bits[remove*stride:]}
}

// trimTrailingBlank returns the bitmap without its trailing all-white rows.
func trimTrailingBlank(mb *dp.MonoBitmap) *dp.MonoBitmap {
	stride := mb.Stride()
	h := mb.Height
	for h > 0 {
		blank := true
		for _, b := range mb.Bits[(h-1)*stride : h*stride] {
			if b != 0 {
				blank = false
				break
			}
		}
		if !blank {
			break
		}
		h--
	}
	if h == mb.Height {
		return mb
	}
	return &dp.MonoBitmap{Width: mb.Width, Height: h, Bits: mb.Bits[:h*stride]}
}

// writeRaster emits one or more GS v 0 blocks (banded by bandRows).
func writeRaster(buf *bytes.Buffer, mb *dp.MonoBitmap) {
	stride := mb.Stride()
	for y0 := 0; y0 < mb.Height; y0 += bandRows {
		h := bandRows
		if y0+h > mb.Height {
			h = mb.Height - y0
		}
		buf.Write(rasterStart)
		buf.WriteByte(byte(stride & 0xFF))
		buf.WriteByte(byte((stride >> 8) & 0xFF))
		buf.WriteByte(byte(h & 0xFF))
		buf.WriteByte(byte((h >> 8) & 0xFF))
		buf.Write(mb.Bits[y0*stride : (y0+h)*stride])
	}
}
