package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// rotatingWriter is a minimal size-based rotating file writer (stdlib only).
// When the file exceeds maxSize it is renamed to "<path>.1", shifting older
// backups, keeping at most maxBackups of them.
type rotatingWriter struct {
	mu         sync.Mutex
	path       string
	maxSize    int64
	maxBackups int
	f          *os.File
	size       int64
}

func newRotatingWriter(path string, maxSizeMB, maxBackups int) (*rotatingWriter, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = 5
	}
	if maxBackups < 0 {
		maxBackups = 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	w := &rotatingWriter{
		path:       path,
		maxSize:    int64(maxSizeMB) * 1024 * 1024,
		maxBackups: maxBackups,
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	w.f = f
	w.size = 0
	if info, err := f.Stat(); err == nil {
		w.size = info.Size()
	}
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.maxSize {
		_ = w.rotate()
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// Close libera el fichero. El Agent no lo llama: vive hasta que el proceso
// termina y el SO cierra el handle. Existe para que las pruebas puedan borrar el
// fichero — en Windows no se puede eliminar un fichero con un handle abierto.
func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

func (w *rotatingWriter) rotate() error {
	_ = w.f.Close()
	if w.maxBackups <= 0 {
		_ = os.Remove(w.path)
		return w.open()
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", w.path, w.maxBackups))
	for i := w.maxBackups - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", w.path, i), fmt.Sprintf("%s.%d", w.path, i+1))
	}
	_ = os.Rename(w.path, w.path+".1")
	return w.open()
}
