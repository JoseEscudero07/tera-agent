package gdi

import (
	"image"
	"image/color"
	"testing"
	"unsafe"
)

// TestBitmapInfoHeader_SizeMatches verifica que el struct tiene el mismo
// tamaño que espera Win32 (BITMAPINFOHEADER == 40 bytes). Un padding
// silencioso del compilador rompería el driver GDI sin dar señal clara.
func TestBitmapInfoHeader_SizeMatches(t *testing.T) {
	if got := unsafe.Sizeof(bitmapInfoHeader{}); got != 40 {
		t.Fatalf("sizeof(BITMAPINFOHEADER) = %d, want 40", got)
	}
}

// TestBmiHeader24 verifica los campos que rellena el helper — un cambio ahí
// (BitCount != 24, planes != 1, Compression != BI_RGB) rompería el driver.
func TestBmiHeader24(t *testing.T) {
	bi := bmiHeader24(100, 50, 15000)
	if bi.Size != 40 {
		t.Errorf("Size = %d, want 40", bi.Size)
	}
	if bi.Width != 100 || bi.Height != 50 {
		t.Errorf("Width/Height = %d/%d, want 100/50", bi.Width, bi.Height)
	}
	if bi.Planes != 1 {
		t.Errorf("Planes = %d, want 1", bi.Planes)
	}
	if bi.BitCount != 24 {
		t.Errorf("BitCount = %d, want 24", bi.BitCount)
	}
	if bi.Compression != bi_RGB {
		t.Errorf("Compression = %d, want %d (BI_RGB)", bi.Compression, bi_RGB)
	}
	if bi.SizeImage != 15000 {
		t.Errorf("SizeImage = %d, want 15000", bi.SizeImage)
	}
	if bi.Height <= 0 {
		t.Error("Height must be > 0 (bottom-up)")
	}
}

// TestBuildBandDIB_Gray_StrideAlignment verifica la alineación de fila del
// DIB (múltiplo de 4). Un stride mal calculado dejaría el driver leyendo mal
// las filas y produce imágenes recortadas / rayas de ruido.
func TestBuildBandDIB_Gray_StrideAlignment(t *testing.T) {
	// srcW = 7 → 7*3 = 21 bytes → padding a 24 (múltiplo de 4).
	g := image.NewGray(image.Rect(0, 0, 7, 4))
	for i := range g.Pix {
		g.Pix[i] = 0x80
	}
	buf, _, err := buildBandDIB(g, 0, 4, 7)
	if err != nil {
		t.Fatalf("buildBandDIB: %v", err)
	}
	wantStride := 24
	if got := len(buf) / 4; got != wantStride {
		t.Fatalf("stride = %d, want %d", got, wantStride)
	}
}

// TestBuildBandDIB_Gray_BottomUp verifica el volteo vertical. Un fallo aquí
// hace que la impresión salga invertida (cabeza abajo).
func TestBuildBandDIB_Gray_BottomUp(t *testing.T) {
	// Imagen 4x3: la primera fila top-down debe aparecer al final del DIB.
	g := image.NewGray(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			g.Pix[y*g.Stride+x] = uint8(y*10 + x)
		}
	}
	buf, _, err := buildBandDIB(g, 0, 3, 4)
	if err != nil {
		t.Fatalf("buildBandDIB: %v", err)
	}
	// stride = align4(4*3) = 12. La última fila del DIB (bytes [24..36))
	// debe ser la fila top-down 0 de la imagen (valores 0,1,2,3 replicados
	// tres veces BGR).
	stride := 12
	lastRowOffset := 2 * stride
	firstRowOfImage := []byte{0, 0, 0, 1, 1, 1, 2, 2, 2, 3, 3, 3}
	for i, want := range firstRowOfImage {
		if got := buf[lastRowOffset+i]; got != want {
			t.Errorf("DIB[%d] = %d, want %d (fila top-down 0 debe ir al final del DIB)",
				lastRowOffset+i, got, want)
		}
	}
	// Y la primera fila del DIB debe ser la fila 2 (bottom) de la imagen:
	// valores 20, 21, 22, 23.
	firstRowOfDIB := []byte{20, 20, 20, 21, 21, 21, 22, 22, 22, 23, 23, 23}
	for i, want := range firstRowOfDIB {
		if got := buf[i]; got != want {
			t.Errorf("DIB[%d] = %d, want %d (fila bottom debe ir al inicio del DIB)",
				i, got, want)
		}
	}
}

// TestBuildBandDIB_BandTop verifica que sólo se copian las filas de la banda,
// no la imagen entera. Un bug aquí duplicaría filas o dejaría zeros.
func TestBuildBandDIB_BandTop(t *testing.T) {
	// Imagen 2×10: valores predecibles por fila (fila y = valor y).
	g := image.NewGray(image.Rect(0, 0, 2, 10))
	for y := 0; y < 10; y++ {
		g.Pix[y*g.Stride+0] = uint8(y)
		g.Pix[y*g.Stride+1] = uint8(y)
	}
	// Banda [4, 7): filas 4, 5, 6.
	buf, _, err := buildBandDIB(g, 4, 3, 2)
	if err != nil {
		t.Fatalf("buildBandDIB: %v", err)
	}
	// stride = align4(2*3) = 8. La última fila del DIB (offset 16) debe ser
	// la fila 4 (top de la banda). La primera fila del DIB (offset 0) debe
	// ser la fila 6.
	stride := 8
	if buf[0] != 6 {
		t.Errorf("DIB primera fila = %d, want 6 (bottom de banda)", buf[0])
	}
	if buf[2*stride] != 4 {
		t.Errorf("DIB última fila = %d, want 4 (top de banda)", buf[2*stride])
	}
}

// TestBuildBandDIB_AnyImage cubre el path lento cuando la fuente no es
// image.Gray (RGBA, NRGBA, palettes, ...). Verifica el mismo layout BGR
// bottom-up y stride padded a 4.
func TestBuildBandDIB_AnyImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	// Fila 0: rojo. Fila 1: azul.
	for x := 0; x < 3; x++ {
		img.Set(x, 0, color.NRGBA{R: 255, A: 255})
		img.Set(x, 1, color.NRGBA{B: 255, A: 255})
	}
	buf, _, err := buildBandDIB(img, 0, 2, 3)
	if err != nil {
		t.Fatalf("buildBandDIB: %v", err)
	}
	stride := 12 // align4(3*3)=12
	// Primera fila DIB = fila 1 imagen (azul) → BGR = 255,0,0 por píxel.
	for x := 0; x < 3; x++ {
		if buf[x*3] != 255 || buf[x*3+1] != 0 || buf[x*3+2] != 0 {
			t.Errorf("DIB fila 0 pixel %d = %d,%d,%d; want 255,0,0 (azul BGR)",
				x, buf[x*3], buf[x*3+1], buf[x*3+2])
		}
	}
	// Segunda fila DIB = fila 0 imagen (rojo) → BGR = 0,0,255.
	for x := 0; x < 3; x++ {
		off := stride + x*3
		if buf[off] != 0 || buf[off+1] != 0 || buf[off+2] != 255 {
			t.Errorf("DIB fila 1 pixel %d = %d,%d,%d; want 0,0,255 (rojo BGR)",
				x, buf[off], buf[off+1], buf[off+2])
		}
	}
}

// TestBuildBandDIB_SizeImageMatchesBuffer: SizeImage en el header debe ser
// exactamente stride*height. Si difiere, StretchDIBits en Windows puede leer
// fuera de límites o rechazar el DIB.
func TestBuildBandDIB_SizeImageMatchesBuffer(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 100, 50))
	buf, biPtr, err := buildBandDIB(g, 0, 50, 100)
	if err != nil {
		t.Fatalf("buildBandDIB: %v", err)
	}
	bi := (*bitmapInfoHeader)(biPtr)
	if int(bi.SizeImage) != len(buf) {
		t.Fatalf("SizeImage = %d, buffer len = %d; deben coincidir",
			bi.SizeImage, len(buf))
	}
}
