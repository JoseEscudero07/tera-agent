//go:build !windows

package popplerbin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Si la herramienta termina bien pero deja un proceso hijo con stderr abierto
// (un driver de impresora puede hacerlo), Run tiene que volver igualmente y
// contarlo como éxito.
func TestRunReturnsWhenChildHoldsStderr(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "tool")
	script := "#!/bin/sh\nsleep 30 &\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := Command(context.Background(), bin)
	cmd.WaitDelay = 200 * time.Millisecond
	var stderr []byte
	cmd.Stderr = writerFunc(func(p []byte) (int, error) { stderr = append(stderr, p...); return len(p), nil })

	start := time.Now()
	if err := Run(cmd); err != nil {
		t.Fatalf("Run = %v; un exit 0 con un hijo rezagado es un éxito", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("Run tardó %s: esperó al proceso hijo", time.Since(start))
	}
}

// Con timeout y un hijo que retiene la tubería, Run debe volver con error poco
// después del plazo, no cuando el hijo termine.
func TestRunTimeoutDoesNotWaitForChild(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "tool")
	script := "#!/bin/sh\nsleep 30 &\nsleep 30\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd := Command(ctx, bin)
	cmd.WaitDelay = 200 * time.Millisecond
	cmd.Stderr = writerFunc(func(p []byte) (int, error) { return len(p), nil })

	start := time.Now()
	if err := Run(cmd); err == nil {
		t.Fatal("Run no devolvió error tras el timeout")
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("Run tardó %s pese al timeout", time.Since(start))
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
