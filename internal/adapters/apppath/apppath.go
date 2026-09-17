// Package apppath resuelve dónde viven la configuración y los datos del Agent
// según el modo de despliegue (servicio del equipo vs aplicación de usuario).
//
// El modo NO es configuración: lo decide quién instala (servicio de Windows o
// autoarranque de usuario) y viaja en la línea de arranque como --scope. De él
// depende la SEGURIDAD de las rutas:
//   - service: datos en una carpeta del equipo, cuya ACL se restringe a SYSTEM y
//     Administradores (ver adapters/fsacl). Un cajero sin privilegios no puede
//     leer el Token ni manipular la configuración que arranca como LocalSystem.
//   - user: datos en el perfil del usuario, protegidos por los permisos del
//     propio perfil. Cada cuenta tiene su registro; nada se comparte entre ellas.
//
// La resolución concreta de las carpetas del SO es específica de plataforma
// (apppath_windows.go / apppath_other.go). Este fichero solo fija el contrato.
package apppath

import "fmt"

// Scope es el modo de despliegue que determina las rutas.
type Scope string

const (
	// ScopeService: servicio del equipo (LocalSystem en Windows). Datos en una
	// carpeta común protegida por ACL.
	ScopeService Scope = "service"
	// ScopeUser: aplicación de usuario. Datos en el perfil del usuario actual.
	ScopeUser Scope = "user"
)

// ParseScope valida el valor recibido por línea de comandos.
func ParseScope(s string) (Scope, error) {
	switch Scope(s) {
	case ScopeService:
		return ScopeService, nil
	case ScopeUser:
		return ScopeUser, nil
	default:
		return "", fmt.Errorf("scope desconocido %q (usa service|user)", s)
	}
}

// ConfigPath devuelve la ruta del config.yaml para el scope dado.
func (s Scope) ConfigPath() (string, error) {
	dir, err := s.DataDir()
	if err != nil {
		return "", err
	}
	return joinConfig(dir), nil
}
