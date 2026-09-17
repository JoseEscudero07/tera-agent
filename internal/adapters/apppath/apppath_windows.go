//go:build windows

package apppath

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// appDir es el nombre de la carpeta del Agent bajo la carpeta base de cada scope.
const appDir = "TeraAgent"

// DataDir resuelve la carpeta de datos:
//   - service: %ProgramData%\TeraAgent  (FOLDERID_ProgramData, común al equipo)
//   - user:    %LOCALAPPDATA%\TeraAgent (FOLDERID_LocalAppData, NO Roaming)
//
// LocalAppData y no Roaming a propósito: un perfil móvil copiaría el Token a
// todos los equipos donde el usuario inicie sesión, y dos agentes con el mismo
// Token duplicarían cada impresión.
func (s Scope) DataDir() (string, error) {
	var folder *windows.KNOWNFOLDERID
	switch s {
	case ScopeService:
		folder = windows.FOLDERID_ProgramData
	default:
		folder = windows.FOLDERID_LocalAppData
	}
	base, err := windows.KnownFolderPath(folder, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDir), nil
}

func joinConfig(dir string) string { return filepath.Join(dir, "config.yaml") }
