//go:build !windows

package popplerbin

import "os/exec"

// hideConsole no hace nada fuera de Windows: en Linux/macOS lanzar un proceso no
// abre ninguna ventana.
func hideConsole(_ *exec.Cmd) {}
