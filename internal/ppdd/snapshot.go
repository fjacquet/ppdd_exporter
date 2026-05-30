package ppdd

import (
	"sync"
	"time"
)

// SystemSnapshot is one system's result for a single collection cycle.
type SystemSnapshot struct {
	System     string
	LastScrape time.Time
	OK         bool   // true if at least the system was reachable & authenticated
	Err        string // top-level failure (auth/unreachable); empty when OK
	Samples    []Sample
}

// Snapshot is an immutable, point-in-time view across all systems.
type Snapshot struct {
	BuiltAt time.Time
	Systems []*SystemSnapshot
}

// SnapshotStore holds the latest Snapshot behind an RWMutex pointer-swap.
type SnapshotStore struct {
	mu   sync.RWMutex
	snap *Snapshot
}

// NewSnapshotStore returns a store pre-populated with an empty snapshot so
// readers never see nil before the first collection cycle.
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{snap: &Snapshot{}}
}

// Store atomically swaps in a new snapshot.
func (s *SnapshotStore) Store(snap *Snapshot) {
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
}

// Load returns the current snapshot (never nil).
func (s *SnapshotStore) Load() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}
