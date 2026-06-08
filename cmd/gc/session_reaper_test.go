package main

import (
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
)

// --- claim-state predicate (the load-bearing guard primitive) ---

// TestSessionHasInProgressClaimedWork distinguishes "owns a claimed
// (in_progress) step" (never reapable) from "owns only open/routed work or
// nothing" (reapable) — the correctness core of the pool-seat reaper guard.
func TestSessionHasInProgressClaimedWork(t *testing.T) {
	mk := func(status string) (beads.Store, string) {
		store := beads.NewMemStore()
		assignee := "ga-session-abc"
		b, err := store.Create(beads.Bead{Title: "step", Type: "task"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := store.Update(b.ID, beads.UpdateOpts{Status: &status, Assignee: &assignee}); err != nil {
			t.Fatalf("update: %v", err)
		}
		return store, assignee
	}

	// open routed-but-unclaimed work: NOT in_progress (reapable), but IS open-assigned.
	store, assignee := mk("open")
	if has, err := sessionHasInProgressClaimedWorkInStoreByIdentifiers(store, []string{assignee}); err != nil || has {
		t.Fatalf("open work: in_progress-claimed = %v (err %v), want false", has, err)
	}
	if has, err := sessionHasOpenAssignedWorkInStoreByIdentifiers(store, []string{assignee}); err != nil || !has {
		t.Fatalf("open work: open-assigned = %v (err %v), want true", has, err)
	}

	// in_progress claimed work: the seat owns a step in flight — never reap.
	store, assignee = mk("in_progress")
	if has, err := sessionHasInProgressClaimedWorkInStoreByIdentifiers(store, []string{assignee}); err != nil || !has {
		t.Fatalf("in_progress work: in_progress-claimed = %v (err %v), want true", has, err)
	}

	// no work at all: reapable.
	empty := beads.NewMemStore()
	if has, err := sessionHasInProgressClaimedWorkInStoreByIdentifiers(empty, []string{"ga-session-zzz"}); err != nil || has {
		t.Fatalf("no work: in_progress-claimed = %v (err %v), want false", has, err)
	}
}

// --- slot-free reason ---

func TestIsPoolSessionSlotFreeable_ReapedCompleted(t *testing.T) {
	freeable := beads.Bead{Metadata: map[string]string{"state": "asleep", "sleep_reason": "reaped-completed"}}
	if !isPoolSessionSlotFreeable(freeable) {
		t.Fatal("sleep_reason=reaped-completed must free the pool slot")
	}
	held := beads.Bead{Metadata: map[string]string{"state": "asleep", "sleep_reason": "wait-hold"}}
	if isPoolSessionSlotFreeable(held) {
		t.Fatal("sleep_reason=wait-hold must NOT free the slot")
	}
}

// --- reconciler triggers ---

// setupEphemeralSeat creates a live, ephemeral pool seat idle past the reap
// debounce. The caller optionally assigns it in_progress work.
func setupEphemeralSeat(t *testing.T) (*reconcilerTestEnv, beads.Bead) {
	t.Helper()
	env := newReconcilerTestEnv()
	env.cfg = &config.City{Agents: []config.Agent{{Name: "arch"}}}
	env.addDesired("arch", "arch", true) // running in the provider
	session := env.createSessionBead("arch", "arch")
	env.markSessionActive(&session)
	env.setSessionMetadata(&session, map[string]string{
		poolManagedMetadataKey: boolMetadata(true),
		"pool_slot":            "1",
	})
	// Last activity well past ephemeralReapDebounce → idle.
	env.sp.SetActivity("arch", env.clk.Now().Add(-5*time.Minute))
	return env, session
}

// TestReconcile_EphemeralSeatCompletedStepIsReaped: an ephemeral seat that
// finished its step (owns no in_progress work) and went idle is reaped
// (trigger 1: a completed ephemeral seat).
func TestReconcile_EphemeralSeatCompletedStepIsReaped(t *testing.T) {
	env, session := setupEphemeralSeat(t)

	env.reconcile([]beads.Bead{session})

	got, err := env.store.Get(session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata["sleep_reason"] != "reaped-completed" && got.Status != "closed" {
		t.Fatalf("idle ephemeral seat with no claimed work must be reaped; got status=%q sleep_reason=%q",
			got.Status, got.Metadata["sleep_reason"])
	}
}

// TestReconcile_EphemeralSeatWithInProgressStepNeverReaped is the LOAD-BEARING
// guard: a seat mid-step (owns an in_progress claimed step) must NEVER be
// reaped even when its last-activity is stale (e.g. a long model call), per the
// claim-state predicate (the mid-grade false-positive reap).
func TestReconcile_EphemeralSeatWithInProgressStepNeverReaped(t *testing.T) {
	env, session := setupEphemeralSeat(t)

	// The seat owns an in_progress claimed step.
	work, err := env.store.Create(beads.Bead{Title: "grade step", Type: "task"})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	st, assignee := "in_progress", session.ID
	if err := env.store.Update(work.ID, beads.UpdateOpts{Status: &st, Assignee: &assignee}); err != nil {
		t.Fatalf("update work: %v", err)
	}

	env.reconcile([]beads.Bead{session})

	got, err := env.store.Get(session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status == "closed" || got.Metadata["sleep_reason"] == "reaped-completed" {
		t.Fatalf("seat holding an in_progress step must NOT be reaped; got status=%q sleep_reason=%q",
			got.Status, got.Metadata["sleep_reason"])
	}
}
