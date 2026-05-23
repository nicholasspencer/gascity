package beads

import (
	"time"
)

// PurgeExpired removes ephemeral beads whose expires_at metadata is in the
// past. It returns the number of beads removed.
func (s *HQStore) PurgeExpired() (int, error) {
	now := time.Now()

	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return 0, err
	}

	var ids []string
	for id, bead := range s.wisps {
		expiresAt, ok := hqBeadExpiresAt(bead)
		if ok && expiresAt.Before(now) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		s.mu.Unlock()
		return 0, nil
	}
	s.mu.Unlock()

	entry := hqWALEntry{Op: hqWALDeleteBatch, Beads: make([]Bead, 0, len(ids))}
	for _, id := range ids {
		entry.Beads = append(entry.Beads, Bead{ID: id})
	}
	if err := s.appendAndApply(entry, func() {
		for _, id := range ids {
			s.deleteLocked(id)
		}
	}); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func hqBeadExpiresAt(b Bead) (time.Time, bool) {
	if len(b.Metadata) == 0 {
		return time.Time{}, false
	}
	raw := b.Metadata[hqExpiresAtMetadataKey]
	if raw == "" {
		raw = b.Metadata[hqExpiresAtMetadataAlt]
	}
	if raw == "" {
		return time.Time{}, false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return expiresAt, true
}

func (s *HQStore) startTTLSweeper() {
	if s.ttlInterval <= 0 {
		return
	}
	s.ttlStop = make(chan struct{})
	s.ttlDone = make(chan struct{})
	go func() {
		defer close(s.ttlDone)
		ticker := time.NewTicker(s.ttlInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, _ = s.PurgeExpired()
			case <-s.ttlStop:
				return
			}
		}
	}()
}

func (s *HQStore) stopTTLSweeper() {
	if s.ttlStop == nil {
		return
	}
	close(s.ttlStop)
	<-s.ttlDone
	s.ttlStop = nil
	s.ttlDone = nil
}
