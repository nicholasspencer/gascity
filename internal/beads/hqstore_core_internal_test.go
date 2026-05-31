package beads

import "testing"

func TestHQTierIndexCandidateIDsUnionsAssignees(t *testing.T) {
	idx := newHQTierIndex()
	idx.add(Bead{ID: "route-a-open", Status: "open", Assignee: "rig/route-a"})
	idx.add(Bead{ID: "route-b-progress", Status: "in_progress", Assignee: "rig/route-b"})
	idx.add(Bead{ID: "route-c-open", Status: "open", Assignee: "rig/route-c"})
	idx.add(Bead{ID: "route-a-closed", Status: "closed", Assignee: "rig/route-a"})

	got := idx.candidateIDs(ListQuery{Assignees: []string{"rig/route-a", "rig/route-b"}})

	assertHQIDSet(t, got, []string{"route-a-open", "route-b-progress"})
}

func TestHQTierIndexCandidateIDsAssigneesEmptyUnionStaysEmpty(t *testing.T) {
	idx := newHQTierIndex()
	idx.add(Bead{ID: "route-a-open", Status: "open", Assignee: "rig/route-a"})
	idx.add(Bead{ID: "route-b-progress", Status: "in_progress", Assignee: "rig/route-b"})
	idx.add(Bead{ID: "route-c-closed", Status: "closed", Assignee: "rig/route-c"})

	got := idx.candidateIDs(ListQuery{Assignees: []string{"rig/missing"}, Status: "open"})
	assertHQIDSet(t, got, nil)

	got = idx.candidateIDs(ListQuery{Assignees: []string{"rig/missing"}, IncludeClosed: true})
	assertHQIDSet(t, got, nil)
}

func assertHQIDSet(t *testing.T, got hqIDSet, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d: %v", len(got), len(want), got)
	}
	for _, id := range want {
		if _, ok := got[id]; !ok {
			t.Fatalf("got missing ID %q: %v", id, got)
		}
	}
}
