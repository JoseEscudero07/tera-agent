package main

import (
	"sort"
	"strings"
)

// isOSProvided reporta las DLLs que no se empaquetan aunque vengan en el bundle
// de poppler: el Universal CRT y los API sets forman parte de Windows 10 y
// posteriores (el mínimo que exige Go), y el cargador usa SIEMPRE la copia del
// sistema aunque haya una junto al .exe. Empaquetarlas solo engorda el
// instalador.
func isOSProvided(dll string) bool {
	d := strings.ToLower(dll)
	return d == "ucrtbase.dll" || strings.HasPrefix(d, "api-ms-win-") || strings.HasPrefix(d, "ext-ms-")
}

// closure calcula qué ficheros del directorio del bundle hacen falta para
// ejecutar roots: los propios roots más todas las DLLs que importan, directa o
// indirectamente, y que están en ese directorio. Las que no están se consideran
// del sistema (kernel32, user32, winspool…).
//
// available mapea nombre en minúsculas → nombre real del fichero; importsOf
// devuelve las DLLs que importa un fichero del bundle. Devuelve los nombres
// reales, ordenados.
func closure(roots []string, available map[string]string, importsOf func(file string) ([]string, error)) ([]string, error) {
	need := map[string]string{}
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		key := strings.ToLower(name)
		file, ok := available[key]
		if !ok || need[key] != "" || isOSProvided(name) {
			continue
		}
		need[key] = file
		deps, err := importsOf(file)
		if err != nil {
			return nil, err
		}
		queue = append(queue, deps...)
	}
	out := make([]string, 0, len(need))
	for _, f := range need {
		out = append(out, f)
	}
	sort.Strings(out)
	return out, nil
}
