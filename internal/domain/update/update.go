// Package update modela la actualización automática del Agent: el manifiesto que
// publica el Backend, el estado que ve el panel y los puertos que implementan
// los adapters (cliente del manifiesto y aplicador del binario). El dominio no
// conoce HTTP, ficheros ni el SO — sólo las reglas de versión.
// Owner: DevOps Engineer (orquestación) + Security Engineer (verificación).
package update

import "context"

// Manifest es la respuesta del endpoint de versión del Backend
// (GET {base}/agent/version). Contrato estable con el Backend.
type Manifest struct {
	Latest       string `json:"latest"`        // versión más nueva disponible, semver
	URL          string `json:"url"`           // descarga HTTPS del binario para os/arch
	SHA256       string `json:"sha256"`        // hash hex del binario, verificación obligatoria
	Mandatory    bool   `json:"mandatory"`     // true = forzar la actualización
	MinSupported string `json:"min_supported"` // por debajo de esto se considera obsoleto
	Notes        string `json:"notes"`         // notas de la versión (se muestran en el panel)
}

// Status es lo que el panel muestra al cliente: dónde está y qué hay disponible.
type Status struct {
	Current   string `json:"current"`   // versión instalada
	Latest    string `json:"latest"`    // última publicada (vacía si no se pudo consultar)
	Available bool   `json:"available"` // hay una versión más nueva
	Mandatory bool   `json:"mandatory"` // la nueva es obligatoria (o la actual está por debajo de min_supported)
	Notes     string `json:"notes"`
}

// Source obtiene el manifiesto del Backend. Lo implementa adapters/update/manifest.
type Source interface {
	// Fetch consulta la versión disponible para el os/arch dados, informando la
	// versión actual (el Backend puede usarla para decidir mandatory/min).
	Fetch(ctx context.Context, current, os, arch string) (Manifest, error)
}

// Applier descarga, verifica y aplica una actualización, reiniciando el Agent.
// Lo implementa adapters/update/selfupdate (específico por SO). Es la superficie
// sensible: DEBE verificar el SHA256 antes de reemplazar nada.
type Applier interface {
	Apply(ctx context.Context, m Manifest) error
}
