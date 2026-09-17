//go:build !windows

package poppler

import "os/exec"

// hideConsole no hace nada fuera de Windows: en Linux/macOS lanzar un proceso no
// abre ninguna ventana.
func hideConsole(_ *exec.Cmd) {}
