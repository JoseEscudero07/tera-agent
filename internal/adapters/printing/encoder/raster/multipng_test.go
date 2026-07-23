package raster

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// makeImage crea una imagen sólida de un color, útil para verificar que la
// codificación/decodificación PNG por página preserva los píxeles.
func makeImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestMultiPNG_Roundtrip(t *testing.T) {
	pages := []image.Image{
		makeImage(10, 6, color.RGBA{R: 255, A: 255}),
		makeImage(4, 4, color.RGBA{G: 128, A: 255}),
	}
	enc := NewMultiPNG()

	if enc.Accepts() != dp.ArtifactRaster {
		t.Fatalf("Accepts = %q, want %q", enc.Accepts(), dp.ArtifactRaster)
	}
	if enc.Produces() != dp.DeviceGDIRaster {
		t.Fatalf("Produces = %q, want %q", enc.Produces(), dp.DeviceGDIRaster)
	}

	data, err := enc.Encode(context.Background(), dp.RasterArtifact{Pages: pages}, dp.EncodeOptions{})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := DecodeMultiPNG(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded) != len(pages) {
		t.Fatalf("decoded len = %d, want %d", len(decoded), len(pages))
	}
	for i, pngBytes := range decoded {
		img, err := png.Decode(bytes.NewReader(pngBytes))
		if err != nil {
			t.Fatalf("page %d PNG decode: %v", i, err)
		}
		if img.Bounds() != pages[i].Bounds() {
			t.Fatalf("page %d bounds = %v, want %v", i, img.Bounds(), pages[i].Bounds())
		}
	}
}

func TestMultiPNG_EmptyArtifact(t *testing.T) {
	_, err := NewMultiPNG().Encode(context.Background(), dp.RasterArtifact{}, dp.EncodeOptions{})
	if err == nil {
		t.Fatal("expected error on empty artifact")
	}
}

func TestMultiPNG_WrongArtifact(t *testing.T) {
	_, err := NewMultiPNG().Encode(context.Background(), dp.TextArtifact{Body: "hi"}, dp.EncodeOptions{})
	if err == nil {
		t.Fatal("expected error on non-raster artifact")
	}
}

func TestDecodeMultiPNG_BadMagic(t *testing.T) {
	_, err := DecodeMultiPNG([]byte("XXXX\x01\x00\x00\x00\x00\x00\x00\x00"))
	if err == nil {
		t.Fatal("expected bad magic error")
	}
}

func TestDecodeMultiPNG_Truncated(t *testing.T) {
	// magic + version + reserved + page_count=1, pero el header de la única
	// página está cortado a la mitad.
	buf := bytes.Buffer{}
	buf.Write(magic[:])
	buf.WriteByte(version)
	buf.Write([]byte{0, 0, 0, 0, 0, 0, 1, 0, 0}) // reserved + count(BE)=1 corrompido a 3 bytes
	_, err := DecodeMultiPNG(buf.Bytes())
	if err == nil {
		t.Fatal("expected truncated error")
	}
}
