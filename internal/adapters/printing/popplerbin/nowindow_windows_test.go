//go:build windows

package popplerbin

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

// Regresión: sin CREATE_NO_WINDOW, cada rasterizado de un PDF abría una ventana
// de consola negra en pantalla, porque el binario de la bandeja (-H=windowsgui)
// no tiene consola propia y Windows le crea una nueva al proceso hijo.
//
// Ojo con cómo se verifica esto a mano: el flag NO impide que se asigne un
// conhost.exe al hijo, solo que la ventana sea visible. Contar procesos conhost
// da un resultado engañoso; hay que mirar ventanas visibles.
func TestHideConsoleSetsCreateNoWindow(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	hideConsole(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr = nil; hideConsole no configuró nada")
	}
	if got := cmd.SysProcAttr.CreationFlags; got&windows.CREATE_NO_WINDOW == 0 {
		t.Errorf("CreationFlags = 0x%X; falta CREATE_NO_WINDOW (0x%X)", got, windows.CREATE_NO_WINDOW)
	}
}
