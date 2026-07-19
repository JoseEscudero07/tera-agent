// Package file is a Driver that writes the encoded output to a file instead of
// a printer. It is a safe way to inspect the exact bytes a pipeline produces
// (dry run) without consuming paper. Owner: Printing Engineer.
package file

import (
	"context"
	"os"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Driver writes device bytes to Path.
type Driver struct {
	path string
	log  ports.Logger
}

// New returns a file driver that writes to path.
func New(path string, log ports.Logger) dp.Driver { return &Driver{path: path, log: log} }

// Accepts any device format (it just stores the bytes).
func (Driver) Accepts(dp.DeviceFormat) bool { return true }

func (d *Driver) Send(_ context.Context, printerID string, data []byte) error {
	if err := os.WriteFile(d.path, data, 0o644); err != nil {
		return err
	}
	d.log.Info("wrote encoded output to file", "printer", printerID, "file", d.path, "bytes", len(data))
	return nil
}
