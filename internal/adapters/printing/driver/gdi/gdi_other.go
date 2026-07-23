//go:build !windows

// Package gdi es un stub en plataformas que no son Windows: mantiene el paquete
// importable desde código común (platform.Drivers) sin arrastrar syscalls
// específicos del sistema. Nadie llama a New() fuera de platform_windows.go.
package gdi

import (
	"context"
	"fmt"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Driver es el stub no-Windows.
type Driver struct{}

// New devuelve un driver que rechaza todo; no debería wiring-ear fuera de
// Windows. La firma acepta el Binarizer para igualar la de Windows.
func New(_ ports.Logger, _ dp.Binarizer) dp.Driver { return &Driver{} }

func (*Driver) Accepts(dp.DeviceFormat) bool { return false }
func (*Driver) Send(context.Context, string, []byte) error {
	return fmt.Errorf("gdi: driver only available on Windows")
}
