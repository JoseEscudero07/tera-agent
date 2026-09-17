//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// fatalStartup reporta un fallo irrecuperable de arranque y termina el proceso.
//
// Existe porque el binario de la bandeja se compila con -H=windowsgui: no tiene
// consola, así que os.Stderr es un handle inválido y un os.Exit(1) tras escribir
// en stderr es COMPLETAMENTE invisible. El síntoma para el cliente era "se
// instala pero nunca abre, no sale nada" — imposible de diagnosticar en remoto.
//
// Deja el error en dos sitios y por ese orden de importancia:
//  1. Un fichero de arranque en la carpeta de datos, para el soporte.
//  2. Un cuadro de diálogo, para que la persona delante del equipo lo vea.
//
// Bajo el SCM no se muestra diálogo: un servicio corre en la sesión 0 y el cuadro
// quedaría invisible bloqueando el proceso hasta el timeout.
func fatalStartup(err error) {
	msg := fmt.Sprintf("tera-agent: startup failed: %v", err)

	// stderr por si SÍ hay consola (binario de consola lanzado a mano).
	fmt.Fprintln(os.Stderr, msg)
	writeStartupError(msg)

	if !isWindowsService() {
		showErrorBox(msg)
	}
	os.Exit(1)
}

// startupErrorFile es donde se deja el error cuando el log normal ni siquiera
// pudo inicializarse (típicamente porque la config no se pudo leer).
const startupErrorFile = "startup-error.log"

// writeStartupError anexa el error a la carpeta de datos. Best-effort: si tampoco
// se puede escribir ahí, se intenta el directorio temporal; si falla todo, se
// continúa (ya se mostrará el diálogo).
func writeStartupError(msg string) {
	line := time.Now().Format(time.RFC3339) + " " + msg + "\r\n"
	for _, dir := range []string{filepath.Join(os.Getenv("ProgramData"), "TeraAgent"), os.TempDir()} {
		if dir == "" {
			continue
		}
		f, err := os.OpenFile(filepath.Join(dir, startupErrorFile),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			continue
		}
		_, _ = f.WriteString(line)
		_ = f.Close()
		return
	}
}

// showErrorBox muestra un cuadro de error nativo. MB_SYSTEMMODAL lo pone por
// encima del resto: si esto salta al iniciar sesión, un diálogo detrás de otras
// ventanas no lo vería nadie.
func showErrorBox(msg string) {
	title, err := windows.UTF16PtrFromString("Tera Agent")
	if err != nil {
		return
	}
	body, err := windows.UTF16PtrFromString(
		msg + "\n\nEl agente no pudo arrancar. Revisa la configuración en:\n" +
			filepath.Join(os.Getenv("ProgramData"), "TeraAgent", "config.yaml"))
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, body, title, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SYSTEMMODAL)
}
