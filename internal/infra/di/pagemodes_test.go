package di

import (
	"sync"
	"testing"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

func TestPageModesDefaultsToVector(t *testing.T) {
	if got := newPageModes(nil).Of("HP"); got != ports.PageModeVector {
		t.Errorf("sin resolutor = %q, want vector", got)
	}
}

// El panel reinstala el resolutor al guardar; el cambio tiene que verse ya.
func TestPageModesSetReplacesResolver(t *testing.T) {
	m := newPageModes(ports.Config{}.PageModeOf)
	cfg := ports.Config{Printers: []ports.ManagedPrinter{{Name: "Samsung", PageMode: ports.PageModeImage}}}
	m.Set(cfg.PageModeOf)
	if got := m.Of("Samsung"); got != ports.PageModeImage {
		t.Errorf("tras Set = %q, want image", got)
	}
	m.Set(nil)
	if got := m.Of("Samsung"); got != ports.PageModeVector {
		t.Errorf("Set(nil) = %q, want vector", got)
	}
}

// Guardar desde el panel y leer desde un trabajo ocurren a la vez (go test -race).
func TestPageModesConcurrentSetAndOf(t *testing.T) {
	m := newPageModes(nil)
	image := ports.Config{Printers: []ports.ManagedPrinter{{Name: "HP", PageMode: ports.PageModeImage}}}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); m.Set(image.PageModeOf) }()
		go func() { defer wg.Done(); _ = m.Of("HP") }()
	}
	wg.Wait()
}
