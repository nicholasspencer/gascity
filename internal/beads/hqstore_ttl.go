package beads

import (
	"fmt"
	"log"
	"time"
)

// PurgeExpired removes expired ephemeral beads and closed main-tier beads whose
// retention window has elapsed. It returns the number of beads removed.
func (s *HQStore) PurgeExpired() (int, error) {
	s.counters.PurgeExpired.Add(1)
	now := time.Now()

	finish, err := s.beginLiveWrite()
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		_ = finish(false)
		return 0, err
	}

	var ids []string
	seenIDs := make(map[string]bool)
	addID := func(id string) bool {
		if seenIDs[id] {
			return false
		}
		seenIDs[id] = true
		ids = append(ids, id)
		return true
	}
	for id, bead := range s.wisps {
		expiresAt, ok := hqBeadExpiresAt(bead)
		if ok && !expiresAt.After(now) {
			addID(id)
		}
	}
	mailPurged := 0
	mailRetentionTTL := s.mailRetentionTTL
	if mailRetentionTTL > 0 {
		for id, bead := range s.wisps {
			if hqReadMessageRetentionExpired(bead, now, mailRetentionTTL) && addID(id) {
				mailPurged++
			}
		}
	}
	if s.closedTaskRetention > 0 {
		for id, bead := range s.main {
			if hqClosedTaskExpired(bead, now, s.closedTaskRetention) && !s.hasOpenChildrenLocked(id) {
				addID(id)
			}
		}
	}
	for _, id := range ids {
		s.deleteLocked(id)
	}
	purged := len(ids)
	if purged > 0 {
		s.counters.PurgeExpiredN.Add(int64(purged))
	}
	s.mu.Unlock()
	if err := finish(purged > 0); err != nil {
		return 0, err
	}
	if mailPurged > 0 {
		log.Printf("hqstore: purged %d read message wisps (retention_ttl=%s)", mailPurged, hqRetentionTTLString(mailRetentionTTL))
	}
	return purged, nil
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

func hqClosedTaskExpired(b Bead, now time.Time, retention time.Duration) bool {
	if b.Status != "closed" {
		return false
	}
	ref := b.CreatedAt
	if len(b.Metadata) > 0 && b.Metadata[hqClosedAtMetadataKey] != "" {
		if closedAt, err := time.Parse(time.RFC3339Nano, b.Metadata[hqClosedAtMetadataKey]); err == nil {
			ref = closedAt
		}
	}
	if ref.IsZero() {
		return false
	}
	return !ref.Add(retention).After(now)
}

func hqReadMessageRetentionExpired(b Bead, now time.Time, retention time.Duration) bool {
	if b.Type != "message" || b.Metadata[hqMailReadMetadataKey] != "true" || b.CreatedAt.IsZero() {
		return false
	}
	return !b.CreatedAt.Add(retention).After(now)
}

func hqRetentionTTLString(d time.Duration) string {
	if d > 0 && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(d/time.Hour))
	}
	return d.String()
}

func (s *HQStore) hasOpenChildrenLocked(parentID string) bool {
	for _, child := range s.main {
		if child.ParentID == parentID && child.Status != "closed" && child.Status != "archived" {
			return true
		}
	}
	for _, child := range s.wisps {
		if child.ParentID == parentID && child.Status != "closed" && child.Status != "archived" {
			return true
		}
	}
	return false
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
