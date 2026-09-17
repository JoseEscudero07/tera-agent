package main

import (
	"bytes"
	"debug/pe"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Prueba de integración contra un bundle real de poppler para Windows. No hay
// PE de Windows en el repo, así que solo corre con POPPLER_BIN apuntando a
// <bundle>\Library\bin:
//
//	POPPLER_BIN=/ruta/poppler-26.09.0/Library/bin go test ./tools/popplerbundle/
func popplerBin(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("POPPLER_BIN")
	if dir == "" {
		t.Skip("POPPLER_BIN no definido")
	}
	return dir
}

func TestBundleAgainstRealPoppler(t *testing.T) {
	src := popplerBin(t)
	dst := t.TempDir()
	var out bytes.Buffer
	if err := run(src, dst, &out); err != nil {
		t.Fatal(err)
	}

	// Todo lo que importa cada fichero copiado está en el bundle o es del sistema.
	copied, _ := os.ReadDir(dst)
	have := map[string]bool{}
	for _, e := range copied {
		have[strings.ToLower(e.Name())] = true
	}
	srcHave := map[string]bool{}
	all, _ := os.ReadDir(src)
	for _, e := range all {
		srcHave[strings.ToLower(e.Name())] = true
	}
	for _, e := range copied {
		f, err := pe.Open(filepath.Join(dst, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		dlls, err := importedDLLs(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range dlls {
			k := strings.ToLower(d)
			if srcHave[k] && !have[k] && !isOSProvided(d) {
				t.Errorf("%s importa %s, que está en el bundle pero no se copió", e.Name(), d)
			}
		}
	}

	// El manifiesto quedó aplicado y la herramienta es idempotente.
	cairo := filepath.Join(dst, "pdftocairo.exe")
	data, _ := os.ReadFile(cairo)
	_, off, size, err := findManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data[off : off+int64(size)]); got != utf8Manifest {
		t.Errorf("manifiesto = %q", got)
	}
	if err := setUTF8Manifest(cairo); err != nil {
		t.Errorf("segunda pasada: %v", err)
	}
	// Sigue siendo un PE válido.
	if f, err := pe.Open(cairo); err != nil {
		t.Errorf("pdftocairo.exe ya no es un PE válido: %v", err)
	} else {
		f.Close()
	}
}
