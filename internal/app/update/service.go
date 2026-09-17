// Package update orquesta la comprobación y aplicación de actualizaciones del
// Agent. Depende sólo de los puertos de dominio (update.Source / update.Applier)
// y del logger; no conoce HTTP ni el SO. Owner: DevOps Engineer.
package update

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/teraerp/tera-agent/internal/app/ports"
	du "github.com/teraerp/tera-agent/internal/domain/update"
)

// Service comprueba y aplica actualizaciones. Es thread-safe: el panel puede
// llamarlo mientras un chequeo periódico corre en background.
type Service struct {
	current string
	src     du.Source
	applier du.Applier
	log     ports.Logger

	mu   sync.Mutex
	last du.Status // último estado conocido (cacheado para el panel)
}

// New crea el servicio de update con la versión actual del binario.
func New(current string, src du.Source, applier du.Applier, log ports.Logger) *Service {
	return &Service{
		current: current,
		src:     src,
		applier: applier,
		log:     log,
		last:    du.Status{Current: current},
	}
}

// Check consulta el Backend y devuelve el estado (actual vs disponible). No
// aplica nada. Cachea el resultado para lecturas rápidas del panel.
func (s *Service) Check(ctx context.Context) (du.Status, error) {
	m, err := s.src.Fetch(ctx, s.current, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return s.Cached(), fmt.Errorf("update: no se pudo consultar la versión: %w", err)
	}
	st := du.Status{
		Current:   s.current,
		Latest:    m.Latest,
		Available: du.Newer(m.Latest, s.current),
		Notes:     m.Notes,
	}
	// Obligatoria si el Backend lo marca, o si la versión actual quedó por
	// debajo del mínimo soportado.
	st.Mandatory = m.Mandatory || (m.MinSupported != "" && du.Compare(s.current, m.MinSupported) < 0)

	s.mu.Lock()
	s.last = st
	s.mu.Unlock()
	s.log.Info("update check", "current", s.current, "latest", m.Latest, "available", st.Available)
	return st, nil
}

// Cached devuelve el último estado conocido sin tocar la red.
func (s *Service) Cached() du.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// Apply consulta la versión, y si hay una más nueva la descarga, verifica y la
// instala (el Applier reinicia el Agent). Devuelve error claro si no hay nada
// que aplicar o si la verificación falla.
func (s *Service) Apply(ctx context.Context) error {
	m, err := s.src.Fetch(ctx, s.current, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("update: no se pudo consultar la versión: %w", err)
	}
	if !du.Newer(m.Latest, s.current) {
		return fmt.Errorf("update: ya estás en la última versión (%s)", s.current)
	}
	if m.URL == "" || m.SHA256 == "" {
		return fmt.Errorf("update: el manifiesto no trae URL o SHA256 (no se aplica sin verificación)")
	}
	s.log.Info("update apply: descargando", "from", s.current, "to", m.Latest)
	if err := s.applier.Apply(ctx, m); err != nil {
		return fmt.Errorf("update: fallo al aplicar %s: %w", m.Latest, err)
	}
	s.log.Info("update apply: instalada, reiniciando", "version", m.Latest)
	return nil
}
