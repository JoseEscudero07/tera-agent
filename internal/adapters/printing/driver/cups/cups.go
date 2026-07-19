// Package cups is the CUPS driver: it only sends bytes to a printer via `lp`.
// It never renders or converts. Raw mode (`-o raw`) is used for device
// languages (ESC/POS, ZPL); native mode lets CUPS filters render PDF/PNG.
// Owner: Printing Engineer.
package cups

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Driver sends bytes to a CUPS printer.
type Driver struct {
	log     ports.Logger
	raw     bool
	accepts map[dp.DeviceFormat]bool
}

// NewRaw returns a driver that submits pre-formatted device bytes (ESC/POS,
// ZPL, EPL, raw) with `-o raw`.
func NewRaw(log ports.Logger) dp.Driver {
	return &Driver{log: log, raw: true, accepts: formats(dp.DeviceESCPOS, dp.DeviceZPL, dp.DeviceEPL, dp.DeviceRaw)}
}

// NewNative returns a driver for formats CUPS renders itself (PDF, PNG).
func NewNative(log ports.Logger) dp.Driver {
	return &Driver{log: log, raw: false, accepts: formats(dp.DevicePDF, dp.DevicePNG)}
}

func formats(fs ...dp.DeviceFormat) map[dp.DeviceFormat]bool {
	m := make(map[dp.DeviceFormat]bool, len(fs))
	for _, f := range fs {
		m[f] = true
	}
	return m
}

func (d *Driver) Accepts(f dp.DeviceFormat) bool { return d.accepts[f] }

func (d *Driver) Send(ctx context.Context, printerID string, data []byte) error {
	if printerID == "" {
		return fmt.Errorf("cups: empty printer id")
	}
	if len(data) == 0 {
		return fmt.Errorf("cups: empty data")
	}

	args := []string{"-d", printerID}
	if d.raw {
		args = append(args, "-o", "raw")
	}

	cmd := exec.CommandContext(ctx, "lp", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C") // stable, language-independent output
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cups: lp failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	d.log.Info("print job submitted", "printer", printerID, "raw", d.raw, "lp", strings.TrimSpace(string(out)))
	return nil
}
