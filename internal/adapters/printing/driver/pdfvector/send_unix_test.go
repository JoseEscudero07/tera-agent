//go:build !windows

package pdfvector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teraerp/tera-agent/internal/adapters/logger"
)

// Estos tests sustituyen pdftocairo por un script de shell que registra cómo lo
// llamaron. Verifican el contrato con el proceso hijo (argumentos, fichero,
// directorio, errores y timeout) sin impresora ni Windows. La impresión real se
// valida en la VM limpia (docs/ACCEPTANCE.md).

// fakeTool escribe un "pdftocairo" que ejecuta body y devuelve su ruta. El
// script ve en $CAPTURE un directorio donde dejar lo que quiera inspeccionar.
func fakeTool(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture")
	if err := os.Mkdir(capture, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CAPTURE", capture)
	bin := filepath.Join(dir, "pdftocairo")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestSendRunsPdftocairoWithTheDocument(t *testing.T) {
	bin := fakeTool(t, `printf '%s\n' "$@" > "$CAPTURE/args"; pwd > "$CAPTURE/pwd"; cp "$5" "$CAPTURE/doc.pdf"`)
	d := New(logger.New("error"), bin)

	pdf := []byte("%PDF-1.4 factura de prueba")
	if err := d.Send(context.Background(), "HP LaserJet Pro", pdf); err != nil {
		t.Fatalf("Send: %v", err)
	}

	capture := os.Getenv("CAPTURE")
	args, _ := os.ReadFile(filepath.Join(capture, "args"))
	if got := strings.TrimSpace(string(args)); got != "-print\n-printer\nHP LaserJet Pro\n-noshrink\ntera-agent.pdf" {
		t.Errorf("args =\n%s", got)
	}
	doc, err := os.ReadFile(filepath.Join(capture, "doc.pdf"))
	if err != nil {
		t.Fatalf("pdftocairo no recibió el documento: %v", err)
	}
	if string(doc) != string(pdf) {
		t.Errorf("documento alterado: %q", doc)
	}

	// El directorio temporal (con datos del cliente) no debe sobrevivir al trabajo.
	pwd, _ := os.ReadFile(filepath.Join(capture, "pwd"))
	if tmp := strings.TrimSpace(string(pwd)); tmp == "" {
		t.Error("no se registró el directorio de trabajo")
	} else if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("el directorio temporal %s sigue existiendo", tmp)
	}
}

// Lo que ve soporte: el error real de pdftocairo, sin los avisos de fuentes.
func TestSendReportsPdftocairoError(t *testing.T) {
	bin := fakeTool(t, `echo "Syntax Error: No display font for 'Symbol'" >&2; echo 'Error: Printer "NO-EXISTE" not found' >&2; exit 99`)
	d := New(logger.New("error"), bin)

	err := d.Send(context.Background(), "NO-EXISTE", []byte("%PDF"))
	if err == nil {
		t.Fatal("Send no devolvió error con exit 99")
	}
	msg := err.Error()
	if !strings.Contains(msg, `Printer "NO-EXISTE" not found`) {
		t.Errorf("el error no explica la causa: %s", msg)
	}
	if strings.Contains(msg, "No display font") {
		t.Errorf("el error arrastra ruido de fuentes: %s", msg)
	}
}

func TestSendTimesOut(t *testing.T) {
	// exec en lugar de lanzar sleep como hijo: así matar el proceso cierra la
	// tubería de stderr y Run vuelve en cuanto vence el plazo.
	d := New(logger.New("error"), fakeTool(t, "exec sleep 30"))
	d.timeout = 200 * time.Millisecond

	start := time.Now()
	err := d.Send(context.Background(), "LASER", []byte("%PDF"))
	if err == nil {
		t.Fatal("Send no respetó el timeout")
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("Send tardó %s pese al timeout", time.Since(start))
	}
	if !strings.Contains(err.Error(), "no terminó de recibir el documento") {
		t.Errorf("mensaje de timeout poco claro: %v", err)
	}
}

// Si quien cancela es el llamante (apagado del Agent), no hay que culpar a la
// impresora de un timeout que no ocurrió.
func TestSendCallerCancellationIsNotReportedAsTimeout(t *testing.T) {
	d := New(logger.New("error"), fakeTool(t, "exec sleep 30"))
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()

	err := d.Send(ctx, "LASER", []byte("%PDF"))
	if err == nil {
		t.Fatal("Send no se canceló")
	}
	if strings.Contains(err.Error(), "no terminó de recibir el documento") {
		t.Errorf("cancelación reportada como timeout de impresora: %v", err)
	}
}
