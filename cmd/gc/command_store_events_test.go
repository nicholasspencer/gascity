package main

import (
	"context"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/runtime"
)

func TestCommandStoreEventsWrapsNativeStoreWrites(t *testing.T) {
	backing := beads.NewMemStore()
	rec := events.NewFake()
	result := beads.StoreOpenResult{
		Store: backing,
		Diagnostic: beads.BeadsDiagnostic{
			Store: beads.BeadsStoreNameNativeDoltStore,
		},
	}

	wrapped := withCommandBeadEvents(result, rec)
	created, err := wrapped.Store.Create(beads.Bead{Title: "native cli work", Type: "task"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := wrapped.Store.Close(created.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := rec.List(events.Filter{Type: events.BeadClosed})
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("closed events = %#v, want one", got)
	}
	if got[0].Subject != created.ID {
		t.Fatalf("closed subject = %q, want %q", got[0].Subject, created.ID)
	}
	if len(got[0].Payload) == 0 {
		t.Fatal("closed event payload is empty")
	}
}

func TestCommandStoreEventsLeavesHookBackedStoresUnwrapped(t *testing.T) {
	backing := beads.NewMemStore()
	rec := events.NewFake()
	result := beads.StoreOpenResult{
		Store: backing,
		Diagnostic: beads.BeadsDiagnostic{
			Store: beads.BeadsStoreNameBdStore,
		},
	}

	wrapped := withCommandBeadEvents(result, rec)
	if wrapped.Store != backing {
		t.Fatalf("store = %T, want original backing for hook-backed store", wrapped.Store)
	}
}

func TestCommandNativeStoreEventUpdatesControllerCache(t *testing.T) {
	backing := beads.NewMemStore()
	created, err := backing.Create(beads.Bead{Title: "workflow source", Type: "task"})
	if err != nil {
		t.Fatalf("Create backing: %v", err)
	}

	prevCityStore := newControllerStateOpenCityStore
	newControllerStateOpenCityStore = func(string) (beads.StoreOpenResult, error) {
		return beads.StoreOpenResult{Store: backing}, nil
	}
	t.Cleanup(func() {
		newControllerStateOpenCityStore = prevCityStore
	})

	rec := events.NewFake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cs := newControllerState(ctx, &config.City{Workspace: config.Workspace{Name: "test-city"}}, runtime.NewFake(), rec, "test-city", t.TempDir())
	cs.startBeadEventWatcher(ctx)

	controllerCache, ok := cs.cityBeadStore.(*beads.CachingStore)
	if !ok {
		t.Fatalf("cityBeadStore = %T, want *beads.CachingStore", cs.cityBeadStore)
	}
	before, err := controllerCache.Get(created.ID)
	if err != nil {
		t.Fatalf("controller cache Get before close: %v", err)
	}
	if before.Status != "open" {
		t.Fatalf("controller cache status before close = %q, want open", before.Status)
	}

	commandStore := withCommandBeadEvents(beads.StoreOpenResult{
		Store: backing,
		Diagnostic: beads.BeadsDiagnostic{
			Store: beads.BeadsStoreNameNativeDoltStore,
		},
	}, rec).Store
	if err := commandStore.Close(created.ID); err != nil {
		t.Fatalf("command store Close: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		got, err := controllerCache.Get(created.ID)
		if err == nil && got.Status == "closed" {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("controller cache did not observe command close; last status=%q err=%v", got.Status, err)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
