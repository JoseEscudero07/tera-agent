//go:build !windows

package main

import (
	"bufio"
	"errors"
	"os"
	"strings"

	"github.com/teraerp/tera-agent/internal/adapters/apppath"
)

// requireElevatedForScope no impone elevación fuera de Windows: en Linux el
// registro del servicio se hace como root (sudo) y los permisos de fichero ya
// protegen la configuración. En user scope tampoco hace falta.
func requireElevatedForScope(apppath.Scope) error { return nil }

// hintForWriteError no añade pista fuera de Windows (el error de permisos de
// POSIX ya es claro y el flujo es sudo).
func hintForWriteError(_ apppath.Scope, err error) error { return err }

// readSecret lee una línea. Fuera de Windows el flujo recomendado es --token-file
// para no dejar el Token en la consola; si se teclea, se lee en claro.
func readSecret(promptMsg string) (string, error) {
	os.Stdout.WriteString(promptMsg)
	sc := bufio.NewScanner(os.Stdin)
	if sc.Scan() {
		return strings.TrimRight(sc.Text(), "\r\n"), nil
	}
	return "", nil
}

// restartServiceBestEffort no gestiona servicios fuera de Windows: en Linux es
// systemd quien lo hace. Se avisa al operador para que lo reinicie él.
func restartServiceBestEffort() error {
	return errors.New("reinicia el servicio manualmente (systemctl restart tera-agent)")
}
