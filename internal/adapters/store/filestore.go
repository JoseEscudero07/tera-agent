// Package store provides a file-based ports.JobStore: it persists processed job
// ids (idempotency across restarts) and buffered result frames (offline
// resilience). It is safe for concurrent use. Owner: Go Core Engineer.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

// maxDone bounds how many processed ids are kept (oldest are pruned).
const maxDone = 2000

type diskState struct {
	Done    map[string]int64  `json:"done"`
	Pending []json.RawMessage `json:"pending"`
}

// Store is a file-backed JobStore.
type Store struct {
	mu      sync.Mutex
	path    string
	done    map[string]int64
	pending [][]byte
}

// New returns a Store persisting to path, loading any existing state.
func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: path, done: make(map[string]int64)}
	s.load()
	return s, nil
}

func (s *Store) Seen(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.done[id]
	return ok
}

func (s *Store) MarkDone(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done[id] = time.Now().Unix()
	s.prune()
	s.persist()
}

func (s *Store) AddPending(frame []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, append([]byte(nil), frame...))
	s.persist()
}

func (s *Store) TakePending() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.pending
	s.pending = nil
	s.persist()
	return out
}

// prune keeps at most maxDone ids, dropping the oldest by timestamp.
func (s *Store) prune() {
	if len(s.done) <= maxDone {
		return
	}
	type kv struct {
		id string
		ts int64
	}
	items := make([]kv, 0, len(s.done))
	for id, ts := range s.done {
		items = append(items, kv{id, ts})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ts < items[j].ts })
	for _, it := range items[:len(items)-maxDone] {
		delete(s.done, it.id)
	}
}

func (s *Store) load() {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var ds diskState
	if json.Unmarshal(b, &ds) != nil {
		return
	}
	if ds.Done != nil {
		s.done = ds.Done
	}
	for _, p := range ds.Pending {
		s.pending = append(s.pending, []byte(p))
	}
}

// persist writes the state atomically (temp file + rename). Caller holds the lock.
func (s *Store) persist() {
	ds := diskState{Done: s.done}
	for _, p := range s.pending {
		ds.Pending = append(ds.Pending, json.RawMessage(p))
	}
	b, err := json.Marshal(ds)
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, s.path)
	}
}

var _ ports.JobStore = (*Store)(nil)
