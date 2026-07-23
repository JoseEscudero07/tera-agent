package gdi

import (
	"image"
	"image/color"
	"testing"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

func TestStrides(t *testing.T) {
	// 24bpp: alineado a DWORD.
	if got := stride24(4960); got != 14880 {
		t.Errorf("stride24(4960)=%d, want 14880", got)
	}
	if got := stride24(1); got != 4 { // 3 bytes -> padded a 4
		t.Errorf("stride24(1)=%d, want 4", got)
	}
	// 1bpp: alineado a DWORD.
	if got := stride1(4960); got != 620 {
		t.Errorf("stride1(4960)=%d, want 620", got)
	}
	if got := stride1(1); got != 4 {
		t.Errorf("stride1(1)=%d, want 4", got)
	}
}

func TestInitialScale(t *testing.T) {
	// 1bpp A4 completo (~4.35MB) está bajo firstDIBCap(8MB) -> sin reducir.
	if s := initialScale(4960, 7016, true); s != 1.0 {
		t.Errorf("mono A4 initialScale=%v, want 1.0", s)
	}
	// 24bpp A4 completo (~104MB) supera el cap -> reduce por debajo de 1.
	s := initialScale(4960, 7016, false)
	if s >= 1.0 || s <= 0 {
		t.Fatalf("color A4 initialScale=%v, want (0,1)", s)
	}
	if b := dibBytes24(int(float64(4960)*s), int(float64(7016)*s)); b > firstDIBCap {
		t.Errorf("color initialScale deja DIB=%d > cap=%d", b, firstDIBCap)
	}
}

func TestScaledDims(t *testing.T) {
	w, h := scaledDims(4960, 7016, 0.5)
	if w != 2480 || h != 3508 {
		t.Errorf("scaledDims=%dx%d, want 2480x3508", w, h)
	}
	// Nunca cae a 0.
	if w, h := scaledDims(3, 3, 0.0001); w < 1 || h < 1 {
		t.Errorf("scaledDims mínimo=%dx%d, want >=1", w, h)
	}
}

func TestFitRect(t *testing.T) {
	// Fuente más ancha que alta en un destino cuadrado: encaja por ancho,
	// centra en vertical (letterbox).
	x, y, w, h := fitRect(200, 100, 1000, 1000)
	if w != 1000 || h != 500 {
		t.Errorf("fit w,h=%d,%d want 1000,500", w, h)
	}
	if x != 0 || y != 250 {
		t.Errorf("fit x,y=%d,%d want 0,250", x, y)
	}
	// Proporción preservada.
	if float64(w)/float64(h) != 2.0 {
		t.Errorf("proporción alterada: %d/%d", w, h)
	}
}

func TestIsGrayscale(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 4, 4))
	if !isGrayscale(g) {
		t.Error("image.Gray debe ser gris")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < 4; i++ {
		rgba.Set(i, 0, color.RGBA{100, 100, 100, 255}) // gris neutro
	}
	if !isGrayscale(rgba) {
		t.Error("RGBA neutro (R=G=B) debe detectarse como gris")
	}
	rgba.Set(1, 1, color.RGBA{255, 0, 0, 255}) // un píxel rojo
	if isGrayscale(rgba) {
		t.Error("RGBA con un píxel de color NO es gris")
	}
}

func TestDownscaleGrayAverages(t *testing.T) {
	// 2x2 mitad negra (arriba) mitad blanca (abajo) -> 1x1 debe promediar a gris.
	g := image.NewGray(image.Rect(0, 0, 2, 2))
	g.Pix = []byte{0, 0, 255, 255}
	out := downscale(g, 1, 1).(*image.Gray)
	if got := out.Pix[0]; got < 120 || got > 135 {
		t.Errorf("downscale promedio=%d, want ~127", got)
	}
	// No amplía.
	if out := downscale(g, 10, 10); out.Bounds().Dx() != 2 {
		t.Errorf("downscale no debe ampliar: %v", out.Bounds())
	}
}

func TestPackMono1bpp(t *testing.T) {
	// MonoBitmap 8x2, stride 1: fila0=0xFF (8 bits negros), fila1=0x00.
	m := &dp.MonoBitmap{Width: 8, Height: 2, Bits: []byte{0xFF, 0x00}}
	buf, biPtr := packMono1bpp(m.Width, m.Height, m.Bits, m.Stride())
	if biPtr == nil {
		t.Fatal("bi nil")
	}
	ds := stride1(8) // 4 (DWORD)
	if len(buf) != ds*2 {
		t.Fatalf("buf len=%d, want %d", len(buf), ds*2)
	}
	// Los bits de origen se copian al inicio de cada fila del DIB.
	if buf[0] != 0xFF {
		t.Errorf("fila0 byte0=%#x, want 0xFF", buf[0])
	}
	if buf[ds] != 0x00 {
		t.Errorf("fila1 byte0=%#x, want 0x00", buf[ds])
	}
}
