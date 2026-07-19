// Package discovery enumerates printers and reports physical status via CUPS
// (lpstat). Output is forced to the C locale so parsing is language-independent.
// Owner: Printing Engineer.
package discovery

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// CUPS implements domain/printing.Discovery over lpstat.
type CUPS struct{ log ports.Logger }

// NewCUPS returns a CUPS-based discovery.
func NewCUPS(log ports.Logger) dp.Discovery { return &CUPS{log: log} }

func (c *CUPS) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd
}

func (c *CUPS) List(ctx context.Context) ([]dp.Printer, error) {
	out, err := c.command(ctx, "lpstat", "-e").Output()
	if err != nil {
		return nil, fmt.Errorf("discovery: lpstat -e: %w", err)
	}
	var printers []dp.Printer
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		status, _ := c.StatusOf(ctx, name)
		printers = append(printers, dp.Printer{ID: name, Name: name, Driver: "cups", Status: status})
	}
	return printers, nil
}

func (c *CUPS) StatusOf(ctx context.Context, printerID string) (dp.Status, error) {
	out, err := c.command(ctx, "lpstat", "-p", printerID).Output()
	if err != nil {
		return dp.StatusUnknown, nil
	}
	s := string(out) // C locale => English keywords
	switch {
	case strings.Contains(s, "printing"):
		return dp.StatusBusy, nil
	case strings.Contains(s, "is idle"):
		return dp.StatusReady, nil
	case strings.Contains(s, "disabled"):
		return dp.StatusOffline, nil
	default:
		return dp.StatusUnknown, nil
	}
}
