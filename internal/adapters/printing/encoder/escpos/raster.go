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

// ESC/POS control sequences.
var (
	cmdInit     = []byte{0x1B, 0x40}                   // ESC @  (initialize)
	cmdFullCut  = []byte{0x1D, 0x56, 0x00}             // GS V 0 (full cut)
	cmdDrawer   = []byte{0x1B, 0x70, 0x00, 0x19, 0xFA} // ESC p 0 (kick cash drawer)
	rasterStart = []byte{0x1D, 0x76, 0x30, 0x00}       // GS v 0 m=0
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
	for _, page := range ra.Pages {
		writeRaster(&buf, e.bin.Binarize(page))
	}
	if opts.OpenDrawer {
		buf.Write(cmdDrawer)
	}
	if opts.Cut {
		buf.Write(cmdFullCut)
	}
	return buf.Bytes(), nil
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
