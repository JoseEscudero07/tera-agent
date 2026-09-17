package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

// utf8Manifest es el manifiesto que se pone a pdftocairo.exe. Declara la página
// de códigos activa UTF-8 para el proceso (Windows 10 1903+).
//
// Por qué: pdftocairo reconstruye sus argumentos en UTF-8 y se los pasa a las
// funciones ANSI de Windows (DocumentPropertiesA, CreateDCA, StartDocA). Con la
// página de códigos del sistema (1252 en Windows en español), una impresora
// llamada "LÁSER Facturación Ñ" llega como "LÃSER FacturaciÃ³n Ã‘" y falla con
// "Printer not found". Con este manifiesto esas mismas funciones interpretan
// UTF-8 y la encuentran. Validado en Windows 11 limpio.
//
// No incluye el bloque trustInfo (requestedExecutionLevel=asInvoker) del
// manifiesto original porque con él no cabe en el hueco del recurso (381 bytes
// en poppler 26.x) y reescribir la sección de recursos es otra liga de riesgo.
// No cambia nada: asInvoker es el comportamiento por defecto, y la detección de
// instaladores de UAC —lo único que ese bloque evitaría— solo se aplica a
// ejecutables de 32 bits con nombres tipo setup/install/update.
const utf8Manifest = `<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">` +
	`<application xmlns="urn:schemas-microsoft-com:asm.v3"><windowsSettings>` +
	`<activeCodePage xmlns="http://schemas.microsoft.com/SMI/2019/WindowsSettings">UTF-8</activeCodePage>` +
	`</windowsSettings></application></assembly>`

const rtManifest = 24 // RT_MANIFEST

// setUTF8Manifest sustituye el manifiesto embebido de path por utf8Manifest.
//
// Solo se reescriben los bytes del recurso RT_MANIFEST y su tamaño en la entrada
// del directorio de recursos: ni código ni secciones cambian de sitio. Por eso
// exige que el manifiesto nuevo quepa en el hueco del original (se rellena con
// ceros) y falla en vez de reorganizar el PE si no cabe.
func setUTF8Manifest(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	entryOff, dataOff, size, err := findManifest(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	current := data[dataOff : dataOff+int64(size)]
	if bytes.Contains(current, []byte("<activeCodePage")) {
		if bytes.Contains(current, []byte(">UTF-8<")) {
			return nil // ya aplicado: la herramienta es idempotente
		}
		return fmt.Errorf("%s: el manifiesto ya declara otra activeCodePage; revisar a mano", path)
	}
	if !bytes.Contains(current, []byte("urn:schemas-microsoft-com:asm.v1")) {
		return fmt.Errorf("%s: el recurso RT_MANIFEST no parece un manifiesto", path)
	}
	if len(utf8Manifest) > int(size) {
		return fmt.Errorf("%s: el manifiesto UTF-8 (%d bytes) no cabe en el original (%d bytes)", path, len(utf8Manifest), size)
	}

	copy(data[dataOff:], utf8Manifest)
	for i := dataOff + int64(len(utf8Manifest)); i < dataOff+int64(size); i++ {
		data[i] = 0
	}
	binary.LittleEndian.PutUint32(data[entryOff+4:], uint32(len(utf8Manifest)))
	return os.WriteFile(path, data, 0o644)
}

// findManifest localiza el primer recurso RT_MANIFEST (tipo 24 → primer nombre →
// primer idioma). Devuelve la posición en el fichero de su
// IMAGE_RESOURCE_DATA_ENTRY, la de sus datos y su tamaño.
func findManifest(data []byte) (entryOff, dataOff int64, size uint32, err error) {
	f, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return 0, 0, 0, err
	}
	rsrcRVA, _ := dataDirectory(f, dirResource)
	if rsrcRVA == 0 {
		return 0, 0, 0, errors.New("sin sección de recursos")
	}
	base, err := fileOffset(f, rsrcRVA)
	if err != nil {
		return 0, 0, 0, err
	}

	u32 := func(off int64) (uint32, error) {
		if off < 0 || off+4 > int64(len(data)) {
			return 0, fmt.Errorf("recurso truncado en 0x%x", off)
		}
		return binary.LittleEndian.Uint32(data[off:]), nil
	}
	u16 := func(off int64) (uint16, error) {
		if off < 0 || off+2 > int64(len(data)) {
			return 0, fmt.Errorf("recurso truncado en 0x%x", off)
		}
		return binary.LittleEndian.Uint16(data[off:]), nil
	}
	// child devuelve el OffsetToData del primer hijo de un
	// IMAGE_RESOURCE_DIRECTORY cuyo id cumpla match. Las entradas (8 bytes)
	// empiezan tras la cabecera de 16.
	child := func(dir int64, match func(id uint32) bool) (uint32, error) {
		named, err := u16(dir + 12)
		if err != nil {
			return 0, err
		}
		ids, err := u16(dir + 14)
		if err != nil {
			return 0, err
		}
		for i := int64(0); i < int64(named)+int64(ids); i++ {
			e := dir + 16 + i*8
			id, err := u32(e)
			if err != nil {
				return 0, err
			}
			if match(id) {
				return u32(e + 4)
			}
		}
		return 0, errors.New("sin manifiesto embebido (RT_MANIFEST)")
	}
	const subdir = 0x80000000
	first := func(uint32) bool { return true }

	typ, err := child(base, func(id uint32) bool { return id == rtManifest })
	if err != nil || typ&subdir == 0 {
		return 0, 0, 0, errors.New("sin manifiesto embebido (RT_MANIFEST)")
	}
	name, err := child(base+int64(typ&^subdir), first)
	if err != nil || name&subdir == 0 {
		return 0, 0, 0, errors.New("RT_MANIFEST sin entrada de nombre")
	}
	lang, err := child(base+int64(name&^subdir), first)
	if err != nil || lang&subdir != 0 {
		return 0, 0, 0, errors.New("RT_MANIFEST sin entrada de idioma")
	}

	entryOff = base + int64(lang)
	dataRVA, err := u32(entryOff)
	if err != nil {
		return 0, 0, 0, err
	}
	if size, err = u32(entryOff + 4); err != nil {
		return 0, 0, 0, err
	}
	if dataOff, err = fileOffset(f, dataRVA); err != nil {
		return 0, 0, 0, err
	}
	if dataOff+int64(size) > int64(len(data)) {
		return 0, 0, 0, errors.New("datos del manifiesto fuera del fichero")
	}
	return entryOff, dataOff, size, nil
}
