// Command winres genera el fichero .syso con los recursos de Windows (icono y
// datos de versión) que el enlazador de Go incrusta en el ejecutable.
//
// Owner: DevOps Engineer. Lo llaman scripts/build-release.sh y build-release.ps1
// justo antes de compilar, para que la versión de las propiedades sea la misma
// que se inyecta con -ldflags:
//
//	go run ./tools/winres -ico installer/tera-agent.ico -version 1.2.0 \
//	  -out cmd/tera-agent/rsrc_windows_amd64.syso
//
// El .syso es un artefacto de build (está en .gitignore): el enlazador recoge
// cualquier .syso que encuentre en el directorio del paquete, y el sufijo
// _windows_amd64 evita que se cuele en los binarios de Linux y macOS.
//
// Sin esto los .exe salen con el icono genérico de Windows y el cuadro de UAC
// muestra el nombre del fichero en vez del nombre del programa.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	ico := flag.String("ico", "", "icono .ico a incrustar (obligatorio)")
	out := flag.String("out", "", "fichero .syso a escribir (obligatorio)")
	version := flag.String("version", "", "versión X.Y.Z[-pre]; vacío = sin datos de versión")
	arch := flag.String("arch", "amd64", "arquitectura del objeto: amd64|386")
	company := flag.String("company", "Grupo Tera", "CompanyName")
	product := flag.String("product", "Tera Agent", "ProductName")
	description := flag.String("description", "Tera Agent", "FileDescription: el nombre que enseña el cuadro de UAC")
	copyright := flag.String("copyright", "", "LegalCopyright")
	internal := flag.String("internal", "tera-agent", "InternalName")
	flag.Parse()

	if *ico == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*ico, *out, *arch, versionInfo{
		version:     *version,
		company:     *company,
		product:     *product,
		description: *description,
		copyright:   *copyright,
		internal:    *internal,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "winres:", err)
		os.Exit(1)
	}
}

func run(icoPath, outPath, arch string, v versionInfo) error {
	ico, err := os.ReadFile(icoPath)
	if err != nil {
		return err
	}
	res, err := iconResources(ico, 1)
	if err != nil {
		return err
	}
	icons := len(res) - 1

	if v.version != "" {
		ver, err := versionResource(v)
		if err != nil {
			return err
		}
		res = append(res, ver)
	}

	obj, err := writeCOFF(res, arch)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, obj, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: icono de %d tamaños", outPath, icons)
	if v.version != "" {
		fmt.Printf(" y versión %s", v.version)
	}
	fmt.Printf(" (%d KB)\n", len(obj)/1024)
	return nil
}
