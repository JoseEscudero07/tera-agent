// Package profile provides an in-memory ProfileCache. The communication layer
// populates it when the Backend pushes printer profiles (on auth and on change);
// the print engine reads from it. The Agent never persists business logic here.
// Owner: Go Core Engineer.
package profile

import (
	"fmt"
	"sync"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

type memoryCache struct {
	mu sync.RWMutex
	m  map[string]dp.PrinterProfile
}

// NewMemoryCache returns an empty in-memory ProfileCache.
func NewMemoryCache() dp.ProfileCache {
	return &memoryCache{m: make(map[string]dp.PrinterProfile)}
}

func (c *memoryCache) Profile(printerID string) (dp.PrinterProfile, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.m[printerID]
	if !ok {
		return dp.PrinterProfile{}, fmt.Errorf("profile: unknown printer %q (no profile from ERP)", printerID)
	}
	return p, nil
}

func (c *memoryCache) Set(p dp.PrinterProfile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[p.PrinterID] = p
}

func (c *memoryCache) SetAll(ps []dp.PrinterProfile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = make(map[string]dp.PrinterProfile, len(ps))
	for _, p := range ps {
		c.m[p.PrinterID] = p
	}
}

func (c *memoryCache) Forget(printerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, printerID)
}
