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
	w := io.MultiWriter(os.Stderr, rw)
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})
	return &slogLogger{l: slog.New(h)}, nil
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
