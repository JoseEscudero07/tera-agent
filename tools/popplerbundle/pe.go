package main

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"strings"
)

// Índices del directorio de datos del PE que interesan aquí.
const (
	dirImport      = 1
	dirResource    = 2
	dirDelayImport = 13
)

// importedDLLs devuelve los nombres de DLL que importa un PE, tanto por la tabla
// de imports normal como por la de carga diferida.
//
// Se leen las tablas a mano en vez de usar pe.File.ImportedSymbols porque este
// último descarta los imports por ordinal (una DLL importada solo por ordinal
// desaparecería del cierre) e ignora la carga diferida: una DLL diferida que no
// se empaquete no falla al arrancar, sino al imprimir el primer documento que la
// necesite.
func importedDLLs(f *pe.File) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		if k := strings.ToLower(name); name != "" && !seen[k] {
			seen[k] = true
			out = append(out, name)
		}
	}

	// IMAGE_IMPORT_DESCRIPTOR: 20 bytes, Name RVA en el offset 12; termina en un
	// descriptor a cero.
	if rva, _ := dataDirectory(f, dirImport); rva != 0 {
		for off := rva; ; off += 20 {
			d, err := readRVA(f, off, 20)
			if err != nil {
				return nil, fmt.Errorf("tabla de imports: %w", err)
			}
			nameRVA := binary.LittleEndian.Uint32(d[12:])
			if nameRVA == 0 {
				break
			}
			name, err := cString(f, nameRVA)
			if err != nil {
				return nil, err
			}
			add(name)
		}
	}

	// ImgDelayDescr: 32 bytes, DllNameRVA en el offset 4; termina en uno a cero.
	if rva, _ := dataDirectory(f, dirDelayImport); rva != 0 {
		for off := rva; ; off += 32 {
			d, err := readRVA(f, off, 32)
			if err != nil {
				return nil, fmt.Errorf("tabla de carga diferida: %w", err)
			}
			nameRVA := binary.LittleEndian.Uint32(d[4:])
			if nameRVA == 0 {
				break
			}
			name, err := cString(f, nameRVA)
			if err != nil {
				return nil, err
			}
			add(name)
		}
	}
	return out, nil
}

// dataDirectory devuelve RVA y tamaño de una entrada del directorio de datos.
func dataDirectory(f *pe.File, idx int) (rva, size uint32) {
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		if idx < int(oh.NumberOfRvaAndSizes) {
			return oh.DataDirectory[idx].VirtualAddress, oh.DataDirectory[idx].Size
		}
	case *pe.OptionalHeader32:
		if idx < int(oh.NumberOfRvaAndSizes) {
			return oh.DataDirectory[idx].VirtualAddress, oh.DataDirectory[idx].Size
		}
	}
	return 0, 0
}

// section devuelve la sección que contiene rva.
func section(f *pe.File, rva uint32) (*pe.Section, error) {
	for _, s := range f.Sections {
		size := s.VirtualSize
		if s.Size > size {
			size = s.Size
		}
		if rva >= s.VirtualAddress && rva < s.VirtualAddress+size {
			return s, nil
		}
	}
	return nil, fmt.Errorf("RVA 0x%x fuera de toda sección", rva)
}

// fileOffset traduce un RVA a posición en el fichero.
func fileOffset(f *pe.File, rva uint32) (int64, error) {
	s, err := section(f, rva)
	if err != nil {
		return 0, err
	}
	return int64(s.Offset) + int64(rva-s.VirtualAddress), nil
}

func readRVA(f *pe.File, rva uint32, n int) ([]byte, error) {
	s, err := section(f, rva)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	if _, err := s.ReadAt(buf, int64(rva-s.VirtualAddress)); err != nil {
		return nil, fmt.Errorf("leer RVA 0x%x: %w", rva, err)
	}
	return buf, nil
}

func cString(f *pe.File, rva uint32) (string, error) {
	var b strings.Builder
	for i := uint32(0); i < 512; i++ {
		c, err := readRVA(f, rva+i, 1)
		if err != nil {
			return "", err
		}
		if c[0] == 0 {
			return b.String(), nil
		}
		b.WriteByte(c[0])
	}
	return "", fmt.Errorf("cadena sin terminar en RVA 0x%x", rva)
}
