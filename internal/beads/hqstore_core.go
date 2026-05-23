package beads

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type hqIDSet map[string]struct{}

type hqTierIndex struct {
	status   map[string]hqIDSet
	assignee map[string]hqIDSet
	typ      map[string]hqIDSet
	parent   map[string]hqIDSet
	label    map[string]hqIDSet
	metadata map[string]hqIDSet
}

func newHQTierIndex() hqTierIndex {
	return hqTierIndex{
		status:   make(map[string]hqIDSet),
		assignee: make(map[string]hqIDSet),
		typ:      make(map[string]hqIDSet),
		parent:   make(map[string]hqIDSet),
		label:    make(map[string]hqIDSet),
		metadata: make(map[string]hqIDSet),
	}
}

func (s *HQStore) resetCoreLocked() {
	s.main = make(map[string]Bead)
	s.wisps = make(map[string]Bead)
	s.order = nil
	s.orderSeen = make(map[string]bool)
	s.deps = nil
	s.mainIdx = newHQTierIndex()
	s.wispIdx = newHQTierIndex()
	s.seq = 0
	s.writesSinceCP = 0
}

// Create persists a new bead.
func (s *HQStore) Create(b Bead) (Bead, error) {
	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return Bead{}, err
	}

	stored := s.normalizeCreateLocked(b)
	if _, ok := s.findLocked(stored.ID); ok {
		s.mu.Unlock()
		return Bead{}, fmt.Errorf("creating bead %q: duplicate id", stored.ID)
	}
	s.mu.Unlock()

	entry := hqWALEntry{Op: hqWALCreate, Bead: &stored}
	if err := s.appendAndApply(entry, func() {
		s.upsertOwnedLocked(stored)
		for _, dep := range depsFromNeeds(stored) {
			s.depAddCoreLocked(dep.IssueID, dep.DependsOnID, dep.Type)
		}
	}); err != nil {
		return Bead{}, err
	}
	return cloneBead(stored), nil
}

func (s *HQStore) normalizeCreateLocked(b Bead) Bead {
	b = cloneBead(b)
	if b.ID == "" {
		s.seq++
		b.ID = fmt.Sprintf("%s-%d", s.prefix, s.seq)
	} else if n := numericIDSuffix(b.ID); n > s.seq {
		s.seq = n
	}
	if b.Status == "" {
		b.Status = "open"
	}
	if b.Type == "" {
		b.Type = "task"
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}
	return b
}

// Get retrieves a bead by ID.
func (s *HQStore) Get(id string) (Bead, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if b, ok := s.main[id]; ok {
		return cloneBead(b), nil
	}
	if b, ok := s.wisps[id]; ok {
		return cloneBead(b), nil
	}
	return Bead{}, fmt.Errorf("getting bead %q: %w", id, ErrNotFound)
}

// Update modifies fields of an existing bead.
func (s *HQStore) Update(id string, opts UpdateOpts) error {
	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	b, ok := s.findLocked(id)
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("updating bead %q: %w", id, ErrNotFound)
	}
	applyHQUpdate(&b, opts)
	s.mu.Unlock()
	return s.persistUpsertLocked("updating", id, b)
}

// Close sets a bead's status to closed.
func (s *HQStore) Close(id string) error {
	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	b, ok := s.findLocked(id)
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("closing bead %q: %w", id, ErrNotFound)
	}
	if b.Status == "closed" {
		s.mu.Unlock()
		return nil
	}
	b.Status = "closed"
	s.mu.Unlock()
	return s.persistUpsertLocked("closing", id, b)
}

// Reopen sets a bead's status to open.
func (s *HQStore) Reopen(id string) error {
	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	b, ok := s.findLocked(id)
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("reopening bead %q: %w", id, ErrNotFound)
	}
	if b.Status == "open" {
		s.mu.Unlock()
		return nil
	}
	b.Status = "open"
	s.mu.Unlock()
	return s.persistUpsertLocked("reopening", id, b)
}

// CloseAll closes multiple beads and applies metadata to each closed bead.
func (s *HQStore) CloseAll(ids []string, metadata map[string]string) (int, error) {
	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return 0, err
	}

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var changed []Bead
	for id := range idSet {
		b, ok := s.findLocked(id)
		if !ok || b.Status == "closed" {
			continue
		}
		b.Status = "closed"
		if len(metadata) > 0 {
			if b.Metadata == nil {
				b.Metadata = make(map[string]string, len(metadata))
			}
			for k, v := range metadata {
				b.Metadata[k] = v
			}
		}
		changed = append(changed, b)
	}
	if len(changed) == 0 {
		s.mu.Unlock()
		return 0, nil
	}
	s.mu.Unlock()

	entry := hqWALEntry{Op: hqWALUpsertBatch, Beads: changed}
	if err := s.appendAndApply(entry, func() {
		for _, b := range changed {
			s.upsertOwnedLocked(b)
		}
	}); err != nil {
		return 0, err
	}
	return len(changed), nil
}

// List returns beads matching the query.
func (s *HQStore) List(query ListQuery) ([]Bead, error) {
	if !query.HasFilter() && !query.AllowScan {
		return nil, fmt.Errorf("listing beads: %w", ErrQueryRequiresScan)
	}
	s.mu.RLock()

	candidates := s.candidateIDsLocked(query)
	snapshot := make([]Bead, 0, len(candidates))
	for _, id := range s.iterationIDsLocked(query, candidates) {
		if _, ok := candidates[id]; !ok {
			continue
		}
		if b, ok := s.main[id]; ok {
			snapshot = append(snapshot, cloneBead(b))
			continue
		}
		if b, ok := s.wisps[id]; ok {
			snapshot = append(snapshot, cloneBead(b))
		}
	}
	s.mu.RUnlock()

	result := make([]Bead, 0, len(snapshot))
	for _, b := range snapshot {
		if query.Matches(b) {
			result = append(result, b)
		}
	}
	sortBeadsForQuery(result, query.Sort)
	if query.Limit > 0 && len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result, nil
}

// ListOpen returns non-closed beads in creation order by default.
func (s *HQStore) ListOpen(status ...string) ([]Bead, error) {
	query := ListQuery{AllowScan: true}
	if len(status) > 0 {
		query.Status = status[0]
	}
	return s.List(query)
}

// Ready returns all open, unblocked actionable main-tier beads.
func (s *HQStore) Ready(query ...ReadyQuery) ([]Bead, error) {
	q := readyQueryFromArgs(query)
	s.mu.RLock()

	candidateIDs := s.readyCandidateIDsLocked(q)
	candidateSet := make(map[string]bool, len(candidateIDs))
	snapshot := make([]Bead, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		b, ok := s.main[id]
		if !ok {
			continue
		}
		candidateSet[id] = true
		snapshot = append(snapshot, cloneBead(b))
	}
	statusByID := make(map[string]string)
	deps := make([]Dep, 0, len(s.deps))
	for _, dep := range s.deps {
		if !candidateSet[dep.IssueID] {
			continue
		}
		deps = append(deps, dep)
		if target, ok := s.main[dep.DependsOnID]; ok {
			statusByID[dep.DependsOnID] = target.Status
		}
	}
	s.mu.RUnlock()

	var result []Bead
	for _, b := range snapshot {
		if b.Status != "open" {
			continue
		}
		if q.Assignee != "" && b.Assignee != q.Assignee {
			continue
		}
		if IsReadyExcludedType(b.Type) || hqBlockedBySnapshot(b.ID, deps, statusByID) {
			continue
		}
		result = append(result, cloneBead(b))
		if q.Limit > 0 && len(result) >= q.Limit {
			break
		}
	}
	return result, nil
}

func (s *HQStore) iterationIDsLocked(q ListQuery, candidates hqIDSet) []string {
	if q.Sort == SortDefault && !q.HasFilter() {
		return s.order
	}
	ids := make([]string, 0, len(candidates))
	for id := range candidates {
		ids = append(ids, id)
	}
	return ids
}

func (s *HQStore) readyCandidateIDsLocked(q ReadyQuery) []string {
	if q.Assignee == "" {
		return s.order
	}
	assigneeIDs := s.mainIdx.assignee[q.Assignee]
	openIDs := s.mainIdx.status["open"]
	if len(openIDs) < len(assigneeIDs) {
		ids := make([]string, 0, len(openIDs))
		for id := range openIDs {
			if _, ok := assigneeIDs[id]; ok {
				ids = append(ids, id)
			}
		}
		return ids
	}
	ids := make([]string, 0, len(assigneeIDs))
	for id := range assigneeIDs {
		if _, ok := openIDs[id]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// Children returns children of parentID.
func (s *HQStore) Children(parentID string, opts ...QueryOpt) ([]Bead, error) {
	return s.List(ListQuery{
		ParentID:      parentID,
		IncludeClosed: HasOpt(opts, IncludeClosed),
		Sort:          SortCreatedAsc,
	})
}

// ListByLabel returns beads matching a label.
func (s *HQStore) ListByLabel(label string, limit int, opts ...QueryOpt) ([]Bead, error) {
	return s.List(ListQuery{
		Label:         label,
		Limit:         limit,
		IncludeClosed: HasOpt(opts, IncludeClosed),
		Sort:          SortCreatedDesc,
		TierMode:      TierModeFromOpts(opts),
	})
}

// ListByAssignee returns beads assigned to assignee with status.
func (s *HQStore) ListByAssignee(assignee, status string, limit int) ([]Bead, error) {
	return s.List(ListQuery{
		Assignee: assignee,
		Status:   status,
		Limit:    limit,
		Sort:     SortCreatedDesc,
	})
}

// ListByMetadata returns beads whose metadata contains all filters.
func (s *HQStore) ListByMetadata(filters map[string]string, limit int, opts ...QueryOpt) ([]Bead, error) {
	return s.List(ListQuery{
		Metadata:      filters,
		Limit:         limit,
		IncludeClosed: HasOpt(opts, IncludeClosed),
		Sort:          SortCreatedDesc,
		TierMode:      TierModeFromOpts(opts),
	})
}

// SetMetadata sets a single metadata key-value pair.
func (s *HQStore) SetMetadata(id, key, value string) error {
	return s.SetMetadataBatch(id, map[string]string{key: value})
}

// SetMetadataBatch atomically merges metadata into a bead.
func (s *HQStore) SetMetadataBatch(id string, kvs map[string]string) error {
	if len(kvs) == 0 {
		return nil
	}
	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	b, ok := s.findLocked(id)
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("setting metadata batch on %q: %w", id, ErrNotFound)
	}
	if b.Metadata == nil {
		b.Metadata = make(map[string]string, len(kvs))
	}
	for k, v := range kvs {
		b.Metadata[k] = v
	}
	s.mu.Unlock()
	return s.persistUpsertLocked("setting metadata batch on", id, b)
}

// Delete permanently removes a bead and dependency edges touching it.
func (s *HQStore) Delete(id string) error {
	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	if _, ok := s.findLocked(id); !ok {
		s.mu.Unlock()
		return fmt.Errorf("deleting bead %q: %w", id, ErrNotFound)
	}
	s.mu.Unlock()

	entry := hqWALEntry{Op: hqWALDelete, ID: id}
	return s.appendAndApply(entry, func() {
		s.deleteLocked(id)
	})
}

// DepAdd records a dependency.
func (s *HQStore) DepAdd(issueID, dependsOnID, depType string) error {
	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	if depType == "" {
		depType = "blocks"
	}
	changed := s.depAddPreviewLocked(issueID, dependsOnID, depType)
	if !changed {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	dep := Dep{IssueID: issueID, DependsOnID: dependsOnID, Type: depType}
	entry := hqWALEntry{Op: hqWALDepAdd, Dep: &dep}
	return s.appendAndApply(entry, func() {
		s.depAddCoreLocked(issueID, dependsOnID, depType)
	})
}

// DepRemove removes a dependency between two beads.
func (s *HQStore) DepRemove(issueID, dependsOnID string) error {
	s.lockWriter()
	defer s.unlockWriter()

	s.mu.Lock()
	if err := s.ensureOpenLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	if !s.depExistsLocked(issueID, dependsOnID) {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	entry := hqWALEntry{Op: hqWALDepRemove, ID: issueID, TargetID: dependsOnID}
	return s.appendAndApply(entry, func() {
		s.depRemoveCoreLocked(issueID, dependsOnID)
	})
}

// DepList returns dependencies in the requested direction.
func (s *HQStore) DepList(id, direction string) ([]Dep, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Dep
	for _, d := range s.deps {
		switch direction {
		case "up":
			if d.DependsOnID == id {
				result = append(result, d)
			}
		default:
			if d.IssueID == id {
				result = append(result, d)
			}
		}
	}
	return result, nil
}

func (s *HQStore) persistUpsertLocked(action, id string, b Bead) error {
	entry := hqWALEntry{Op: hqWALUpsert, ID: id, Bead: &b}
	if err := s.appendAndApply(entry, func() {
		s.upsertOwnedLocked(b)
	}); err != nil {
		return fmt.Errorf("%s bead %q: %w", action, id, err)
	}
	return nil
}

func (s *HQStore) appendAndApply(entry hqWALEntry, apply func()) error {
	if s.wal != nil {
		if err := s.wal.append(entry); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	apply()
	s.writesSinceCP++
	if s.checkpointEvery > 0 && s.writesSinceCP >= s.checkpointEvery {
		return s.checkpointLocked()
	}
	return nil
}

func (s *HQStore) findLocked(id string) (Bead, bool) {
	if b, ok := s.main[id]; ok {
		return cloneBead(b), true
	}
	if b, ok := s.wisps[id]; ok {
		return cloneBead(b), true
	}
	return Bead{}, false
}

func (s *HQStore) upsertLocked(b Bead) {
	s.upsertOwnedLocked(cloneBead(b))
}

func (s *HQStore) upsertOwnedLocked(b Bead) {
	if old, ok := s.main[b.ID]; ok {
		s.mainIdx.remove(old)
		delete(s.main, b.ID)
	}
	if old, ok := s.wisps[b.ID]; ok {
		s.wispIdx.remove(old)
		delete(s.wisps, b.ID)
	}
	if !s.orderSeen[b.ID] {
		s.order = append(s.order, b.ID)
		s.orderSeen[b.ID] = true
	}
	if n := numericIDSuffix(b.ID); n > s.seq {
		s.seq = n
	}
	if b.Ephemeral {
		s.wisps[b.ID] = b
		s.wispIdx.add(b)
		return
	}
	s.main[b.ID] = b
	s.mainIdx.add(b)
}

func (s *HQStore) deleteLocked(id string) {
	if old, ok := s.main[id]; ok {
		s.mainIdx.remove(old)
		delete(s.main, id)
	}
	if old, ok := s.wisps[id]; ok {
		s.wispIdx.remove(old)
		delete(s.wisps, id)
	}
	filtered := s.deps[:0]
	for _, dep := range s.deps {
		if dep.IssueID == id || dep.DependsOnID == id {
			continue
		}
		filtered = append(filtered, dep)
	}
	s.deps = filtered
}

func (s *HQStore) candidateIDsLocked(q ListQuery) hqIDSet {
	switch q.TierMode {
	case TierWisps:
		return s.wispIdx.candidateIDs(q)
	case TierBoth:
		return unionHQIDSets(s.mainIdx.candidateIDs(q), s.wispIdx.candidateIDs(q))
	default:
		return s.mainIdx.candidateIDs(q)
	}
}

func hqBlockedBySnapshot(id string, deps []Dep, statusByID map[string]string) bool {
	for _, dep := range deps {
		if dep.IssueID != id {
			continue
		}
		switch dep.Type {
		case "blocks", "waits-for", "conditional-blocks":
		default:
			continue
		}
		if statusByID[dep.DependsOnID] != "closed" {
			return true
		}
	}
	return false
}

func (s *HQStore) depAddPreviewLocked(issueID, dependsOnID, depType string) bool {
	for _, d := range s.deps {
		if d.IssueID == issueID && d.DependsOnID == dependsOnID && d.Type == depType {
			return false
		}
		if d.IssueID == issueID && d.DependsOnID == dependsOnID && d.Type != "parent-child" && depType != "parent-child" {
			return d.Type != depType
		}
	}
	return true
}

func (s *HQStore) depAddCoreLocked(issueID, dependsOnID, depType string) {
	if depType == "" {
		depType = "blocks"
	}
	for i, d := range s.deps {
		if d.IssueID == issueID && d.DependsOnID == dependsOnID && d.Type == depType {
			return
		}
		if d.IssueID == issueID && d.DependsOnID == dependsOnID && d.Type != "parent-child" && depType != "parent-child" {
			s.deps[i].Type = depType
			return
		}
	}
	s.deps = append(s.deps, Dep{IssueID: issueID, DependsOnID: dependsOnID, Type: depType})
}

func (s *HQStore) depExistsLocked(issueID, dependsOnID string) bool {
	for _, d := range s.deps {
		if d.IssueID == issueID && d.DependsOnID == dependsOnID {
			return true
		}
	}
	return false
}

func (s *HQStore) depRemoveCoreLocked(issueID, dependsOnID string) {
	filtered := s.deps[:0]
	for _, d := range s.deps {
		if d.IssueID == issueID && d.DependsOnID == dependsOnID {
			continue
		}
		filtered = append(filtered, d)
	}
	s.deps = filtered
}

func applyHQUpdate(b *Bead, opts UpdateOpts) {
	if opts.Title != nil {
		b.Title = *opts.Title
	}
	if opts.Status != nil {
		b.Status = *opts.Status
	}
	if opts.Description != nil {
		b.Description = *opts.Description
	}
	if opts.Priority != nil {
		b.Priority = cloneIntPtr(opts.Priority)
	}
	if opts.ParentID != nil {
		b.ParentID = *opts.ParentID
	}
	if opts.Assignee != nil {
		b.Assignee = *opts.Assignee
	}
	if opts.Type != nil {
		b.Type = *opts.Type
	}
	if len(opts.Metadata) > 0 {
		if b.Metadata == nil {
			b.Metadata = make(map[string]string, len(opts.Metadata))
		}
		for k, v := range opts.Metadata {
			b.Metadata[k] = v
		}
	}
	if len(opts.Labels) > 0 {
		b.Labels = append(b.Labels, opts.Labels...)
	}
	if len(opts.RemoveLabels) > 0 {
		remove := make(map[string]bool, len(opts.RemoveLabels))
		for _, label := range opts.RemoveLabels {
			remove[label] = true
		}
		filtered := b.Labels[:0]
		for _, label := range b.Labels {
			if !remove[label] {
				filtered = append(filtered, label)
			}
		}
		b.Labels = filtered
	}
}

func depsFromNeeds(b Bead) []Dep {
	deps := make([]Dep, 0, len(b.Needs))
	for _, need := range b.Needs {
		depType := "blocks"
		dependsOnID := need
		if strings.Contains(need, ":") {
			parts := strings.SplitN(need, ":", 2)
			if parts[0] != "" && parts[1] != "" {
				depType = parts[0]
				dependsOnID = parts[1]
			}
		}
		deps = append(deps, Dep{
			IssueID:     b.ID,
			DependsOnID: dependsOnID,
			Type:        depType,
		})
	}
	return deps
}

func (i hqTierIndex) add(b Bead) {
	addHQIndex(i.status, b.Status, b.ID)
	addHQIndex(i.assignee, b.Assignee, b.ID)
	addHQIndex(i.typ, b.Type, b.ID)
	addHQIndex(i.parent, b.ParentID, b.ID)
	for _, label := range b.Labels {
		addHQIndex(i.label, label, b.ID)
	}
	for k, v := range b.Metadata {
		addHQIndex(i.metadata, hqMetadataIndexKey(k, v), b.ID)
	}
}

func (i hqTierIndex) remove(b Bead) {
	removeHQIndex(i.status, b.Status, b.ID)
	removeHQIndex(i.assignee, b.Assignee, b.ID)
	removeHQIndex(i.typ, b.Type, b.ID)
	removeHQIndex(i.parent, b.ParentID, b.ID)
	for _, label := range b.Labels {
		removeHQIndex(i.label, label, b.ID)
	}
	for k, v := range b.Metadata {
		removeHQIndex(i.metadata, hqMetadataIndexKey(k, v), b.ID)
	}
}

func (i hqTierIndex) candidateIDs(q ListQuery) hqIDSet {
	var candidates []hqIDSet
	if q.Status != "" {
		candidates = append(candidates, i.status[q.Status])
	} else if !q.IncludeClosed {
		candidates = append(candidates, i.nonClosedIDs())
	}
	if q.Type != "" {
		candidates = append(candidates, i.typ[q.Type])
	}
	if q.Assignee != "" {
		candidates = append(candidates, i.assignee[q.Assignee])
	}
	if q.ParentID != "" {
		candidates = append(candidates, i.parent[q.ParentID])
	}
	if q.Label != "" {
		candidates = append(candidates, i.label[q.Label])
	}
	for k, v := range q.Metadata {
		candidates = append(candidates, i.metadata[hqMetadataIndexKey(k, v)])
	}
	if len(candidates) == 0 {
		return i.allIDs()
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if len(c) < len(best) {
			best = c
		}
	}
	out := make(hqIDSet, len(best))
	for id := range best {
		if hqIDInAllSets(id, candidates) {
			out[id] = struct{}{}
		}
	}
	return out
}

func hqIDInAllSets(id string, sets []hqIDSet) bool {
	for _, set := range sets {
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

func (i hqTierIndex) nonClosedIDs() hqIDSet {
	out := make(hqIDSet)
	for id := range i.status["open"] {
		out[id] = struct{}{}
	}
	for id := range i.status["in_progress"] {
		out[id] = struct{}{}
	}
	return out
}

func (i hqTierIndex) allIDs() hqIDSet {
	out := make(hqIDSet)
	for _, ids := range i.status {
		for id := range ids {
			out[id] = struct{}{}
		}
	}
	return out
}

func addHQIndex(index map[string]hqIDSet, key, id string) {
	ids := index[key]
	if ids == nil {
		ids = make(hqIDSet)
		index[key] = ids
	}
	ids[id] = struct{}{}
}

func removeHQIndex(index map[string]hqIDSet, key, id string) {
	ids := index[key]
	if ids == nil {
		return
	}
	delete(ids, id)
	if len(ids) == 0 {
		delete(index, key)
	}
}

func unionHQIDSets(a, b hqIDSet) hqIDSet {
	out := make(hqIDSet, len(a)+len(b))
	for id := range a {
		out[id] = struct{}{}
	}
	for id := range b {
		out[id] = struct{}{}
	}
	return out
}

func hqMetadataIndexKey(k, v string) string {
	return k + "\x00" + v
}

func numericIDSuffix(id string) int {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] < '0' || id[i] > '9' {
			if i == len(id)-1 {
				return 0
			}
			n, _ := strconv.Atoi(id[i+1:])
			return n
		}
	}
	n, _ := strconv.Atoi(id)
	return n
}

func snapshotHQDeps(in []Dep) []Dep {
	return slices.Clone(in)
}
