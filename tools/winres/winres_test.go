package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// fakeICO arma un .ico mínimo con n imágenes de datos reconocibles.
func fakeICO(n int) []byte {
	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(n))
	off := 6 + 16*n
	var blobs [][]byte
	for i := 0; i < n; i++ {
		blob := bytes.Repeat([]byte{byte(0xA0 + i)}, 32+i)
		blobs = append(blobs, blob)
		buf.WriteByte(byte(16 * (i + 1))) // ancho
		buf.WriteByte(byte(16 * (i + 1))) // alto
		buf.WriteByte(0)
		buf.WriteByte(0)
		_ = binary.Write(buf, binary.LittleEndian, uint16(1))
		_ = binary.Write(buf, binary.LittleEndian, uint16(32))
		_ = binary.Write(buf, binary.LittleEndian, uint32(len(blob)))
		_ = binary.Write(buf, binary.LittleEndian, uint32(off))
		off += len(blob)
	}
	for _, b := range blobs {
		buf.Write(b)
	}
	return buf.Bytes()
}

// leaf es un recurso ya extraído del árbol del .syso.
type leaf struct {
	typ, id uint16
	data    []byte
}

// readRsrc recorre el árbol de recursos de la sección .rsrc tal y como lo haría
// Windows: tipo → identificador → idioma → entrada de datos.
func readRsrc(t *testing.T, sec []byte) []leaf {
	t.Helper()
	var out []leaf
	entries := func(dir int) [][2]uint32 {
		if dir+16 > len(sec) {
			t.Fatalf("directorio fuera de la sección: %d", dir)
		}
		n := int(binary.LittleEndian.Uint16(sec[dir+12:])) + int(binary.LittleEndian.Uint16(sec[dir+14:]))
		var es [][2]uint32
		for i := 0; i < n; i++ {
			at := dir + 16 + 8*i
			es = append(es, [2]uint32{
				binary.LittleEndian.Uint32(sec[at:]),
				binary.LittleEndian.Uint32(sec[at+4:]),
			})
		}
		return es
	}
	for _, typeEntry := range entries(0) {
		if typeEntry[1]&0x80000000 == 0 {
			t.Fatal("el nivel de tipos debe apuntar a subdirectorios")
		}
		for _, idEntry := range entries(int(typeEntry[1] &^ 0x80000000)) {
			for _, langEntry := range entries(int(idEntry[1] &^ 0x80000000)) {
				if langEntry[1]&0x80000000 != 0 {
					t.Fatal("el nivel de idioma debe apuntar a datos, no a otro directorio")
				}
				de := int(langEntry[1])
				off := binary.LittleEndian.Uint32(sec[de:])
				size := binary.LittleEndian.Uint32(sec[de+4:])
				if int(off)+int(size) > len(sec) {
					t.Fatalf("datos fuera de la sección: off=%d size=%d", off, size)
				}
				out = append(out, leaf{
					typ:  uint16(typeEntry[0]),
					id:   uint16(idEntry[0]),
					data: sec[off : off+size],
				})
			}
		}
	}
	return out
}

// El .syso tiene que ser un COFF que Go pueda enlazar y llevar dentro el icono
// completo: una entrada RT_ICON por tamaño y el RT_GROUP_ICON que las enumera.
func TestSysoLlevaElIconoEntero(t *testing.T) {
	res, err := iconResources(fakeICO(3), 1)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := writeCOFF(res, "amd64")
	if err != nil {
		t.Fatal(err)
	}

	f, err := pe.NewFile(bytes.NewReader(obj))
	if err != nil {
		t.Fatalf("el .syso no es un COFF válido: %v", err)
	}
	defer f.Close()
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		t.Errorf("Machine = %#x", f.Machine)
	}
	sec := f.Section(".rsrc")
	if sec == nil {
		t.Fatal("falta la sección .rsrc")
	}
	data, err := sec.Data()
	if err != nil {
		t.Fatal(err)
	}

	leaves := readRsrc(t, data)
	if len(leaves) != 4 { // 3 iconos + el grupo
		t.Fatalf("recursos = %d; se esperaban 4", len(leaves))
	}
	var group []byte
	icons := map[uint16][]byte{}
	for _, l := range leaves {
		switch l.typ {
		case rtIcon:
			icons[l.id] = l.data
		case rtGroupIcon:
			group = l.data
		default:
			t.Errorf("tipo de recurso inesperado: %d", l.typ)
		}
	}
	if len(icons) != 3 || group == nil {
		t.Fatalf("iconos = %d, grupo = %v", len(icons), group != nil)
	}
	// El grupo debe describir las mismas imágenes y referenciarlas por id.
	if n := binary.LittleEndian.Uint16(group[4:]); n != 3 {
		t.Fatalf("el grupo declara %d imágenes", n)
	}
	for i := 0; i < 3; i++ {
		e := group[6+14*i:]
		id := binary.LittleEndian.Uint16(e[12:])
		size := binary.LittleEndian.Uint32(e[8:])
		if got := icons[id]; len(got) != int(size) {
			t.Errorf("imagen %d: el grupo dice %d bytes y el recurso %d tiene %d", i+1, size, id, len(got))
		}
		if want := byte(16 * (i + 1)); e[0] != want {
			t.Errorf("imagen %d: ancho = %d; se esperaba %d", i+1, e[0], want)
		}
	}

	// Cada entrada de datos necesita su relocalización: sin ella el ejecutable
	// apuntaría el icono a una dirección que no existe.
	if len(sec.Relocs) != len(leaves) {
		t.Fatalf("relocalizaciones = %d; se esperaba una por recurso (%d)", len(sec.Relocs), len(leaves))
	}
	for _, r := range sec.Relocs {
		if r.Type != relAMD64Addr32NB {
			t.Errorf("tipo de relocalización = %d", r.Type)
		}
		if int(r.VirtualAddress)+4 > len(data) {
			t.Fatalf("relocalización fuera de la sección: %d", r.VirtualAddress)
		}
	}
}

// Los datos de versión son lo que hace que el cuadro de UAC diga "Tera Agent" y
// que las propiedades del fichero muestren la versión del release.
func TestRecursoDeVersion(t *testing.T) {
	res, err := versionResource(versionInfo{
		version:     "1.2.3-rc.1",
		company:     "Grupo Tera",
		product:     "Tera Agent",
		description: "Tera Agent",
		internal:    "tera-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	b := res.data
	if got := int(binary.LittleEndian.Uint16(b)); got != len(b) {
		t.Errorf("wLength = %d; el bloque mide %d", got, len(b))
	}
	if key := utf16str(b[6:]); key != "VS_VERSION_INFO" {
		t.Fatalf("clave raíz = %q", key)
	}
	// VS_FIXEDFILEINFO va detrás de la clave, alineado a 4.
	fixed := align(6+2*len("VS_VERSION_INFO\x00"), 4)
	if sig := binary.LittleEndian.Uint32(b[fixed:]); sig != 0xFEEF04BD {
		t.Fatalf("firma de VS_FIXEDFILEINFO = %#x", sig)
	}
	if ms, ls := binary.LittleEndian.Uint32(b[fixed+8:]), binary.LittleEndian.Uint32(b[fixed+12:]); ms != 1<<16|2 || ls != 3<<16 {
		t.Errorf("versión = %d.%d.%d.%d; se esperaba 1.2.3.0", ms>>16, ms&0xffff, ls>>16, ls&0xffff)
	}
	for _, want := range []string{"StringFileInfo", "040904B0", "FileDescription", "Tera Agent", "1.2.3-rc.1", "Translation"} {
		if !bytes.Contains(b, utf16z(want)[:2*len([]rune(want))]) {
			t.Errorf("el bloque de versión no contiene %q", want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	ok := map[string][4]uint16{
		"1.2.3":      {1, 2, 3, 0},
		"1.2.3-rc.1": {1, 2, 3, 0},
		"10.0.0.7":   {10, 0, 0, 7},
		"0.0.0-dev":  {0, 0, 0, 0},
	}
	for in, want := range ok {
		got, err := parseVersion(in)
		if err != nil || got != want {
			t.Errorf("parseVersion(%q) = %v, %v; se esperaba %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "1.2", "x.y.z", "1.2.3.4.5", "1.2.-3", "70000.0.0"} {
		if _, err := parseVersion(bad); err == nil {
			t.Errorf("parseVersion(%q) debería fallar", bad)
		}
	}
}

// Un .ico corrupto tiene que parar el build, no colar un recurso a medias.
func TestParseICORechazaFicherosInvalidos(t *testing.T) {
	good := fakeICO(2)
	cases := map[string][]byte{
		"vacío":               nil,
		"no es un ico":        []byte("MZ\x00\x00\x00\x00"),
		"sin imágenes":        {0, 0, 1, 0, 0, 0},
		"directorio truncado": good[:10],
		"datos fuera":         append(append([]byte{}, good[:6+12]...), []byte{0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0}...),
	}
	for name, in := range cases {
		if _, err := parseICO(in); err == nil {
			t.Errorf("%s: parseICO debería fallar", name)
		}
	}
}

// utf16str lee una cadena UTF-16 terminada en cero.
func utf16str(b []byte) string {
	var units []uint16
	for i := 0; i+1 < len(b); i += 2 {
		u := binary.LittleEndian.Uint16(b[i:])
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}
