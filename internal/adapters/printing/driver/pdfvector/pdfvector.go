// Package pdfvector imprime PDF en impresoras de hoja de Windows (láser,
// inyección) dibujando el documento directamente en el driver de la impresora
// con `pdftocairo -print` de Poppler.
//
// Por qué existe, y no basta el driver GDI raster: ese camino convierte cada
// página en un bitmap y lo pinta con StretchDIBits. Los drivers host-based solo
// aceptan unos pocos MB de bitmap por página, así que una página con color (logo,
// cabecera corporativa) acababa reducida a ~170 ppp —texto borroso— después de
// haberla rasterizado a 600 ppp y de tener todas las páginas en memoria a la vez
// (8,6 GB comprometidos con una factura de 29 páginas en un equipo de 4 GB).
//
// Aquí el texto y los vectores llegan al driver como tales y la impresora los
// rasteriza a su resolución nativa; solo las imágenes del propio PDF (logo, QR)
// viajan como bitmap, a su resolución original. Medido en Windows 11 limpio con
// facturas del ERP (fpdf2) enviadas por WebSocket: 29 páginas en 25 s con el
// agente en 24 MB, y el texto idéntico al del PDF.
//
// El driver no conoce ni el ERP ni la configuración: recibe los bytes del PDF y
// el nombre de la impresora. Qué impresoras van por aquí lo decide el perfil
// (ver platform.NormalizeForWindows).
// Owner: Printing Engineer.
package pdfvector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/teraerp/tera-agent/internal/adapters/printing/popplerbin"
	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// printTimeout acota un trabajo completo. Es holgado a propósito: una factura de
// 29 páginas tarda ~30 s en un equipo normal y un listado largo en un PC de caja
// lento puede ir varias veces más despacio. Lo que tiene que cortar es el caso
// patológico —un driver que abre un diálogo en una sesión sin escritorio y se
// queda esperando para siempre—, no un documento largo.
const printTimeout = 5 * time.Minute

// docFile es el nombre del PDF temporal. pdftocairo lo usa como nombre del
// documento en la cola de impresión, así que se lanza con la ruta relativa: en
// la cola se ve "tera-agent.pdf" y no una ruta de %TEMP%.
const docFile = "tera-agent.pdf"

// Driver imprime PDF vía pdftocairo.
type Driver struct {
	log       ports.Logger
	bin       string
	available bool
	timeout   time.Duration
}

// New construye el driver sobre el pdftocairo de bin (ver popplerbin.Find).
//
// Si el ejecutable no existe, el driver se declara no disponible y deja de
// aceptar PDF. No es un error: el perfil de las impresoras de hoja lista también
// gdi-raster detrás de pdf, así que el resolver cae al modo imagen al componer
// el pipeline, antes de enviar nada a la impresora.
func New(log ports.Logger, bin string) *Driver {
	d := &Driver{log: log, bin: bin, available: popplerbin.Available(bin), timeout: printTimeout}
	if !d.available && log != nil {
		log.Warn("pdftocairo no encontrado: las impresoras de hoja imprimirán en modo imagen", "path", bin)
	}
	return d
}

// Accepts solo PDF, y solo si pdftocairo está instalado.
func (d *Driver) Accepts(f dp.DeviceFormat) bool { return f == dp.DevicePDF && d.available }

// Send imprime el PDF en printerID y espera a que el documento quede en la cola.
func (d *Driver) Send(ctx context.Context, printerID string, data []byte) error {
	if printerID == "" {
		return errors.New("pdfvector: impresora vacía")
	}
	if len(data) == 0 {
		return errors.New("pdfvector: PDF vacío")
	}

	// El PDF puede traer datos de clientes: directorio propio y borrado al
	// terminar, pase lo que pase. En Windows el 0o600 no aplica (Go solo lo usa
	// para el atributo de solo lectura); ahí protege el ACL del directorio
	// temporal del usuario, o el de SystemTemp cuando corre como servicio.
	dir, err := os.MkdirTemp("", "tera-print-*")
	if err != nil {
		return fmt.Errorf("pdfvector: directorio temporal: %w", err)
	}
	defer func() {
		// Un antivirus o el indexador pueden tener el PDF abierto: si no se puede
		// borrar tiene que quedar constancia, porque son datos de un cliente.
		if err := os.RemoveAll(dir); err != nil && d.log != nil {
			d.log.Warn("no se pudo borrar el PDF temporal", "dir", dir, "err", err)
		}
	}()
	if err := os.WriteFile(filepath.Join(dir, docFile), data, 0o600); err != nil {
		return fmt.Errorf("pdfvector: escribir PDF temporal: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	cmd := popplerbin.Command(runCtx, d.bin, printArgs(printerID, docFile)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	start := time.Now()
	if err := popplerbin.Run(cmd); err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return fmt.Errorf("pdfvector: la impresora %q no terminó de recibir el documento en %s; "+
				"revisa que su driver no esté esperando un diálogo o que la cola no esté bloqueada",
				printerID, d.timeout)
		}
		return popplerbin.RunError(d.bin, err, stderr.String())
	}

	if d.log != nil {
		d.log.Info("print job submitted (pdf vector)", "printer", printerID,
			"bytes", len(data), "ms", time.Since(start).Milliseconds())
	}
	return nil
}

// printArgs arma la línea de pdftocairo.
//
//   - -noshrink: tamaño real, 1:1 con el papel FÍSICO. Con este flag pdftocairo
//     toma como página el papel completo y desplaza el origen por el margen no
//     imprimible del hardware (ver win32BeginPage en poppler). Sin él, encaja la
//     página en el área imprimible y la reduce al ~96 %, sumando el margen del
//     hardware al que el documento ya trae. Es el mismo criterio que el modo
//     imagen con margen 0 (ports.DefaultPageMarginMM).
//   - Sin -paper: cada página pide al driver su propio tamaño, así una factura
//     carta sale en carta y un informe A4 en A4 en la misma impresora.
func printArgs(printerID, file string) []string {
	return []string{"-print", "-printer", printerID, "-noshrink", file}
}

var _ dp.Driver = (*Driver)(nil)
