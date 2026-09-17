// Package popplerbin localiza y lanza los ejecutables de Poppler que el Agent
// usa como procesos hijo: pdftoppm (rasterizador) y pdftocairo (impresión
// vectorial en Windows).
//
// Existe como paquete propio porque esas herramientas las usan dos adapters
// distintos —rasterizer/poppler y driver/pdfvector— y ninguno debe importar al
// otro. Aquí vive lo que comparten: dónde buscar el .exe, cómo lanzarlo sin
// ventana de consola y cómo explicar sus fallos a quien da soporte.
// Owner: Printing Engineer.
package popplerbin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Find resuelve la ruta ABSOLUTA de una herramienta de Poppler ("pdftoppm",
// "pdftocairo"). Orden:
//  1. Variable de entorno TERA_<HERRAMIENTA> (p. ej. TERA_PDFTOPPM), solo si es
//     una ruta absoluta a un fichero con el nombre esperado. Útil para tests y
//     administradores.
//  2. Junto al ejecutable del Agent: permite dejar el .exe al lado de
//     tera-agent.exe sin tocar el PATH (los servicios corren como LocalSystem y
//     no ven el PATH del usuario).
//  3. <exe>/poppler/bin/ — donde lo deja el instalador de Windows.
//  4. <exe>/bin/.
//  5. El PATH del proceso, solo con resultado absoluto y nombre exacto.
//
// Lo que NO se acepta, a propósito (estas herramientas pueden correr como
// SYSTEM en modo servicio, y un binario plantado sería una escalada local):
//   - resoluciones por el directorio de trabajo o por entradas relativas del PATH
//     (exec.ErrDot): Go las bloquea desde 1.19 por esa razón;
//   - otro nombre que el esperado: en Windows LookPath prueba las extensiones de
//     PATHEXT y resolvería "pdftocairo.exe.bat" si alguien lo deja en el PATH, y
//     cmd.exe no respeta el escapado de argumentos de Go.
//
// Si no la encuentra devuelve el nombre pelado: Available lo detecta y exec
// fallará con un error que nombra la herramienta.
func Find(tool string) string {
	bin := exeName(tool)

	if p := os.Getenv("TERA_" + strings.ToUpper(tool)); p != "" && isExecutableFile(p, bin) {
		return p
	}

	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, c := range []string{
			filepath.Join(dir, bin),
			filepath.Join(dir, "poppler", "bin", bin),
			filepath.Join(dir, "bin", bin),
		} {
			if isExecutableFile(c, bin) {
				return c
			}
		}
	}

	if p, err := exec.LookPath(bin); err == nil && isExecutableFile(p, bin) {
		return p
	}

	return bin
}

// Available reporta si la ruta que devolvió Find apunta a un ejecutable real.
func Available(bin string) bool {
	return isExecutableFile(bin, filepath.Base(bin))
}

// exeName es el nombre de fichero de la herramienta en este SO.
func exeName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}
	return tool
}

// isExecutableFile: ruta absoluta, fichero regular y nombre exacto (sin
// distinguir mayúsculas en Windows).
func isExecutableFile(path, name string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	base := filepath.Base(path)
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(base, name) {
			return false
		}
	} else if base != name {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// waitDelay acota cuánto se espera a que se cierren stdout/stderr después de que
// la herramienta termine o se la mate por timeout. Sin él, si un driver de
// impresora cargado dentro de pdftocairo lanza un proceso que hereda stderr,
// Run no vuelve aunque pdftocairo ya haya salido: el trabajo (y con él el bucle
// de sesión del Agent) se quedaría colgado más allá de cualquier timeout.
const waitDelay = 10 * time.Second

// Command prepara la ejecución de una herramienta de Poppler: sin ventana de
// consola (ver hideConsole), con LC_ALL=C para que los mensajes de error no
// dependan del idioma del equipo y con WaitDelay. Ejecútalo con Run.
func Command(ctx context.Context, bin string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	hideConsole(cmd)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.WaitDelay = waitDelay
	return cmd
}

// Run ejecuta cmd. exec.ErrWaitDelay se trata como éxito: solo se devuelve
// cuando la herramienta salió con código 0 y lo único pendiente era una tubería
// que retenía un proceso hijo.
func Run(cmd *exec.Cmd) error {
	if err := cmd.Run(); err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return err
	}
	return nil
}

// dllNotFoundExit es STATUS_DLL_NOT_FOUND (0xC0000135) visto como código de
// salida: Windows no llegó a ejecutar el binario porque le falta una DLL.
const dllNotFoundExit = -1073741515

// RunError explica por qué falló una herramienta de Poppler.
//
// El caso 0xC0000135 merece mensaje propio: el proceso no arranca siquiera, así
// que stderr viene vacío y el error crudo ("exit status 0xc0000135") no dice nada
// a quien da soporte en el equipo de un cliente. En la práctica siempre significa
// lo mismo: falta el runtime de Visual C++ que Poppler importa y que no viene en
// su bundle.
func RunError(bin string, err error, stderr string) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == dllNotFoundExit {
		return dllNotFoundError(bin)
	}
	name := toolName(bin)
	if detail := CleanStderr(stderr); detail != "" {
		return fmt.Errorf("poppler: %s (%s): %w: %s", name, bin, err, detail)
	}
	return fmt.Errorf("poppler: %s (%s): %w", name, bin, err)
}

// dllNotFoundError redacta el diagnóstico de 0xC0000135 con la acción concreta a
// tomar, que es lo que necesita quien está delante del equipo del cliente.
func dllNotFoundError(bin string) error {
	return fmt.Errorf("poppler: %s no pudo arrancar: falta una DLL requerida (0xC0000135). "+
		"Normalmente es el runtime de Visual C++: comprueba que msvcp140.dll, "+
		"vcruntime140.dll y vcruntime140_1.dll estén junto a %s.exe, o instala "+
		"el Microsoft Visual C++ Redistributable (x64)", bin, toolName(bin))
}

// maxStderr acota lo que se sube al mensaje de error: acaba en el toast del panel
// y en el estado del trabajo en el ERP, que no son sitio para un volcado.
const maxStderr = 400

// CleanStderr deja solo las líneas de stderr que explican un fallo.
//
// Poppler en Windows escribe en cada ejecución una línea "Syntax Error: No
// display font for '...'" por cada fuente base que no encuentra en el sistema
// (Symbol, ArialNarrow, BookAntiqua...). Son avisos, no errores: el documento se
// procesa igual. Sin filtrarlos, el mensaje real ("Printer not found", "Couldn't
// read xref table") queda enterrado debajo de veinte líneas de ruido.
func CleanStderr(stderr string) string {
	var keep []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "No display font for") {
			continue
		}
		keep = append(keep, line)
	}
	out := strings.Join(keep, " | ")
	if len(out) > maxStderr {
		// Cortar por bytes puede partir una runa: ToValidUTF8 limpia el borde.
		out = "…" + strings.ToValidUTF8(out[len(out)-maxStderr:], "")
	}
	return out
}

// toolName devuelve el nombre de la herramienta sin ruta ni extensión, para
// mensajes: "C:\...\pdftocairo.exe" -> "pdftocairo".
func toolName(bin string) string {
	base := filepath.Base(strings.ReplaceAll(bin, `\`, "/"))
	return strings.TrimSuffix(base, filepath.Ext(base))
}
