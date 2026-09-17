package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// graph simula el bundle: fichero -> DLLs que importa.
type graph map[string][]string

func (g graph) available() map[string]string {
	m := map[string]string{}
	for f := range g {
		m[strings.ToLower(f)] = f
	}
	return m
}

func (g graph) importsOf(file string) ([]string, error) { return g[file], nil }

func TestClosureTakesOnlyWhatTheToolsLoad(t *testing.T) {
	g := graph{
		"pdftoppm.exe":     {"poppler.dll", "KERNEL32.dll", "api-ms-win-crt-runtime-l1-1-0.dll"},
		"pdftocairo.exe":   {"POPPLER.DLL", "cairo.dll"},
		"poppler.dll":      {"libcurl.dll", "msvcp140.dll"},
		"libcurl.dll":      {"psl-5.dll"},
		"psl-5.dll":        {"icuuc78.dll"},
		"icuuc78.dll":      {"icudt78.dll", "libcurl.dll"}, // ciclo
		"icudt78.dll":      nil,
		"cairo.dll":        {"pixman-1-0.dll"},
		"pixman-1-0.dll":   nil,
		"msvcp140.dll":     {"vcruntime140.dll"},
		"vcruntime140.dll": nil,
		// Presentes en el bundle pero que no carga ninguna de las dos herramientas.
		"harfbuzz.dll":   nil,
		"glib-2.0-0.dll": nil,
		"pdftotext.exe":  {"poppler.dll"},
		// Vienen en el bundle, pero Windows 10+ usa siempre la copia del sistema.
		"api-ms-win-crt-runtime-l1-1-0.dll": {"ucrtbase.dll"},
		"ucrtbase.dll":                      nil,
	}
	got, err := closure(tools, g.available(), g.importsOf)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"cairo.dll", "icudt78.dll", "icuuc78.dll", "libcurl.dll", "msvcp140.dll",
		"pdftocairo.exe", "pdftoppm.exe", "pixman-1-0.dll", "poppler.dll", "psl-5.dll",
		"vcruntime140.dll",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("closure =\n%v\nwant\n%v", got, want)
	}
}

func TestClosurePropagatesReadErrors(t *testing.T) {
	boom := errors.New("PE corrupto")
	_, err := closure(tools, map[string]string{"pdftoppm.exe": "pdftoppm.exe"}, func(string) ([]string, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestIsOSProvided(t *testing.T) {
	for dll, want := range map[string]bool{
		"ucrtbase.dll": true, "UCRTBASE.DLL": true,
		"api-ms-win-crt-heap-l1-1-0.dll": true, "ext-ms-win-foo.dll": true,
		"vcruntime140.dll": false, "poppler.dll": false,
	} {
		if got := isOSProvided(dll); got != want {
			t.Errorf("isOSProvided(%q) = %v, want %v", dll, got, want)
		}
	}
}
