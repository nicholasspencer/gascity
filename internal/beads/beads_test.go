package beads

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestIsContainerType(t *testing.T) {
	tests := []struct {
		typ  string
		want bool
	}{
		{"convoy", true},
		{"epic", false},
		{"task", false},
		{"message", false},
		{"", false},
		{"CONVOY", false}, // case-sensitive
	}
	for _, tt := range tests {
		if got := IsContainerType(tt.typ); got != tt.want {
			t.Errorf("IsContainerType(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestIsMoleculeType(t *testing.T) {
	tests := []struct {
		typ  string
		want bool
	}{
		{"molecule", true},
		{"wisp", true},
		{"task", false},
		{"convoy", false},
		{"step", false},
		{"", false},
		{"MOLECULE", false}, // case-sensitive
	}
	for _, tt := range tests {
		if got := IsMoleculeType(tt.typ); got != tt.want {
			t.Errorf("IsMoleculeType(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestIsReadyExcludedType(t *testing.T) {
	tests := []struct {
		typ  string
		want bool
	}{
		{"merge-request", true},
		{"gate", true},
		{"molecule", true},
		{"step", true},
		{"message", true},
		{"session", true},
		{"agent", true},
		{"role", true},
		{"rig", true},
		{"task", false},
		{"convoy", false},
		{"wisp", false},
		{"", false},
		{"MOLECULE", false}, // case-sensitive
	}
	for _, tt := range tests {
		if got := IsReadyExcludedType(tt.typ); got != tt.want {
			t.Errorf("IsReadyExcludedType(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestBeadJSONOmitsZeroLifecycleTimestamps(t *testing.T) {
	base := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(Bead{ID: "bd-1", Title: "open", Status: "open", CreatedAt: base})
	if err != nil {
		t.Fatalf("Marshal open bead: %v", err)
	}
	if strings.Contains(string(raw), "updated_at") || strings.Contains(string(raw), "closed_at") {
		t.Fatalf("zero lifecycle timestamps leaked into JSON: %s", raw)
	}

	raw, err = json.Marshal(Bead{
		ID:        "bd-1",
		Title:     "closed",
		Status:    "closed",
		CreatedAt: base,
		UpdatedAt: base.Add(time.Minute),
		ClosedAt:  base.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Marshal closed bead: %v", err)
	}
	if !strings.Contains(string(raw), "updated_at") || !strings.Contains(string(raw), "closed_at") {
		t.Fatalf("nonzero lifecycle timestamps missing from JSON: %s", raw)
	}
}

func TestListQueryCreatedBeforeFiltersBeforeLimit(t *testing.T) {
	base := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	items := []Bead{
		{ID: "newer-2", Title: "newer 2", Status: "closed", CreatedAt: base.Add(2 * time.Minute), Labels: []string{"order-run:digest"}},
		{ID: "newer-1", Title: "newer 1", Status: "closed", CreatedAt: base.Add(time.Minute), Labels: []string{"order-run:digest"}},
		{ID: "older-2", Title: "older 2", Status: "closed", CreatedAt: base.Add(-2 * time.Minute), Labels: []string{"order-run:digest"}},
		{ID: "older-1", Title: "older 1", Status: "closed", CreatedAt: base.Add(-time.Minute), Labels: []string{"order-run:digest"}},
	}

	got := ApplyListQuery(items, ListQuery{
		Label:         "order-run:digest",
		CreatedBefore: base,
		Limit:         1,
		IncludeClosed: true,
		Sort:          SortCreatedDesc,
	})

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1: %+v", len(got), got)
	}
	if got[0].ID != "older-1" {
		t.Fatalf("got[0].ID = %q, want older-1", got[0].ID)
	}
}

func TestListQueryClosedBeforeFiltersBeforeLimit(t *testing.T) {
	base := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	items := []Bead{
		{ID: "created-old-closed-new", Title: "new close", Status: "closed", CreatedAt: base.Add(-48 * time.Hour), ClosedAt: base.Add(time.Minute), Labels: []string{"gc:session"}},
		{ID: "created-new-closed-old", Title: "old close", Status: "closed", CreatedAt: base.Add(time.Minute), ClosedAt: base.Add(-time.Minute), Labels: []string{"gc:session"}},
		{ID: "missing-closed-at", Title: "missing", Status: "closed", CreatedAt: base.Add(-48 * time.Hour), Labels: []string{"gc:session"}},
	}

	got := ApplyListQuery(items, ListQuery{
		Label:         "gc:session",
		ClosedBefore:  base,
		Limit:         1,
		IncludeClosed: true,
		Sort:          SortCreatedDesc,
	})

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1: %+v", len(got), got)
	}
	if got[0].ID != "created-new-closed-old" {
		t.Fatalf("got[0].ID = %q, want created-new-closed-old", got[0].ID)
	}
}
