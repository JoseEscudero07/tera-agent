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

// cutFeedDots is fed (ESC J n) before cutting so the last content line clears
// the cutter blade (the blade sits ~10-15mm above the print head). Too small a
// feed cuts THROUGH the last line (e.g. the date on the test ticket comes out
// halved); this is tuned so the cut lands cleanly below the content.
//
// ▶ ESTE ES EL VALOR A AJUSTAR SI EL CORTE QUEDA MAL:
//   - Corta el contenido / la última línea sale partida → SUBIR el número.
//   - Deja demasiado papel en blanco tras el corte      → BAJAR el número.
//   1 mm ≈ 8 dots a 203dpi. Rango válido: 0–255 (ESC J admite un solo byte).
//   Tras cambiarlo hay que recompilar y reinstalar el binario (ver docs/CORTE.md).
const cutFeedDots = 232 // ~29mm at 203dpi

// ESC/POS control sequences.
var (
	cmdInit       = []byte{0x1B, 0x40}                   // ESC @  (initialize)
	cmdFullCut    = []byte{0x1D, 0x56, 0x00}             // GS V 0 (full cut)
	cmdDrawer     = []byte{0x1B, 0x70, 0x00, 0x19, 0xFA} // ESC p 0 (kick cash drawer)
	rasterStart   = []byte{0x1D, 0x76, 0x30, 0x00}       // GS v 0 m=0
	feedBeforeCut = []byte{0x1B, 0x4A, cutFeedDots}      // ESC J n (feed n dots)
)

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
		// Halve the blank top margin of the first page so the receipt starts
		// closer to the content (leaves ~half the leading paper it did before).
		if i == 0 {
			mb = halveLeadingBlank(mb)
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
		buf.Write(feedBeforeCut) // clear the cutter, then cut right after content
		buf.Write(cmdFullCut)
	}
	return buf.Bytes(), nil
}

// halveLeadingBlank removes HALF of the fully-blank rows at the top so the
// receipt starts closer to the content without cropping it. Leaving half (rather
// than all) keeps a small, deliberate top margin.
func halveLeadingBlank(mb *dp.MonoBitmap) *dp.MonoBitmap {
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
	remove := blank / 2
	if remove == 0 {
		return mb
	}
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
