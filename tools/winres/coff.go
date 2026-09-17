package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// Tipos de recurso de Windows que necesita el agente.
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
)

// langEnUS: los recursos se guardan bajo un idioma concreto, pero la búsqueda de
// Windows cae en el primero disponible si no encuentra el del sistema, así que
// con uno basta para que el icono y las propiedades se vean en cualquier idioma.
const langEnUS = 0x0409

// resource es una hoja del árbol de recursos: tipo → identificador → idioma.
type resource struct {
	typ  uint16
	id   uint16
	data []byte
}

// Máquinas y relocalizaciones de COFF (winnt.h).
const (
	machineAMD64 = 0x8664
	machine386   = 0x014c

	relAMD64Addr32NB = 0x0003 // IMAGE_REL_AMD64_ADDR32NB
	rel386Dir32NB    = 0x0007 // IMAGE_REL_I386_DIR32NB
)

// writeCOFF serializa los recursos como un objeto COFF con una única sección
// .rsrc, que es lo que el enlazador de Go incrusta cuando encuentra un .syso en
// el directorio del paquete.
//
// Las direcciones de los datos dentro de la sección son offsets relativos a
// ella; el enlazador los convierte en RVA gracias a una relocalización por cada
// IMAGE_RESOURCE_DATA_ENTRY. Sin esas relocalizaciones Windows leería el icono
// en una dirección equivocada.
func writeCOFF(res []resource, arch string) ([]byte, error) {
	var machine, relType uint16
	switch arch {
	case "amd64":
		machine, relType = machineAMD64, relAMD64Addr32NB
	case "386":
		machine, relType = machine386, rel386Dir32NB
	default:
		return nil, fmt.Errorf("arquitectura no soportada: %q (amd64|386)", arch)
	}

	section, relocs, err := buildRsrc(res)
	if err != nil {
		return nil, err
	}
	if len(relocs) > 0xffff {
		return nil, fmt.Errorf("demasiados recursos: %d relocalizaciones", len(relocs))
	}

	const headers = 20 + 40 // cabecera de fichero + una cabecera de sección
	ptrRaw := uint32(headers)
	ptrReloc := ptrRaw + uint32(len(section))
	ptrSyms := ptrReloc + uint32(len(relocs))*10

	buf := &bytes.Buffer{}
	w := func(v any) { _ = binary.Write(buf, binary.LittleEndian, v) }

	// IMAGE_FILE_HEADER
	w(machine)
	w(uint16(1)) // NumberOfSections
	w(uint32(0)) // TimeDateStamp: 0 para que el build sea reproducible
	w(ptrSyms)   // PointerToSymbolTable
	w(uint32(1)) // NumberOfSymbols: solo el símbolo de la sección
	w(uint16(0)) // SizeOfOptionalHeader: los objetos no la llevan
	w(uint16(0)) // Characteristics

	// IMAGE_SECTION_HEADER
	buf.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	w(uint32(0))            // VirtualSize
	w(uint32(0))            // VirtualAddress
	w(uint32(len(section))) // SizeOfRawData
	w(ptrRaw)               // PointerToRawData
	w(ptrReloc)             // PointerToRelocations
	w(uint32(0))            // PointerToLinenumbers
	w(uint16(len(relocs)))  // NumberOfRelocations
	w(uint16(0))            // NumberOfLinenumbers
	w(uint32(0x40000040))   // IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ

	buf.Write(section)

	for _, at := range relocs {
		w(at)        // VirtualAddress: el campo OffsetToData de una entrada de datos
		w(uint32(0)) // SymbolTableIndex: el símbolo .rsrc
		w(relType)
	}

	// Tabla de símbolos: un único símbolo estático para la sección.
	buf.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	w(uint32(0))     // Value
	w(uint16(1))     // SectionNumber (1-based)
	w(uint16(0))     // Type
	buf.WriteByte(3) // StorageClass: IMAGE_SYM_CLASS_STATIC
	buf.WriteByte(0) // NumberOfAuxSymbols
	w(uint32(4))     // tabla de cadenas vacía: solo su propio tamaño

	return buf.Bytes(), nil
}

// buildRsrc arma el árbol de recursos de tres niveles (tipo → id → idioma) y
// devuelve la sección ya serializada junto con los offsets que hay que relocar.
func buildRsrc(res []resource) (section []byte, relocs []uint32, err error) {
	if len(res) == 0 {
		return nil, nil, fmt.Errorf("no hay recursos que escribir")
	}
	// El árbol exige las entradas ordenadas por identificador en cada nivel:
	// Windows las busca con una búsqueda binaria.
	sorted := append([]resource(nil), res...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].typ != sorted[j].typ {
			return sorted[i].typ < sorted[j].typ
		}
		return sorted[i].id < sorted[j].id
	})

	type group struct {
		typ  uint16
		list []resource
	}
	var groups []group
	for _, r := range sorted {
		if n := len(groups); n > 0 && groups[n-1].typ == r.typ {
			groups[n-1].list = append(groups[n-1].list, r)
			continue
		}
		groups = append(groups, group{typ: r.typ, list: []resource{r}})
	}

	// Primera pasada: tamaños y offsets. Cada recurso tiene su propio directorio
	// de idioma (un solo idioma), de ahí "16 + 8" por hoja.
	dirs := 16 + 8*len(groups)
	for _, g := range groups {
		dirs += 16 + 8*len(g.list)
		dirs += len(g.list) * (16 + 8)
	}
	entriesAt := dirs
	dataAt := align(entriesAt+16*len(sorted), 8)

	total := dataAt
	for _, r := range sorted {
		total = align(total+len(r.data), 8)
	}

	out := make([]byte, total)
	put16 := func(off int, v uint16) { binary.LittleEndian.PutUint16(out[off:], v) }
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(out[off:], v) }

	// Segunda pasada: se escriben los directorios en el mismo orden en que se
	// calcularon los tamaños.
	typeDirAt := 16 + 8*len(groups) // primer directorio de tipo
	put16(12, 0)                    // NumberOfNamedEntries
	put16(14, uint16(len(groups)))  // NumberOfIdEntries
	cursor := typeDirAt
	nameDirs := make([]int, 0, len(sorted))
	for i, g := range groups {
		entry := 16 + 8*i
		put32(entry, uint32(g.typ))
		put32(entry+4, uint32(cursor)|0x80000000) // apunta a un subdirectorio
		put16(cursor+12, 0)
		put16(cursor+14, uint16(len(g.list)))
		cursor += 16 + 8*len(g.list)
	}
	// Directorios de idioma, uno por recurso, detrás de todos los de tipo.
	langAt := cursor
	cursor = typeDirAt
	for _, g := range groups {
		for i, r := range g.list {
			entry := cursor + 16 + 8*i
			put32(entry, uint32(r.id))
			put32(entry+4, uint32(langAt)|0x80000000)
			nameDirs = append(nameDirs, langAt)
			langAt += 16 + 8
		}
		cursor += 16 + 8*len(g.list)
	}

	dataOff := dataAt
	for i, r := range sorted {
		lang := nameDirs[i]
		put16(lang+12, 0)
		put16(lang+14, 1)
		put32(lang+16, uint32(langEnUS))
		entry := entriesAt + 16*i
		put32(lang+20, uint32(entry)) // hoja: sin el bit alto

		put32(entry, uint32(dataOff)) // lo reloca el enlazador a un RVA
		put32(entry+4, uint32(len(r.data)))
		put32(entry+8, 0) // CodePage
		put32(entry+12, 0)
		relocs = append(relocs, uint32(entry))

		copy(out[dataOff:], r.data)
		dataOff = align(dataOff+len(r.data), 8)
	}
	return out, relocs, nil
}

func align(n, to int) int {
	if r := n % to; r != 0 {
		return n + to - r
	}
	return n
}
