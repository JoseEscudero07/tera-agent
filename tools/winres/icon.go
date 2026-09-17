package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// iconEntry es una imagen del .ico: la cabecera que describe el tamaño y el
// bloque de píxeles (BMP o PNG) tal cual, que es lo que se guarda como RT_ICON.
type iconEntry struct {
	width, height byte
	colors, _pad  byte
	planes, bits  uint16
	data          []byte
}

// iconResources convierte un .ico en los recursos que espera Windows: una
// entrada RT_ICON por imagen, numeradas desde 1, y un RT_GROUP_ICON que las
// enumera. El Explorador, la barra de tareas y el cuadro de UAC eligen ahí el
// tamaño que necesitan.
func iconResources(ico []byte, groupID uint16) ([]resource, error) {
	entries, err := parseICO(ico)
	if err != nil {
		return nil, err
	}

	res := make([]resource, 0, len(entries)+1)
	group := &bytes.Buffer{}
	binary.Write(group, binary.LittleEndian, uint16(0)) // Reserved
	binary.Write(group, binary.LittleEndian, uint16(1)) // Type: icono
	binary.Write(group, binary.LittleEndian, uint16(len(entries)))
	for i, e := range entries {
		id := uint16(i + 1)
		group.WriteByte(e.width)
		group.WriteByte(e.height)
		group.WriteByte(e.colors)
		group.WriteByte(0)
		binary.Write(group, binary.LittleEndian, e.planes)
		binary.Write(group, binary.LittleEndian, e.bits)
		binary.Write(group, binary.LittleEndian, uint32(len(e.data)))
		// A diferencia del fichero .ico, el grupo referencia cada imagen por su
		// identificador de recurso, no por un offset dentro del fichero.
		binary.Write(group, binary.LittleEndian, id)
		res = append(res, resource{typ: rtIcon, id: id, data: e.data})
	}
	return append(res, resource{typ: rtGroupIcon, id: groupID, data: group.Bytes()}), nil
}

// parseICO lee la cabecera ICONDIR y sus entradas. Acepta tanto imágenes BMP
// como PNG (los .ico modernos guardan así los tamaños grandes).
func parseICO(b []byte) ([]iconEntry, error) {
	if len(b) < 6 {
		return nil, fmt.Errorf("icono: fichero truncado")
	}
	if binary.LittleEndian.Uint16(b[0:]) != 0 || binary.LittleEndian.Uint16(b[2:]) != 1 {
		return nil, fmt.Errorf("icono: no es un .ico")
	}
	n := int(binary.LittleEndian.Uint16(b[4:]))
	if n == 0 {
		return nil, fmt.Errorf("icono: sin imágenes")
	}
	if len(b) < 6+16*n {
		return nil, fmt.Errorf("icono: directorio truncado (%d imágenes)", n)
	}

	out := make([]iconEntry, 0, n)
	for i := 0; i < n; i++ {
		d := b[6+16*i:]
		size := int(binary.LittleEndian.Uint32(d[8:]))
		off := int(binary.LittleEndian.Uint32(d[12:]))
		if size <= 0 || off < 0 || off+size > len(b) {
			return nil, fmt.Errorf("icono: la imagen %d se sale del fichero", i+1)
		}
		out = append(out, iconEntry{
			width:  d[0],
			height: d[1],
			colors: d[2],
			planes: binary.LittleEndian.Uint16(d[4:]),
			bits:   binary.LittleEndian.Uint16(d[6:]),
			data:   b[off : off+size],
		})
	}
	return out, nil
}
