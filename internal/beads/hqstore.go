package beads

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	hqDefaultCheckpointEvery = 1000
	hqWALFileName            = "wal.jsonl"
	hqCheckpointFileName     = "checkpoint.json"
	hqExpiresAtMetadataKey   = "expires_at"
	hqExpiresAtMetadataAlt   = "gc.expires_at"
)

// HQStore is a dormant, WAL-backed in-process Store implementation for the
// coordination-store migration experiments. It is not wired into live city
// storage; callers must opt in by opening it directly.
type HQStore struct {
	mu      sync.RWMutex
	writeMu sync.Mutex

	dir             string
	prefix          string
	seq             int
	checkpointEvery int
	writesSinceCP   int

	wal    *hqWAL
	closed bool
	walOn  bool

	main      map[string]Bead
	wisps     map[string]Bead
	order     []string
	orderSeen map[string]bool
	deps      []Dep
	mainIdx   hqTierIndex
	wispIdx   hqTierIndex

	ttlInterval time.Duration
	ttlStop     chan struct{}
	ttlDone     chan struct{}
}

type hqStoreOptions struct {
	prefix          string
	checkpointEvery int
	ttlInterval     time.Duration
	syncWrites      bool
	walEnabled      bool
}

// HQStoreOption customizes OpenHQStore.
type HQStoreOption func(*hqStoreOptions)

// WithHQStoreCheckpointEvery sets how many successful writes happen between
// automatic checkpoints. A value of 0 disables automatic checkpointing.
func WithHQStoreCheckpointEvery(n int) HQStoreOption {
	return func(o *hqStoreOptions) {
		if n < 0 {
			n = 0
		}
		o.checkpointEvery = n
	}
}

// WithHQStoreTTLInterval starts a background TTL sweeper at the given interval.
// A non-positive interval leaves TTL purge explicit via PurgeExpired.
func WithHQStoreTTLInterval(d time.Duration) HQStoreOption {
	return func(o *hqStoreOptions) {
		o.ttlInterval = d
	}
}

// WithHQStoreIDPrefix sets the generated ID prefix. Empty keeps the default.
func WithHQStoreIDPrefix(prefix string) HQStoreOption {
	return func(o *hqStoreOptions) {
		if prefix != "" {
			o.prefix = prefix
		}
	}
}

// WithHQStoreSyncWrites controls whether each WAL append calls fsync before
// the write returns. The default is true.
func WithHQStoreSyncWrites(syncWrites bool) HQStoreOption {
	return func(o *hqStoreOptions) {
		o.syncWrites = syncWrites
	}
}

// WithHQStoreWAL controls whether writes are appended to the JSONL WAL. The
// default is true; disabling WAL is intended for isolated hot-core benchmarks.
func WithHQStoreWAL(enabled bool) HQStoreOption {
	return func(o *hqStoreOptions) {
		o.walEnabled = enabled
	}
}

// OpenHQStore opens or creates a dormant HQStore rooted at dir.
func OpenHQStore(dir string, opts ...HQStoreOption) (*HQStore, error) {
	cfg := hqStoreOptions{
		prefix:          "hq",
		checkpointEvery: hqDefaultCheckpointEvery,
		syncWrites:      true,
		walEnabled:      true,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("opening hqstore: %w", err)
	}

	store := &HQStore{
		dir:             dir,
		prefix:          cfg.prefix,
		checkpointEvery: cfg.checkpointEvery,
		ttlInterval:     cfg.ttlInterval,
		walOn:           cfg.walEnabled,
	}
	store.resetCoreLocked()

	if err := store.loadCheckpoint(); err != nil {
		return nil, err
	}
	if cfg.walEnabled {
		if err := store.replayWAL(); err != nil {
			return nil, err
		}
		wal, err := openHQWAL(filepath.Join(dir, hqWALFileName), cfg.syncWrites)
		if err != nil {
			return nil, err
		}
		store.wal = wal
	}
	store.startTTLSweeper()
	return store, nil
}

// Shutdown releases the WAL file and stops the optional TTL sweeper. It is
// idempotent.
func (s *HQStore) Shutdown() error {
	s.stopTTLSweeper()

	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.wal == nil {
		return nil
	}
	if err := s.wal.close(); err != nil {
		return fmt.Errorf("shutting down hqstore: %w", err)
	}
	return nil
}

// Ping verifies that the store is open.
func (s *HQStore) Ping() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return fmt.Errorf("pinging hqstore: closed")
	}
	return nil
}

// Tx executes fn against the HQStore write surface.
func (s *HQStore) Tx(_ string, fn func(tx Tx) error) error {
	return runSequentialTx(s, fn)
}

func (s *HQStore) ensureOpenLocked() error {
	if s.closed {
		return fmt.Errorf("hqstore is closed")
	}
	if s.walOn && s.wal == nil {
		return fmt.Errorf("hqstore wal is not open")
	}
	return nil
}

func (s *HQStore) lockWriter() {
	if s.walOn {
		s.writeMu.Lock()
	}
}

func (s *HQStore) unlockWriter() {
	if s.walOn {
		s.writeMu.Unlock()
	}
}
