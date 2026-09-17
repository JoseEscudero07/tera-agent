// Command popplerbundle prepara el poppler que empaqueta el instalador de
// Windows: copia pdftoppm.exe y pdftocairo.exe con SOLO las DLLs que cargan, y
// pone a pdftocairo.exe el manifiesto UTF-8 (ver manifest.go).
//
//	go run ./tools/popplerbundle -src <poppler>\Library\bin -dst installer\staging\poppler\bin
//
// Lo usan scripts/build-release.ps1 (Windows) y la compilación desde Linux, así
// que el contenido del bundle no depende de dónde se compile.
//
// Por qué calcular las DLLs en vez de copiar *.dll: los bundles de poppler para
// Windows traen todo lo que usa cualquier herramienta de poppler (ICU, harfbuzz,
// glib, Kerberos…). Desde 26.07 son 143 ficheros y más de 100 MB; pdftoppm y
// pdftocairo usan una fracción. Y una lista escrita a mano se rompe en silencio
// al subir de versión: aquí la saca el propio PE.
// Owner: DevOps Engineer.
package main

import (
	"debug/pe"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// tools son los ejecutables que usa el Agent (ver adapters/printing/popplerbin).
var tools = []string{"pdftoppm.exe", "pdftocairo.exe"}

func main() {
	src := flag.String("src", "", "directorio bin del bundle de poppler (Library\\bin)")
	dst := flag.String("dst", "", "directorio de destino (se crea si no existe)")
	flag.Parse()
	if *src == "" || *dst == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*src, *dst, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "popplerbundle:", err)
		os.Exit(1)
	}
}

func run(src, dst string, out io.Writer) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	available := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			available[strings.ToLower(e.Name())] = e.Name()
		}
	}
	for _, t := range tools {
		if _, ok := available[t]; !ok {
			return fmt.Errorf("no se encuentra %s en %s", t, src)
		}
	}

	files, err := closure(tools, available, func(file string) ([]string, error) {
		f, err := pe.Open(filepath.Join(src, file))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		defer f.Close()
		dlls, err := importedDLLs(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		return dlls, nil
	})
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	var total int64
	for _, name := range files {
		n, err := copyFile(filepath.Join(src, name), filepath.Join(dst, name))
		if err != nil {
			return err
		}
		total += n
	}
	if err := setUTF8Manifest(filepath.Join(dst, "pdftocairo.exe")); err != nil {
		return err
	}

	fmt.Fprintf(out, "poppler: %d ficheros (%.1f MB) de %d en el bundle; pdftocairo.exe con manifiesto UTF-8\n",
		len(files), float64(total)/(1<<20), len(available))
	for _, name := range files {
		fmt.Fprintln(out, "  ", name)
	}
	return nil
}

func copyFile(from, to string) (int64, error) {
	in, err := os.Open(from)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	outFile, err := os.Create(to)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(outFile, in)
	if cerr := outFile.Close(); err == nil {
		err = cerr
	}
	return n, err
}
