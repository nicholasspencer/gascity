package beads

import (
	"encoding/json"
	"testing"
)

func TestNotifyingStoreEmitsWriteEvents(t *testing.T) {
	backing := NewMemStore()
	var events []string
	var payloads []Bead
	store := NewNotifyingStore(backing, func(eventType, beadID string, payload json.RawMessage) {
		events = append(events, eventType+":"+beadID)
		var b Bead
		if err := json.Unmarshal(payload, &b); err != nil {
			t.Fatalf("notification payload: %v", err)
		}
		payloads = append(payloads, b)
	})

	created, err := store.Create(Bead{Title: "work", Type: "task"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.SetMetadata(created.ID, "gc.routed_to", "worker"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if err := store.Close(created.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	want := []string{
		"bead.created:" + created.ID,
		"bead.updated:" + created.ID,
		"bead.closed:" + created.ID,
		"bead.deleted:" + created.ID,
	}
	if len(events) != len(want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events[%d] = %q, want %q; all events=%#v", i, events[i], want[i], events)
		}
	}
	if payloads[2].Status != "closed" {
		t.Fatalf("close payload status = %q, want closed", payloads[2].Status)
	}
	if payloads[3].ID != created.ID {
		t.Fatalf("delete payload ID = %q, want %q", payloads[3].ID, created.ID)
	}
}

func TestNotifyingStoreEmitsEventsAfterTxCommit(t *testing.T) {
	backing := NewMemStore()
	created, err := backing.Create(Bead{Title: "work", Type: "task"})
	if err != nil {
		t.Fatalf("Create backing: %v", err)
	}

	var events []string
	store := NewNotifyingStore(backing, func(eventType, beadID string, _ json.RawMessage) {
		events = append(events, eventType+":"+beadID)
	})

	if err := store.Tx("close work", func(tx Tx) error {
		return tx.Close(created.ID)
	}); err != nil {
		t.Fatalf("Tx: %v", err)
	}

	want := "bead.closed:" + created.ID
	if len(events) != 1 || events[0] != want {
		t.Fatalf("events = %#v, want [%q]", events, want)
	}
}
