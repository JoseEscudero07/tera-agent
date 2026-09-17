package logger

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failingWriter simula os.Stderr en un binario -H=windowsgui: cada escritura
// falla porque el handle no es válido.
type failingWriter struct{ calls int }

func (f *failingWriter) Write(p []byte) (int, error) {
	f.calls++
	return 0, errors.New("write /dev/stderr: handle inválido")
}

// Regresión: un stderr que falla no debe impedir que el log llegue al fichero.
// io.MultiWriter aborta en el primer error, así que sin bestEffortWriter el
// fichero de log quedaba vacío en el binario de la bandeja.
func TestBestEffortWriterKeepsMultiWriterGoing(t *testing.T) {
	stderr := &failingWriter{}
	var file strings.Builder

	mw := io.MultiWriter(bestEffortWriter{stderr}, &file)
	n, err := mw.Write([]byte("linea de log\n"))
	if err != nil {
		t.Fatalf("Write devolvió error %v; el fallo de stderr no debe propagarse", err)
	}
	if n != len("linea de log\n") {
		t.Errorf("n = %d; se esperaba %d", n, len("linea de log\n"))
	}
	if file.String() != "linea de log\n" {
		t.Errorf("el fichero recibió %q; se esperaba la línea completa", file.String())
	}
	if stderr.calls != 1 {
		t.Errorf("stderr recibió %d escrituras; se esperaba 1 (intento best-effort)", stderr.calls)
	}
}

// Comprobación de extremo a extremo del logger con fichero: lo que se registra
// tiene que acabar en disco.
//
// No usamos t.TempDir(): su limpieza es estricta y en Windows falla porque el
// writer mantiene el fichero abierto (el Agent nunca lo cierra, vive hasta salir).
func TestNewWithFileWritesToDisk(t *testing.T) {
	dir, err := os.MkdirTemp("", "logger-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) }) // best-effort: el handle sigue abierto

	path := filepath.Join(dir, "sub", "tera-agent.log")
	log, err := NewWithFile("info", path, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	log.Info("mensaje de prueba", "clave", "valor")

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se creó el fichero de log: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, "mensaje de prueba") || !strings.Contains(out, "clave=valor") {
		t.Errorf("el log no contiene el mensaje esperado: %q", out)
	}
}

// La rotación cierra y reabre el fichero; Close debe dejarlo liberado para que se
// pueda borrar (en Windows, un handle abierto lo impide).
func TestRotatingWriterClose(t *testing.T) {
	dir, err := os.MkdirTemp("", "rotate-test-*")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "app.log")

	w, err := newRotatingWriter(path, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hola\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close repetido debe ser inocuo, dio %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Errorf("tras Close el fichero debe poder borrarse: %v", err)
	}
}

// Sin ruta de fichero, NewWithFile degrada al logger de solo consola sin fallar.
func TestNewWithFileEmptyPath(t *testing.T) {
	log, err := NewWithFile("warn", "", 0, 0)
	if err != nil {
		t.Fatalf("err = %v; sin fichero no debe fallar", err)
	}
	if log == nil {
		t.Fatal("logger nil")
	}
	log.Warn("no debe entrar en pánico")
}
