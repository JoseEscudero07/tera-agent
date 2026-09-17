//go:build !windows

package main

import (
	"fmt"
	"os"
)

// fatalStartup reporta un fallo irrecuperable de arranque y termina el proceso.
// Fuera de Windows el Agent siempre tiene stderr utilizable (terminal, o el
// journal cuando lo lanza systemd), así que basta con escribir ahí.
func fatalStartup(err error) {
	fmt.Fprintln(os.Stderr, "tera-agent: startup failed:", err)
	os.Exit(1)
}
