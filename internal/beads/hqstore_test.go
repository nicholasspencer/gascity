package beads_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/beads/beadstest"
)

func TestHQStoreConformance(t *testing.T) {
	factory := func() beads.Store {
		t.Helper()
		store, err := beads.OpenHQStore(t.TempDir())
		if err != nil {
			t.Fatalf("OpenHQStore: %v", err)
		}
		t.Cleanup(func() {
			if err := store.Shutdown(); err != nil {
				t.Errorf("Shutdown: %v", err)
			}
		})
		return store
	}

	beadstest.RunStoreTests(t, factory)
	beadstest.RunDepTests(t, factory)
	beadstest.RunCreationOrderTests(t, factory)
}

func TestHQStoreRecoversAfterSIGKILL(t *testing.T) {
	if os.Getenv("HQSTORE_SIGKILL_HELPER") == "1" {
		hqStoreSIGKILLHelper(t)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestHQStoreRecoversAfterSIGKILL")
	cmd.Env = append(os.Environ(),
		"HQSTORE_SIGKILL_HELPER=1",
		"HQSTORE_DIR="+dir,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting helper: %v", err)
	}

	idPath := filepath.Join(dir, "created-id")
	var id string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(idPath)
		if err == nil {
			id = string(data)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if id == "" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("helper did not write created bead id")
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("killing helper: %v", err)
	}
	_ = cmd.Wait()

	recovered, err := beads.OpenHQStore(dir)
	if err != nil {
		t.Fatalf("reopen after kill: %v", err)
	}
	t.Cleanup(func() {
		if err := recovered.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	got, err := recovered.Get(id)
	if err != nil {
		t.Fatalf("Get(%q) after kill: %v", id, err)
	}
	if got.Title != "persist-before-sigkill" {
		t.Fatalf("recovered title = %q, want persist-before-sigkill", got.Title)
	}
}

func hqStoreSIGKILLHelper(t *testing.T) {
	t.Helper()
	dir := os.Getenv("HQSTORE_DIR")
	if dir == "" {
		t.Fatal("HQSTORE_DIR is required")
	}
	store, err := beads.OpenHQStore(dir, beads.WithHQStoreCheckpointEvery(0))
	if err != nil {
		t.Fatalf("helper OpenHQStore: %v", err)
	}
	created, err := store.Create(beads.Bead{Title: "persist-before-sigkill"})
	if err != nil {
		t.Fatalf("helper Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "created-id"), []byte(created.ID), 0o644); err != nil {
		t.Fatalf("helper write id: %v", err)
	}
	select {}
}

func TestHQStoreRecoverySkipsPartialFinalWALLine(t *testing.T) {
	dir := t.TempDir()
	store, err := beads.OpenHQStore(dir, beads.WithHQStoreCheckpointEvery(0))
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	created, err := store.Create(beads.Bead{Title: "before-partial-line"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Shutdown(); err != nil {
		t.Fatalf("closing store for corruption setup: %v", err)
	}

	walPath := filepath.Join(dir, "wal.jsonl")
	f, err := os.OpenFile(walPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open wal append: %v", err)
	}
	if _, err := f.WriteString(`{"op":"upsert","id":"never-finished"`); err != nil {
		_ = f.Close()
		t.Fatalf("write partial wal line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close partial wal file: %v", err)
	}

	recovered, err := beads.OpenHQStore(dir)
	if err != nil {
		t.Fatalf("OpenHQStore with partial final WAL line: %v", err)
	}
	t.Cleanup(func() {
		if err := recovered.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	got, err := recovered.Get(created.ID)
	if err != nil {
		t.Fatalf("Get(%q): %v", created.ID, err)
	}
	if got.Title != "before-partial-line" {
		t.Fatalf("recovered title = %q, want before-partial-line", got.Title)
	}
}

func TestHQStoreCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := beads.OpenHQStore(dir, beads.WithHQStoreCheckpointEvery(0))
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	first, err := store.Create(beads.Bead{Title: "checkpointed", Metadata: map[string]string{"phase": "one"}})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := store.Create(beads.Bead{Title: "wisp", Type: "message", Ephemeral: true})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if err := store.DepAdd(first.ID, second.ID, "tracks"); err != nil {
		t.Fatalf("DepAdd: %v", err)
	}
	if err := store.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if err := store.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	recovered, err := beads.OpenHQStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() {
		if err := recovered.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	got, err := recovered.Get(first.ID)
	if err != nil {
		t.Fatalf("Get first: %v", err)
	}
	if got.Metadata["phase"] != "one" {
		t.Fatalf("metadata phase = %q, want one", got.Metadata["phase"])
	}
	deps, err := recovered.DepList(first.ID, "down")
	if err != nil {
		t.Fatalf("DepList: %v", err)
	}
	if len(deps) != 1 || deps[0].DependsOnID != second.ID {
		t.Fatalf("deps = %+v, want dependency on %s", deps, second.ID)
	}
}

func TestHQStorePurgeExpired(t *testing.T) {
	store, err := beads.OpenHQStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	expired, err := store.Create(beads.Bead{
		Title:     "expired",
		Type:      "order-tracking",
		Ephemeral: true,
		Metadata: map[string]string{
			"expires_at": time.Now().Add(-time.Second).Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatalf("Create expired: %v", err)
	}
	live, err := store.Create(beads.Bead{
		Title:     "live",
		Type:      "order-tracking",
		Ephemeral: true,
		Metadata: map[string]string{
			"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatalf("Create live: %v", err)
	}

	purged, err := store.PurgeExpired()
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if purged != 1 {
		t.Fatalf("PurgeExpired purged %d, want 1", purged)
	}
	if _, err := store.Get(expired.ID); !errors.Is(err, beads.ErrNotFound) {
		t.Fatalf("Get expired error = %v, want ErrNotFound", err)
	}
	if _, err := store.Get(live.ID); err != nil {
		t.Fatalf("Get live: %v", err)
	}
}

func TestHQStoreConcurrentCreateUpdate(t *testing.T) {
	store, err := beads.OpenHQStore(t.TempDir(), beads.WithHQStoreCheckpointEvery(0))
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := range workers {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			created, err := store.Create(beads.Bead{
				Title:    fmt.Sprintf("worker-%d", i),
				Assignee: "builder",
			})
			if err != nil {
				errs <- err
				return
			}
			status := "in_progress"
			if err := store.Update(created.ID, beads.UpdateOpts{Status: &status}); err != nil {
				errs <- err
				return
			}
			if err := store.SetMetadataBatch(created.ID, map[string]string{"worker": fmt.Sprint(i)}); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent worker error: %v", err)
		}
	}

	got, err := store.List(beads.ListQuery{Assignee: "builder", Status: "in_progress", AllowScan: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != workers {
		t.Fatalf("List returned %d workers, want %d", len(got), workers)
	}
	seen := make(map[string]bool, workers)
	for _, b := range got {
		if seen[b.ID] {
			t.Fatalf("duplicate ID %q", b.ID)
		}
		seen[b.ID] = true
	}
}
