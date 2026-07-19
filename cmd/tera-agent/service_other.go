//go:build !windows

package main

import "errors"

// isWindowsService is always false off Windows.
func isWindowsService() bool { return false }

// runWindowsService is never called off Windows; present for compilation.
func runWindowsService(_, _ func()) error { return nil }

// controlService is unsupported off Windows.
func controlService(_, _ string) error {
	return errors.New("service management is only available on Windows")
}
