package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// versionInfo son los datos que Windows enseña en las propiedades del fichero y,
// lo que más se nota, en el cuadro de UAC al instalar: sin esto pone el nombre
// del ejecutable y "Editor desconocido" a secas.
type versionInfo struct {
	version     string // X.Y.Z[-pre]
	company     string
	product     string
	description string
	copyright   string
	internal    string
}

// versionResource construye el bloque VS_VERSIONINFO (RT_VERSION).
func versionResource(v versionInfo) (resource, error) {
	nums, err := parseVersion(v.version)
	if err != nil {
		return resource{}, err
	}

	fixed := &bytes.Buffer{}
	w := func(x uint32) { _ = binary.Write(fixed, binary.LittleEndian, x) }
	w(0xFEEF04BD) // dwSignature
	w(0x00010000) // dwStrucVersion
	w(uint32(nums[0])<<16 | uint32(nums[1]))
	w(uint32(nums[2])<<16 | uint32(nums[3]))
	w(uint32(nums[0])<<16 | uint32(nums[1]))
	w(uint32(nums[2])<<16 | uint32(nums[3]))
	w(0x3f)    // dwFileFlagsMask
	w(0)       // dwFileFlags
	w(0x40004) // dwFileOS: VOS_NT_WINDOWS32
	w(1)       // dwFileType: VFT_APP
	w(0)       // dwFileSubtype
	w(0)       // dwFileDateMS
	w(0)       // dwFileDateLS

	// El bloque de cadenas va bajo un idioma + página de códigos: 0409 (inglés
	// de EE. UU.) con 04B0 (Unicode) es la combinación que todas las
	// herramientas saben leer, y el texto en sí puede estar en cualquier idioma.
	strTable := &verNode{key: "040904B0", text: true}
	for _, kv := range [][2]string{
		{"CompanyName", v.company},
		{"FileDescription", v.description},
		{"FileVersion", v.version},
		{"InternalName", v.internal},
		{"LegalCopyright", v.copyright},
		{"ProductName", v.product},
		{"ProductVersion", v.version},
	} {
		if kv[1] == "" {
			continue
		}
		strTable.children = append(strTable.children, &verNode{key: kv[0], text: true, value: utf16z(kv[1])})
	}

	translation := &bytes.Buffer{}
	_ = binary.Write(translation, binary.LittleEndian, uint16(0x0409))
	_ = binary.Write(translation, binary.LittleEndian, uint16(0x04B0))

	root := &verNode{
		key:   "VS_VERSION_INFO",
		value: fixed.Bytes(),
		children: []*verNode{
			{key: "StringFileInfo", text: true, children: []*verNode{strTable}},
			{key: "VarFileInfo", text: true, children: []*verNode{
				{key: "Translation", value: translation.Bytes()},
			}},
		},
	}
	return resource{typ: rtVersion, id: 1, data: root.bytes()}, nil
}

// verNode es un nodo del árbol de VS_VERSIONINFO: todos tienen la misma forma
// (longitud, longitud del valor, tipo, clave, valor, hijos) y se alinean a 4.
type verNode struct {
	key      string
	value    []byte
	text     bool // wType: 1 = texto, 0 = binario
	children []*verNode
}

func (n *verNode) bytes() []byte {
	buf := &bytes.Buffer{}
	valueLen := len(n.value)
	if n.text {
		valueLen /= 2 // en las cadenas se cuenta en WCHAR, con el terminador
	}
	wType := uint16(0)
	if n.text {
		wType = 1
	}
	_ = binary.Write(buf, binary.LittleEndian, uint16(0)) // wLength, se rellena al final
	_ = binary.Write(buf, binary.LittleEndian, uint16(valueLen))
	_ = binary.Write(buf, binary.LittleEndian, wType)
	buf.Write(utf16z(n.key))
	pad4(buf)
	buf.Write(n.value)
	for _, c := range n.children {
		pad4(buf)
		buf.Write(c.bytes())
	}
	pad4(buf)
	b := buf.Bytes()
	binary.LittleEndian.PutUint16(b, uint16(len(b)))
	return b
}

func pad4(buf *bytes.Buffer) {
	for buf.Len()%4 != 0 {
		buf.WriteByte(0)
	}
}

// utf16z codifica en UTF-16 little endian con terminador, que es como guarda
// Windows las cadenas de los recursos.
func utf16z(s string) []byte {
	units := append(utf16.Encode([]rune(s)), 0)
	out := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[2*i:], u)
	}
	return out
}

// parseVersion pasa "1.2.3" o "1.2.3-rc.1" a los cuatro números de 16 bits de
// VS_FIXEDFILEINFO. El sufijo de preversión no cabe ahí: queda en las cadenas.
func parseVersion(v string) ([4]uint16, error) {
	var out [4]uint16
	base, _, _ := strings.Cut(v, "-")
	parts := strings.Split(base, ".")
	if len(parts) < 3 || len(parts) > 4 {
		return out, fmt.Errorf("versión %q: se esperaba X.Y.Z[-pre]", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 0xffff {
			return out, fmt.Errorf("versión %q: componente %q inválido", v, p)
		}
		out[i] = uint16(n)
	}
	return out, nil
}
