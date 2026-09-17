// Package logger provides the standard-library slog implementation of
// ports.Logger. Owner: Go Core Engineer.
package logger

import (
	"io"
	"log/slog"
	"os"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

// slogLogger adapts *slog.Logger to ports.Logger.
type slogLogger struct {
	l *slog.Logger
}

// New returns a ports.Logger writing structured logs to stderr at the given
// level (debug|info|warn|error; anything else defaults to info).
func New(level string) ports.Logger {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: parseLevel(level)})
	return &slogLogger{l: slog.New(h)}
}

// NewWithFile returns a Logger writing to stderr and to a rotating file. If file
// is empty it behaves like New. Owner: Go Core Engineer.
func NewWithFile(level, file string, maxSizeMB, maxBackups int) (ports.Logger, error) {
	if file == "" {
		return New(level), nil
	}
	rw, err := newRotatingWriter(file, maxSizeMB, maxBackups)
	if err != nil {
		return nil, err
	}
	w := io.MultiWriter(bestEffortWriter{os.Stderr}, rw)
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})
	return &slogLogger{l: slog.New(h)}, nil
}

// bestEffortWriter descarta los errores del writer que envuelve y siempre
// reporta éxito. Se usa para stderr.
//
// Motivo: el binario de la bandeja se compila con -H=windowsgui, así que corre
// sin consola y os.Stderr es un handle inválido — cada escritura falla con
// "handle inválido". io.MultiWriter aborta en el primer error y no llega a los
// writers siguientes, de modo que ese fallo dejaba el log EN FICHERO
// completamente vacío: el Agent parecía no registrar nada y no había forma de
// diagnosticar un problema en el equipo de un cliente.
//
// Envolver stderr (y no reordenar los writers) mantiene la intención: escribir en
// consola cuando la hay, sin que su ausencia afecte al fichero.
type bestEffortWriter struct{ w io.Writer }

func (b bestEffortWriter) Write(p []byte) (int, error) {
	_, _ = b.w.Write(p)
	return len(p), nil
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (s *slogLogger) Debug(msg string, args ...any) { s.l.Debug(msg, args...) }
func (s *slogLogger) Info(msg string, args ...any)  { s.l.Info(msg, args...) }
func (s *slogLogger) Warn(msg string, args ...any)  { s.l.Warn(msg, args...) }
func (s *slogLogger) Error(msg string, args ...any) { s.l.Error(msg, args...) }
