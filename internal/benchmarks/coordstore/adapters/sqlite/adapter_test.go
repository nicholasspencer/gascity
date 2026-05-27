package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/benchmarks/coordstore"
)

// TestWALBoundedUnderSustainedWrites is the ga-2s6sz spike regression test:
// with FullSyncPragmas (wal_autocheckpoint=1000) and the background
// wal_checkpoint(TRUNCATE) loop enabled, the on-disk WAL file must not grow
// without bound under sustained writes. Before the spike the WAL was
// monotone for the lifetime of the process — see ga-tm9sg for the 8GB OOM
// that motivated the fix.
func TestWALBoundedUnderSustainedWrites(t *testing.T) {
	// Short interval so the goroutine fires several times in this test's
	// budget; the production default is 30s.
	t.Setenv("COORDSTORE_SQLITE_CHECKPOINT_INTERVAL", "200ms")

	dir := t.TempDir()
	ctx := context.Background()

	a := NewWithDriver(DefaultDriverName, FullSyncPragmas, "tst")
	if err := a.Open(ctx, coordstore.Config{DataDir: dir}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	// Hammer writes — enough to exceed 1000 pages (~4MB) and trigger
	// wal_autocheckpoint on the writer side at least once. With FULL sync
	// each commit fsyncs, so this also exercises the goroutine + writer
	// contention path.
	const writes = 1500
	for i := 0; i < writes; i++ {
		_, err := a.Create(ctx, coordstore.Record{
			Title:    fmt.Sprintf("wal-bound-%d", i),
			Status:   "open",
			Type:     "task",
			Assignee: "test",
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// Give the TRUNCATE goroutine room to fire after the last write.
	time.Sleep(500 * time.Millisecond)

	stats := a.Stats(ctx)
	wal, ok := stats["wal_size_bytes"]
	if !ok {
		t.Fatalf("stats missing wal_size_bytes; got %#v", stats)
	}
	// 16MiB is a generous ceiling: the WAL high-water mark on a healthy
	// run sits around 4-8MB between auto-checkpoints; ga-tm9sg's broken
	// configuration would already be tens of MB by this point. Tightening
	// this bound is a follow-up after the soak data lands.
	const ceiling = 16 << 20
	if wal > ceiling {
		t.Errorf("wal_size_bytes = %d, want <= %d (%dMB ceiling)", wal, ceiling, ceiling>>20)
	}
	t.Logf("wal_size_bytes after %d writes + 500ms drain: %d (%.2f MB)",
		writes, wal, float64(wal)/(1<<20))
}

// TestWALUnboundedWithCheckpointerDisabled documents the prior failure
// mode: with the background loop off and wal_autocheckpoint=0 (DefaultPragmas
// in the pure-Go path), the same write volume leaves a WAL that already
// dwarfs the bounded ceiling. This locks in the contrast so future changes
// to either knob don't silently regress to the OOM-prone configuration.
func TestWALUnboundedWithCheckpointerDisabled(t *testing.T) {
	t.Setenv("COORDSTORE_SQLITE_CHECKPOINT_INTERVAL", "off")

	dir := t.TempDir()
	ctx := context.Background()

	// DefaultPragmas keeps wal_autocheckpoint=0 from before the spike.
	a := NewWithDriver(DefaultDriverName, DefaultPragmas, "tst")
	if err := a.Open(ctx, coordstore.Config{DataDir: dir}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	const writes = 1500
	for i := 0; i < writes; i++ {
		_, err := a.Create(ctx, coordstore.Record{
			Title:    fmt.Sprintf("wal-unbound-%d", i),
			Status:   "open",
			Type:     "task",
			Assignee: "test",
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	stats := a.Stats(ctx)
	wal := stats["wal_size_bytes"]
	// With autocheckpoint=0 and no background TRUNCATE, the WAL should be
	// noticeably larger than the bounded-config ceiling. This is the
	// canary: if it ever drops back below 4MB, somebody quietly fixed the
	// default and the contrast test loses its meaning.
	const floor = 4 << 20
	if wal < floor {
		t.Errorf("wal_size_bytes = %d, expected > %d (%dMB) to demonstrate "+
			"the unbounded-WAL failure mode this contrast test exists to lock in",
			wal, floor, floor>>20)
	}
	t.Logf("wal_size_bytes after %d writes with checkpointer disabled: %d (%.2f MB)",
		writes, wal, float64(wal)/(1<<20))
}
