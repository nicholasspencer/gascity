//go:build cgo && sqlite_cgo

package beads

import (
	"runtime"
	"testing"
	"time"
)

// TestSQLiteCGOStoreLeaksWithoutCloseStore characterizes ga-qsvwe1.
//
// OpenSQLiteCGOStore starts a retention-sweeper goroutine (startRetentionSweeper)
// plus the database/sql connectionOpener/connectionCleaner goroutines, and holds
// a *sql.DB with live cgo SQLite connections. There is no CloseStore() method to
// stop the sweeper or close the *sql.DB, and the running sweeper goroutine pins
// the store in memory so GC cannot reclaim it.
//
// Production opens-and-discards a fresh coordstore on this path every tick
// (memoryOrderDispatcher.dispatch -> storeFn=openStoreAtForCity), so each tick
// leaks >=1 store's goroutines + connections. This test reproduces that pattern
// in isolation: after discarding N stores, the goroutine count stays elevated.
//
// It is a CHARACTERIZATION test (documents the bug, passes today). The fix adds
// CloseStore(); the regression test in the fix spec asserts open+CloseStore
// returns the goroutine count to baseline.
func TestSQLiteCGOStoreLeaksWithoutCloseStore(t *testing.T) {
	const n = 25

	settle := func() {
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
		runtime.GC()
	}

	settle()
	base := runtime.NumGoroutine()

	for i := 0; i < n; i++ {
		// Retention sweeper ENABLED, matching production openCoordStoreAt
		// (WithSQLiteCGOStoreRetention(4h, 30s)).
		_, err := OpenSQLiteCGOStore(t.TempDir(),
			WithSQLiteCGOStoreRetention(4*time.Hour, 30*time.Second))
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		// Intentionally discard the store, exactly like
		// memoryOrderDispatcher.dispatch()'s per-tick `stores` map.
	}

	settle()
	leaked := runtime.NumGoroutine() - base
	t.Logf("goroutines: base=%d after=%d leaked=%d (discarded %d stores)",
		base, base+leaked, leaked, n)

	if leaked < n {
		t.Fatalf("expected >= %d leaked goroutines (>=1 immortal sweeper per discarded store); got %d — the leak may have been fixed (add a CloseStore-based regression test instead)", n, leaked)
	}
}

// TestSQLiteCGOStoreCloseStoreReleasesResources is the regression test for the
// ga-qsvwe1 fix: opening N stores with the retention sweeper enabled and then
// calling CloseStore() on each must return the goroutine count to ~baseline
// (the sweeper goroutine exits and the *sql.DB connection goroutines stop).
func TestSQLiteCGOStoreCloseStoreReleasesResources(t *testing.T) {
	const n = 25

	settle := func() {
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
		runtime.GC()
	}

	settle()
	base := runtime.NumGoroutine()

	for i := 0; i < n; i++ {
		store, err := OpenSQLiteCGOStore(t.TempDir(),
			WithSQLiteCGOStoreRetention(4*time.Hour, 30*time.Second))
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		closer, ok := store.(interface{ CloseStore() error })
		if !ok {
			t.Fatalf("SQLiteCGOStore does not implement CloseStore() error")
		}
		if err := closer.CloseStore(); err != nil {
			t.Fatalf("CloseStore %d: %v", i, err)
		}
		// CloseStore must be idempotent.
		if err := closer.CloseStore(); err != nil {
			t.Fatalf("second CloseStore %d: %v", i, err)
		}
	}

	settle()
	residual := runtime.NumGoroutine() - base
	t.Logf("goroutines: base=%d after=%d residual=%d (opened+closed %d stores)",
		base, base+residual, residual, n)

	// Allow a small slack for scheduler/runtime goroutines; without the fix
	// this would be ~3*n.
	if residual > 5 {
		t.Fatalf("CloseStore did not release resources: residual goroutines=%d after %d open+close cycles (want <=5)", residual, n)
	}
}
