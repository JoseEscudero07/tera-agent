//go:build !linux && !darwin

// Fallback for platforms without a silent-print backend yet (e.g. Windows,
// pending the Windows Print API adapter). Keeps cross-compilation green.
// Owner: Printing Engineer.
package printing

import (
	"context"
	"errors"

	"github.com/teraerp/tera-agent/internal/app/ports"
	domainprinting "github.com/teraerp/tera-agent/internal/domain/printing"
)

var errUnsupported = errors.New("printing: no silent-print backend for this platform yet")

type unsupported struct{}

// New returns a Port/Discovery that report the platform is unsupported.
func New(_ ports.Logger) (domainprinting.Port, domainprinting.Discovery) {
	u := &unsupported{}
	return u, u
}

func (u *unsupported) Print(context.Context, domainprinting.Document) error {
	return errUnsupported
}

func (u *unsupported) List(context.Context) ([]domainprinting.Printer, error) {
	return nil, errUnsupported
}

func (u *unsupported) StatusOf(context.Context, string) (domainprinting.Status, error) {
	return domainprinting.StatusUnknown, errUnsupported
}
