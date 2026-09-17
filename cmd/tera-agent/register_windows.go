//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"

	"github.com/teraerp/tera-agent/internal/adapters/apppath"
)

// requireElevatedForScope no bloquea: la barrera real es la ACL de la carpeta
// (solo SYSTEM y Administradores pueden escribir), así que si quien ejecuta no
// tiene permisos, el guardado fallará con un error claro (ver hintForWriteError).
//
// No se comprueba Token.IsElevated porque da falsos negativos: una sesión de red
// (WinRM, despliegue remoto) recibe un token de administrador SIN elevar aunque
// la cuenta sea administradora, y rechazaríamos un registro legítimo.
func requireElevatedForScope(apppath.Scope) error { return nil }

// hintForWriteError añade, en modo servicio, la pista de que hay que ejecutar
// como administrador cuando el guardado falla por falta de permisos.
func hintForWriteError(sc apppath.Scope, err error) error {
	if err == nil || sc != apppath.ScopeService {
		return err
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w\nla carpeta de datos solo la pueden modificar los administradores: "+
			"abre una consola como administrador (clic derecho → Ejecutar como administrador)", err)
	}
	return err
}

// readSecret lee una línea sin mostrarla en pantalla, para que el Token no quede
// en la consola ni en el historial. Si la entrada no es una consola (por ejemplo
// una tubería), lee normal: en ese caso quien invoca ya controla el canal.
func readSecret(promptMsg string) (string, error) {
	fmt.Print(promptMsg)
	h := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		// No es una consola: lectura normal.
		return readLine(), nil
	}
	// Quitar el eco mientras se teclea el Token.
	if err := windows.SetConsoleMode(h, mode&^windows.ENABLE_ECHO_INPUT); err != nil {
		return readLine(), nil
	}
	defer windows.SetConsoleMode(h, mode)
	line := readLine()
	fmt.Println()
	return line, nil
}

// restartServiceBestEffort reinicia el servicio para que tome la nueva
// configuración. controlService ya existe para Windows.
func restartServiceBestEffort() error {
	if err := controlService("stop", ""); err != nil {
		return err
	}
	return controlService("start", "")
}
