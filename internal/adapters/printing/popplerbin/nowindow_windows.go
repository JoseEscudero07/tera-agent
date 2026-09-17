//go:build windows

package popplerbin

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideConsole evita que Windows abra una ventana de consola negra al lanzar
// la herramienta de Poppler (pdftoppm, pdftocairo).
//
// Por qué ocurre: pdftoppm.exe y pdftocairo.exe son aplicaciones de consola. Cuando el proceso
// padre YA tiene consola (el binario `tera-agent.exe`) el hijo la hereda y no se
// ve nada raro. Pero el binario de la bandeja se compila con -H=windowsgui y no
// tiene consola, así que Windows le crea una NUEVA al hijo — y aparece un cuadro
// negro parpadeando en pantalla cada vez que se imprime un PDF.
//
// CREATE_NO_WINDOW ejecuta el proceso de consola sin ventana. No afecta a la
// captura de stdout/stderr: las tuberías siguen funcionando igual, así que los
// errores de poppler se siguen recogiendo.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
