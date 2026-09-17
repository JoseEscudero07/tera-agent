//go:build !windows

package apppath

import (
	"os"
	"path/filepath"
)

// En Linux/macOS el despliegue como servicio usa systemd con --config explícito
// (ver deploy/systemd), así que estas rutas son un valor por defecto razonable
// más que la vía principal. El endurecimiento de permisos de Windows no aplica:
// en POSIX el config se guarda con modo 0600 y la carpeta de servicio pertenece
// a root.
func (s Scope) DataDir() (string, error) {
	switch s {
	case ScopeService:
		return "/var/lib/tera-agent", nil
	default:
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "tera-agent"), nil
	}
}

func joinConfig(dir string) string { return filepath.Join(dir, "config.yaml") }
