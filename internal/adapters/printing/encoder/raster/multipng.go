// Package raster empaqueta un RasterArtifact como páginas PNG contiguas para el
// driver GDI de Windows. El contenedor es un contrato privado entre este
// encoder y el driver: no se documenta al Backend ni viaja por la red.
// Owner: Printing Engineer.
package raster

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image/png"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// magic marca la cabecera del contenedor. 4 bytes ASCII "TGDR" (Tera GDI Raster).
var magic = [4]byte{'T', 'G', 'D', 'R'}

// version del formato del contenedor. Sube si cambia el layout.
const version uint8 = 1

// MultiPNG codifica RasterArtifact como:
//
//	magic[4] | version[1] | reserved[3] | pageCount(uint32 BE)
//	[ pageLen(uint32 BE) | png_bytes ] * pageCount
//
// El driver reproduce el mismo layout al decodificar. Ver DecodeMultiPNG.
type MultiPNG struct{}

// NewMultiPNG devuelve el encoder.
func NewMultiPNG() dp.Encoder { return MultiPNG{} }

func (MultiPNG) Accepts() dp.ArtifactKind  { return dp.ArtifactRaster }
func (MultiPNG) Produces() dp.DeviceFormat { return dp.DeviceGDIRaster }

func (MultiPNG) Encode(_ context.Context, a dp.Artifact, _ dp.EncodeOptions) ([]byte, error) {
	ra, ok := a.(dp.RasterArtifact)
	if !ok {
		return nil, fmt.Errorf("raster/multipng: expected raster artifact, got %s", a.Kind())
	}
	if len(ra.Pages) == 0 {
		return nil, fmt.Errorf("raster/multipng: empty artifact")
	}
	var out bytes.Buffer
	out.Grow(64 * 1024)
	out.Write(magic[:])
	out.WriteByte(version)
	out.Write([]byte{0, 0, 0}) // reserved
	if err := binary.Write(&out, binary.BigEndian, uint32(len(ra.Pages))); err != nil {
		return nil, err
	}
	var page bytes.Buffer
	for i, img := range ra.Pages {
		page.Reset()
		if err := png.Encode(&page, img); err != nil {
			return nil, fmt.Errorf("raster/multipng: encode page %d: %w", i, err)
		}
		if err := binary.Write(&out, binary.BigEndian, uint32(page.Len())); err != nil {
			return nil, err
		}
		out.Write(page.Bytes())
	}
	return out.Bytes(), nil
}

// DecodeMultiPNG parsea el contenedor y devuelve las páginas PNG en bruto (una
// entrada por página). Lo usa el driver GDI de Windows; expuesto para que otros
// consumidores (tests) puedan verificar la ida y vuelta.
func DecodeMultiPNG(data []byte) ([][]byte, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("raster/multipng: short header")
	}
	if !bytes.Equal(data[:4], magic[:]) {
		return nil, fmt.Errorf("raster/multipng: bad magic")
	}
	if data[4] != version {
		return nil, fmt.Errorf("raster/multipng: unsupported version %d", data[4])
	}
	n := binary.BigEndian.Uint32(data[8:12])
	pages := make([][]byte, 0, n)
	off := 12
	for i := uint32(0); i < n; i++ {
		if off+4 > len(data) {
			return nil, fmt.Errorf("raster/multipng: truncated at page %d header", i)
		}
		pl := int(binary.BigEndian.Uint32(data[off : off+4]))
		off += 4
		if off+pl > len(data) {
			return nil, fmt.Errorf("raster/multipng: truncated at page %d body", i)
		}
		pages = append(pages, data[off:off+pl])
		off += pl
	}
	return pages, nil
}
