package gdi

import "testing"

// hpLaser son las capacidades reales medidas en una HP Laser MFP 131 (A4,
// 600dpi): la hoja completa es 4960x7014 px y el área imprimible 4760x6814,
// con 100 px (≈4,2mm) de margen físico por lado.
var hpLaser = pageArea{
	PhysW: 4960, PhysH: 7014,
	PrintW: 4760, PrintH: 6814,
	OffX: 100, OffY: 100,
	DPIX: 600, DPIY: 600,
}

// Con margen 0 la página se mapea 1:1 con la HOJA FÍSICA, no con el área
// imprimible. Las coordenadas salen negativas a propósito: el origen de dibujo
// es la esquina del área imprimible, así que -100,-100 sitúa el borde del papel.
//
// Es la regresión del bug que motivó todo esto: antes se devolvía (0,0,4760,6814),
// que encogía el A4 al 96% y sumaba el margen del hardware al del propio PDF.
func TestTargetRectSinMargenUsaLaHojaCompleta(t *testing.T) {
	x, y, w, h := targetRect(hpLaser, 0)
	if x != -100 || y != -100 {
		t.Errorf("origen = (%d,%d); se esperaba (-100,-100) = borde físico del papel", x, y)
	}
	if w != 4960 || h != 7014 {
		t.Errorf("tamaño = %dx%d; se esperaba 4960x7014 (hoja completa)", w, h)
	}
}

// Un margen en mm se traduce a píxeles del dispositivo y se mide desde el borde
// del papel, así que el rectángulo encoge 2x el margen en cada eje.
func TestTargetRectConMargen(t *testing.T) {
	// 10mm a 600dpi = 236 px (10/25.4*600 = 236.22 -> redondeo)
	const px = 236
	x, y, w, h := targetRect(hpLaser, 10)
	if x != px-100 || y != px-100 {
		t.Errorf("origen = (%d,%d); se esperaba (%d,%d)", x, y, px-100, px-100)
	}
	if w != 4960-2*px || h != 7014-2*px {
		t.Errorf("tamaño = %dx%d; se esperaba %dx%d", w, h, 4960-2*px, 7014-2*px)
	}
}

// Con un margen mayor que el margen físico de la impresora, el destino cae
// dentro del área imprimible y las coordenadas ya son positivas.
func TestTargetRectMargenMayorQueElFisico(t *testing.T) {
	x, y, _, _ := targetRect(hpLaser, 8) // 8mm = 189px > 100px de margen físico
	if x <= 0 || y <= 0 {
		t.Errorf("origen = (%d,%d); con margen > margen físico debe ser positivo", x, y)
	}
}

// Una impresora sin margen físico (p. ej. "Microsoft Print to PDF") no debe
// desplazar nada: hoja física == área imprimible.
func TestTargetRectSinMargenFisico(t *testing.T) {
	virtual := pageArea{PhysW: 4961, PhysH: 7016, PrintW: 4961, PrintH: 7016, DPIX: 600, DPIY: 600}
	x, y, w, h := targetRect(virtual, 0)
	if x != 0 || y != 0 || w != 4961 || h != 7016 {
		t.Errorf("(%d,%d,%d,%d); se esperaba (0,0,4961,7016)", x, y, w, h)
	}
}

// Si el driver no reporta geometría física fiable, hay que caer al área
// imprimible en vez de calcular con ceros y mandar la página a ninguna parte.
func TestTargetRectGeometriaInvalida(t *testing.T) {
	casos := map[string]pageArea{
		"sin tamaño físico":            {PrintW: 4760, PrintH: 6814, DPIX: 600, DPIY: 600},
		"sin dpi":                      {PhysW: 4960, PhysH: 7014, PrintW: 4760, PrintH: 6814},
		"imprimible mayor que la hoja": {PhysW: 100, PhysH: 100, PrintW: 4760, PrintH: 6814, DPIX: 600, DPIY: 600},
	}
	for nombre, a := range casos {
		x, y, w, h := targetRect(a, 0)
		if x != 0 || y != 0 || w != a.PrintW || h != a.PrintH {
			t.Errorf("%s: (%d,%d,%d,%d); se esperaba el área imprimible (0,0,%d,%d)",
				nombre, x, y, w, h, a.PrintW, a.PrintH)
		}
	}
}

// Un margen absurdo dejaría un área nula o negativa; hay que degradar al área
// imprimible en vez de intentar imprimir en un rectángulo imposible.
func TestTargetRectMargenAbsurdo(t *testing.T) {
	x, y, w, h := targetRect(hpLaser, 200) // 200mm por lado en un A4 de 210mm
	if x != 0 || y != 0 || w != hpLaser.PrintW || h != hpLaser.PrintH {
		t.Errorf("(%d,%d,%d,%d); se esperaba caer al área imprimible", x, y, w, h)
	}
}

func TestMmToPx(t *testing.T) {
	casos := []struct {
		mm   float64
		dpi  int
		want int
	}{
		{0, 600, 0},
		{25.4, 600, 600}, // una pulgada
		{10, 600, 236},   // 236.22 -> 236
		{10, 300, 118},   // 118.11 -> 118
		{-5, 600, 0},     // negativo = sin margen
		{10, 0, 0},       // sin dpi no se puede convertir
	}
	for _, c := range casos {
		if got := mmToPx(c.mm, c.dpi); got != c.want {
			t.Errorf("mmToPx(%v, %d) = %d; se esperaba %d", c.mm, c.dpi, got, c.want)
		}
	}
}
