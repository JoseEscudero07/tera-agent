package poppler

import (
	"errors"
	"strings"
	"testing"
)

// El código de salida que clasificamos debe ser exactamente STATUS_DLL_NOT_FOUND
// (0xC0000135) interpretado como int32 con signo, que es como lo reporta
// os/exec en Windows.
func TestDllNotFoundExitValue(t *testing.T) {
	// 0xC0000135 no cabe en int32 como constante positiva: el valor con signo es
	// 0xC0000135 - 2^32.
	if want := 0xC0000135 - (1 << 32); dllNotFoundExit != want {
		t.Errorf("dllNotFoundExit = %d; se esperaba %d (0xC0000135 con signo)", dllNotFoundExit, want)
	}
}

// Regresión de soporte: el mensaje de DLL ausente tiene que nombrar el problema y
// la acción. El error crudo de Windows ("exit status 0xc0000135") no dice nada, y
// stderr viene vacío porque el proceso no llega a arrancar.
func TestDllNotFoundErrorIsActionable(t *testing.T) {
	const bin = `C:\Program Files\TeraAgent\poppler\bin\pdftoppm.exe`
	msg := dllNotFoundError(bin).Error()

	for _, want := range []string{bin, "0xC0000135", "msvcp140.dll", "vcruntime140.dll", "vcruntime140_1.dll", "Visual C++"} {
		if !strings.Contains(msg, want) {
			t.Errorf("el mensaje no menciona %q: %s", want, msg)
		}
	}
}

// Sin stderr no debe quedar un separador ": " colgando al final del mensaje.
func TestRunErrorWithoutStderr(t *testing.T) {
	err := runError("pdftoppm", errors.New("boom"), "")
	got := err.Error()
	if strings.HasSuffix(got, ": ") {
		t.Errorf("separador colgando al final: %q", got)
	}
	if !strings.Contains(got, "boom") {
		t.Errorf("el mensaje debe conservar la causa: %q", got)
	}
}

// Con stderr, hay que incluirlo: es donde poppler explica los PDF corruptos.
func TestRunErrorWithStderr(t *testing.T) {
	err := runError("pdftoppm", errors.New("exit status 1"), "Syntax Error: Couldn't read xref table")
	if !strings.Contains(err.Error(), "xref table") {
		t.Errorf("el mensaje debe incluir stderr: %q", err.Error())
	}
}

// Un error que no sea *exec.ExitError (p. ej. binario no encontrado) no debe
// clasificarse como problema de DLLs.
func TestRunErrorNonExitErrorIsNotMisclassified(t *testing.T) {
	err := runError("pdftoppm", errors.New("executable file not found in %PATH%"), "")
	if strings.Contains(err.Error(), "0xC0000135") {
		t.Errorf("clasificado erróneamente como DLL ausente: %q", err.Error())
	}
}
