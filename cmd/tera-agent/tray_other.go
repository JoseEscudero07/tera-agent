//go:build !windows

package main

import (
	"context"

	"github.com/teraerp/tera-agent/internal/infra/di"
)

// traySupported is false on non-Windows builds (no system-tray dependency, to
// keep these builds cgo-free). A tray build for Linux/macOS can be added later.
func traySupported() bool { return false }

// runWithTray falls back to a headless run on platforms without tray support.
func runWithTray(ctx context.Context, stop func(), app *di.App) {
	defer stop()
	runLoop(ctx, app)
}
