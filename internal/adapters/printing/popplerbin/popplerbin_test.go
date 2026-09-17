package popplerbin

import (
	"errors"
	"os"
	"path/filepath"
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
	const bin = `C:\Program Files\TeraAgent\poppler\bin\pdftocairo.exe`
	msg := dllNotFoundError(bin).Error()

	for _, want := range []string{bin, "0xC0000135", "msvcp140.dll", "vcruntime140.dll", "vcruntime140_1.dll", "Visual C++", "junto a pdftocairo.exe"} {
		if !strings.Contains(msg, want) {
			t.Errorf("el mensaje no menciona %q: %s", want, msg)
		}
	}
}

// Sin stderr no debe quedar un separador ": " colgando al final del mensaje.
func TestRunErrorWithoutStderr(t *testing.T) {
	err := RunError("pdftoppm", errors.New("boom"), "")
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
	err := RunError("pdftoppm", errors.New("exit status 1"), "Syntax Error: Couldn't read xref table")
	if !strings.Contains(err.Error(), "xref table") {
		t.Errorf("el mensaje debe incluir stderr: %q", err.Error())
	}
}

// La causa original tiene que seguir siendo inspeccionable con errors.Is/As.
func TestRunErrorWrapsCause(t *testing.T) {
	cause := errors.New("exit status 99")
	if err := RunError("pdftocairo", cause, "Error: StartDoc failed"); !errors.Is(err, cause) {
		t.Errorf("RunError no envuelve la causa: %v", err)
	}
}

// Un error que no sea *exec.ExitError (p. ej. binario no encontrado) no debe
// clasificarse como problema de DLLs.
func TestRunErrorNonExitErrorIsNotMisclassified(t *testing.T) {
	err := RunError("pdftoppm", errors.New("executable file not found in %PATH%"), "")
	if strings.Contains(err.Error(), "0xC0000135") {
		t.Errorf("clasificado erróneamente como DLL ausente: %q", err.Error())
	}
}

// Salida real de pdftocairo en Windows 11 limpio al imprimir en una impresora que
// no existe: veinte avisos de fuentes delante del único dato útil.
func TestCleanStderrDropsFontNoise(t *testing.T) {
	stderr := "Syntax Error: No display font for 'Symbol'\r\n" +
		"Syntax Error: No display font for 'ArialNarrow'\r\n" +
		"Syntax Error: No display font for 'BookAntiqua,Bold'\r\n" +
		"\r\n" +
		"Error: Printer \"NO-EXISTE\" not found\r\n"

	got := CleanStderr(stderr)
	if got != `Error: Printer "NO-EXISTE" not found` {
		t.Errorf("CleanStderr = %q", got)
	}
}

func TestCleanStderrOnlyNoiseIsEmpty(t *testing.T) {
	if got := CleanStderr("Syntax Error: No display font for 'Symbol'\n"); got != "" {
		t.Errorf("solo ruido debería quedar vacío, got %q", got)
	}
}

// Se conserva el final, que es donde las herramientas dejan el error que las
// hizo salir, y el corte no deja UTF-8 inválido.
func TestCleanStderrTruncatesKeepingTheEnd(t *testing.T) {
	long := strings.Repeat("ñ", maxStderr) + "\nError: el final importa"
	got := CleanStderr(long)
	if !strings.HasSuffix(got, "Error: el final importa") {
		t.Errorf("se perdió el final: %q", got)
	}
	if len(got) > maxStderr+len("…") {
		t.Errorf("no se acotó: %d bytes", len(got))
	}
	if !strings.HasPrefix(got, "…") {
		t.Errorf("falta la marca de recorte: %q", got[:10])
	}
}

func TestToolName(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Program Files\TeraAgent\poppler\bin\pdftocairo.exe`: "pdftocairo",
		"/usr/bin/pdftoppm": "pdftoppm",
		"pdftoppm.exe":      "pdftoppm",
	} {
		if got := toolName(in); got != want {
			t.Errorf("toolName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TERA_<HERRAMIENTA> gana a todo lo demás; es como un administrador fuerza una
// copia concreta de poppler.
func TestFindHonoursEnvOverride(t *testing.T) {
	fake := writeTool(t, t.TempDir(), exeName("pdftocairo"))
	t.Setenv("TERA_PDFTOCAIRO", fake)

	if got := Find("pdftocairo"); got != fake {
		t.Errorf("Find = %q, want %q", got, fake)
	}
	if !Available(fake) {
		t.Errorf("Available(%q) = false para un fichero que existe", fake)
	}
}

// Una variable que apunta a nada, relativa o a otro nombre no debe usarse: con
// el Agent corriendo como SYSTEM sería una forma de ejecutar un binario ajeno.
func TestFindRejectsUnsafeEnvOverride(t *testing.T) {
	dir := t.TempDir()
	for name, value := range map[string]string{
		"inexistente":   filepath.Join(dir, exeName("pdftocairo")),
		"relativa":      exeName("pdftocairo"),
		"otro nombre":   writeTool(t, dir, "pdftocairo.bat"),
		"un directorio": dir,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TERA_PDFTOCAIRO", value)
			// Find devuelve el nombre pelado cuando no encuentra nada: eso no es
			// aceptar la variable. Aceptarla es devolver esa ruta absoluta.
			if got := Find("pdftocairo"); got == value && filepath.IsAbs(got) {
				t.Errorf("Find aceptó %q", value)
			}
		})
	}
}

func TestAvailableRequiresAbsoluteRegularFile(t *testing.T) {
	dir := t.TempDir()
	if Available(filepath.Join(dir, "pdftocairo-que-no-existe")) {
		t.Error("Available = true para una ruta inexistente")
	}
	if Available("pdftocairo") {
		t.Error("Available = true para un nombre pelado sin ruta")
	}
	if Available(dir) {
		t.Error("Available = true para un directorio")
	}
	if !Available(writeTool(t, dir, "pdftocairo")) {
		t.Error("Available = false para un fichero que existe")
	}
}

func writeTool(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}
