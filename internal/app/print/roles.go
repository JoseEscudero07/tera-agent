// Package print — role-based printer resolver for backend-automatic jobs.
//
// El backend envía trabajos con `printer_id=""` y `role="cocina"` cuando NO
// conoce el nombre físico de la impresora en la sede (path automático). El
// Agent local resuelve el rol contra sus `ports.ManagedPrinter` — que el
// usuario etiqueta desde el panel.
//
// Regla de resolución:
//  1. Impresoras activadas y con el rol pedido → si sólo una, esa; si varias,
//     preferimos la predeterminada, si no la primera.
//  2. Ninguna con el rol → usar la predeterminada del agente.
//  3. Sin predeterminada → cadena vacía (el caller decide cómo fallar).
package print

import (
	"sync"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

// RoleResolver es thread-safe: la UI puede actualizarlo al guardar cambios en
// el panel sin coordinar con el goroutine que atiende jobs.
type RoleResolver struct {
	mu             sync.RWMutex
	printers       []ports.ManagedPrinter
	defaultPrinter string
}

// NewRoleResolver crea un resolver a partir del snapshot inicial del config.
func NewRoleResolver(printers []ports.ManagedPrinter, defaultPrinter string) *RoleResolver {
	r := &RoleResolver{}
	r.Update(printers, defaultPrinter)
	return r
}

// Update reemplaza el snapshot. Llamar cuando el usuario cambia roles o
// predeterminada desde la UI para que el próximo job vea el estado fresco.
func (r *RoleResolver) Update(printers []ports.ManagedPrinter, defaultPrinter string) {
	// Copia defensiva para que quien llamó pueda seguir mutando su slice.
	dup := make([]ports.ManagedPrinter, len(printers))
	copy(dup, printers)
	r.mu.Lock()
	r.printers = dup
	r.defaultPrinter = defaultPrinter
	r.mu.Unlock()
}

// Resolve devuelve el nombre de la impresora física para el rol pedido.
// Rol vacío = "el backend no supo, dame la predeterminada del agente".
func (r *RoleResolver) Resolve(role string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if role != "" {
		var matches []ports.ManagedPrinter
		for _, p := range r.printers {
			if string(p.Role) == role && p.Enabled {
				matches = append(matches, p)
			}
		}
		if len(matches) == 1 {
			return matches[0].Name
		}
		if len(matches) > 1 {
			// Varios candidatos → predeterminada si coincide, si no el primero.
			for _, m := range matches {
				if m.Name == r.defaultPrinter {
					return m.Name
				}
			}
			return matches[0].Name
		}
	}
	return r.defaultPrinter
}
