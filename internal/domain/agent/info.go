package agent

import (
	"sync"
	"time"
)

// Info is a thread-safe holder for the runtime identity the Agent learns from
// the Backend (company/branch/equipo) and the last sync time. The lifecycle
// writes it; the UI reads it. Owner: Go Core Engineer.
type Info struct {
	mu       sync.RWMutex
	identity Identity
	lastSync time.Time
}

// NewInfo returns an empty Info.
func NewInfo() *Info { return &Info{} }

// SetIdentity stores the identity received at authentication.
func (i *Info) SetIdentity(id Identity) {
	i.mu.Lock()
	i.identity = id
	i.mu.Unlock()
}

// Touch records the current time as the last successful sync.
func (i *Info) Touch() {
	i.mu.Lock()
	i.lastSync = time.Now()
	i.mu.Unlock()
}

// Snapshot returns a copy of the identity and last sync time.
func (i *Info) Snapshot() (Identity, time.Time) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.identity, i.lastSync
}
