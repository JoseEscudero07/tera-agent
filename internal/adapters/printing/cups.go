//go:build linux || darwin

// CUPS backend: silent printing on Linux/macOS via the standard CUPS command
// line tools (lp / lpstat). No external Go dependencies. Owner: Printing Engineer.
package printing

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/teraerp/tera-agent/internal/app/ports"
	domainprinting "github.com/teraerp/tera-agent/internal/domain/printing"
)

// cups implements the printing Port and Discovery over CUPS.
type cups struct {
	log ports.Logger
}

// command builds a CUPS command forced to the C locale so its output is stable
// and language-independent (status parsing must not depend on the user locale).
func (c *cups) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd
}

// New returns the platform printing Port and Discovery. On Linux/macOS these
// use CUPS (lp/lpstat), which print silently with no dialog.
func New(log ports.Logger) (domainprinting.Port, domainprinting.Discovery) {
	c := &cups{log: log}
	return c, c
}

// List enumerates the available CUPS destinations and their status.
func (c *cups) List(ctx context.Context) ([]domainprinting.Printer, error) {
	out, err := c.command(ctx, "lpstat", "-e").Output()
	if err != nil {
		return nil, fmt.Errorf("cups: lpstat -e: %w", err)
	}
	var printers []domainprinting.Printer
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		status, _ := c.StatusOf(ctx, name)
		printers = append(printers, domainprinting.Printer{
			ID:     name,
			Name:   name,
			Driver: "cups",
			Status: status,
		})
	}
	return printers, nil
}

// StatusOf reports the status of a single printer by parsing `lpstat -p`.
func (c *cups) StatusOf(ctx context.Context, printerID string) (domainprinting.Status, error) {
	out, err := c.command(ctx, "lpstat", "-p", printerID).Output()
	if err != nil {
		return domainprinting.StatusUnknown, nil
	}
	// Output is forced to the C locale by command(); parse the English text.
	s := string(out)
	switch {
	case strings.Contains(s, "printing"):
		return domainprinting.StatusBusy, nil
	case strings.Contains(s, "is idle"):
		return domainprinting.StatusReady, nil
	case strings.Contains(s, "disabled"):
		return domainprinting.StatusOffline, nil
	default:
		return domainprinting.StatusUnknown, nil
	}
}

// Print submits a document to a printer silently. The bytes are piped to `lp`
// via stdin; with Raw set, `-o raw` bypasses driver filters (ESC/POS, ZPL).
func (c *cups) Print(ctx context.Context, doc domainprinting.Document) error {
	if doc.PrinterID == "" {
		return fmt.Errorf("cups: empty printer id")
	}
	if len(doc.Data) == 0 {
		return fmt.Errorf("cups: empty document")
	}

	args := []string{"-d", doc.PrinterID}
	if doc.Raw {
		args = append(args, "-o", "raw")
	}

	cmd := c.command(ctx, "lp", args...)
	cmd.Stdin = bytes.NewReader(doc.Data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cups: lp failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	c.log.Info("print job submitted", "printer", doc.PrinterID, "lp", strings.TrimSpace(string(out)))
	return nil
}

var (
	_ domainprinting.Port      = (*cups)(nil)
	_ domainprinting.Discovery = (*cups)(nil)
)
